package sign

import (
	"errors"

	"github.com/cronokirby/safenum"
	"github.com/taurusgroup/multi-party-sig/internal/hash"
	"github.com/taurusgroup/multi-party-sig/internal/mta"
	"github.com/taurusgroup/multi-party-sig/internal/round"
	"github.com/taurusgroup/multi-party-sig/pkg/math/curve"
	"github.com/taurusgroup/multi-party-sig/pkg/paillier"
	"github.com/taurusgroup/multi-party-sig/pkg/party"
	zkenc "github.com/taurusgroup/multi-party-sig/pkg/zk/enc"
	zklogstar "github.com/taurusgroup/multi-party-sig/pkg/zk/logstar"
)

var _ round.Round = (*round2)(nil)

type round2 struct {
	*round1

	// K[j] = Kⱼ = encⱼ(kⱼ)
	K map[party.ID]*paillier.Ciphertext
	// G[j] = Gⱼ = encⱼ(γⱼ)
	G map[party.ID]*paillier.Ciphertext

	// BigGammaShare[j] = Γⱼ = [γⱼ]•G
	BigGammaShare map[party.ID]curve.Point

	// GammaShare = γᵢ <- 𝔽
	GammaShare *safenum.Int
	// KShare = kᵢ  <- 𝔽
	KShare curve.Scalar

	// KNonce = ρᵢ <- ℤₙ
	// used to encrypt Kᵢ = Encᵢ(kᵢ)
	KNonce *safenum.Nat
	// GNonce = νᵢ <- ℤₙ
	// used to encrypt Gᵢ = Encᵢ(γᵢ)
	GNonce *safenum.Nat
}

type broadcast2 struct {
	K *paillier.Ciphertext
	G *paillier.Ciphertext
}

type message2 struct {
	broadcast2
	ProofEnc *zkenc.Proof
}

// VerifyMessage implements round.Round.
//
// - verify zkenc(Kⱼ).
func (r *round2) VerifyMessage(msg round.Message) error {
	from, to := msg.From, msg.To
	body, ok := msg.Content.(*message2)
	if !ok || body == nil {
		return round.ErrInvalidContent
	}

	if body.ProofEnc == nil || body.G == nil || body.K == nil {
		return round.ErrNilFields
	}

	if !body.ProofEnc.Verify(r.Group(), r.HashForID(from), zkenc.Public{
		K:      body.K,
		Prover: r.Paillier[from],
		Aux:    r.Pedersen[to],
	}) {
		return errors.New("failed to validate enc proof for K")
	}
	return nil
}

// StoreMessage implements round.Round.
//
// - store Kⱼ, Gⱼ.
func (r *round2) StoreMessage(msg round.Message) error {
	from, body := msg.From, msg.Content.(*message2)
	r.K[from] = body.K
	r.G[from] = body.G
	return nil
}

// Finalize implements round.Round
//
// - compute Hash(ssid, K₁, G₁, …, Kₙ, Gₙ).
func (r *round2) Finalize(out chan<- *round.Message) (round.Session, error) {
	otherIDs := r.OtherPartyIDs()
	type mtaOut struct {
		err       error
		DeltaBeta *safenum.Int
		ChiBeta   *safenum.Int
	}
	mtaOuts := r.Pool.Parallelize(len(otherIDs), func(i int) interface{} {
		j := otherIDs[i]

		DeltaBeta, DeltaD, DeltaF, DeltaProof := mta.ProveAffG(r.Group(), r.HashForID(r.SelfID()),
			r.GammaShare, r.BigGammaShare[r.SelfID()], r.K[j],
			r.SecretPaillier, r.Paillier[j], r.Pedersen[j])
		ChiBeta, ChiD, ChiF, ChiProof := mta.ProveAffG(r.Group(),
			r.HashForID(r.SelfID()), curve.MakeInt(r.SecretECDSA), r.ECDSA[r.SelfID()], r.K[j],
			r.SecretPaillier, r.Paillier[j], r.Pedersen[j])

		proof := zklogstar.NewProof(r.Group(), r.HashForID(r.SelfID()),
			zklogstar.Public{
				C:      r.G[r.SelfID()],
				X:      r.BigGammaShare[r.SelfID()],
				Prover: r.Paillier[r.SelfID()],
				Aux:    r.Pedersen[j],
			}, zklogstar.Private{
				X:   r.GammaShare,
				Rho: r.GNonce,
			})

		err := r.SendMessage(out, &message3{
			BigGammaShare: r.BigGammaShare[r.SelfID()],
			DeltaD:        DeltaD,
			DeltaF:        DeltaF,
			DeltaProof:    DeltaProof,
			ChiD:          ChiD,
			ChiF:          ChiF,
			ChiProof:      ChiProof,
			ProofLog:      proof,
		}, j)
		return mtaOut{
			err:       err,
			DeltaBeta: DeltaBeta,
			ChiBeta:   ChiBeta,
		}
	})
	DeltaShareBetas := make(map[party.ID]*safenum.Int, len(otherIDs)-1)
	ChiShareBetas := make(map[party.ID]*safenum.Int, len(otherIDs)-1)
	for idx, mtaOutRaw := range mtaOuts {
		j := otherIDs[idx]
		m := mtaOutRaw.(mtaOut)
		if m.err != nil {
			return r, m.err
		}
		DeltaShareBetas[j] = m.DeltaBeta
		ChiShareBetas[j] = m.ChiBeta
	}

	return &round3{
		round2:          r,
		DeltaShareBeta:  DeltaShareBetas,
		ChiShareBeta:    ChiShareBetas,
		DeltaShareAlpha: map[party.ID]*safenum.Int{},
		ChiShareAlpha:   map[party.ID]*safenum.Int{},
	}, nil
}

// MessageContent implements round.Round.
func (round2) MessageContent() round.Content { return &message2{} }

// Number implements round.Round.
func (round2) Number() round.Number { return 2 }

// Init implements round.Content.
func (message2) Init(curve.Curve) {}

// BroadcastData implements broadcast.Broadcaster.
func (m broadcast2) BroadcastData() []byte {
	return hash.New(m.K, m.G).Sum()
}
