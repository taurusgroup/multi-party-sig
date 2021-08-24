package keygen

import (
	"errors"

	"github.com/taurusgroup/multi-party-sig/internal/round"
	"github.com/taurusgroup/multi-party-sig/internal/types"
	"github.com/taurusgroup/multi-party-sig/pkg/math/curve"
	"github.com/taurusgroup/multi-party-sig/pkg/math/polynomial"
	"github.com/taurusgroup/multi-party-sig/pkg/paillier"
	"github.com/taurusgroup/multi-party-sig/pkg/party"
	zkmod "github.com/taurusgroup/multi-party-sig/pkg/zk/mod"
	zkprm "github.com/taurusgroup/multi-party-sig/pkg/zk/prm"
	"github.com/taurusgroup/multi-party-sig/protocols/cmp/config"
)

var _ round.Round = (*round4)(nil)

type round4 struct {
	*round3

	// RID = ⊕ⱼ RIDⱼ
	// Random ID generated by taking the XOR of all ridᵢ
	RID types.RID
	// ChainKey is a sequence of random bytes agreed upon together
	ChainKey []byte
}

type message4 struct {
	Mod *zkmod.Proof
	Prm *zkprm.Proof
	// Share = Encᵢ(x) is the encryption of the receivers share
	Share *paillier.Ciphertext
}

// VerifyMessage implements round.Round.
//
// - verify Mod, Prm proof for N
func (r *round4) VerifyMessage(msg round.Message) error {
	from := msg.From
	body, ok := msg.Content.(*message4)
	if !ok || body == nil {
		return round.ErrInvalidContent
	}

	if body.Mod == nil || body.Prm == nil || body.Share == nil {
		return round.ErrNilFields
	}

	// verify zkmod
	if !body.Mod.Verify(zkmod.Public{N: r.NModulus[from]}, r.HashForID(from), r.Pool) {
		return errors.New("failed to validate mod proof")
	}

	// verify zkprm
	if !body.Prm.Verify(zkprm.Public{N: r.NModulus[from], S: r.S[from], T: r.T[from]}, r.HashForID(from), r.Pool) {
		return errors.New("failed to validate prm proof")
	}

	return nil
}

// StoreMessage implements round.Round.
//
// Since this message is only intended for us, we need to to the VSS verification here.
// - check that the decrypted share did not overflow.
// - check VSS condition.
// - save share.
func (r *round4) StoreMessage(msg round.Message) error {
	from, body := msg.From, msg.Content.(*message4)
	// decrypt share
	DecryptedShare, err := r.PaillierSecret.Dec(body.Share)
	if err != nil {
		return err
	}
	Share := r.Group().NewScalar().SetNat(DecryptedShare.Mod(r.Group().Order()))
	if DecryptedShare.Eq(curve.MakeInt(Share)) != 1 {
		return errors.New("decrypted share is not in correct range")
	}

	// verify share with VSS
	ExpectedPublicShare := r.VSSPolynomials[from].Evaluate(r.SelfID().Scalar(r.Group())) // Fⱼ(i)
	PublicShare := Share.ActOnBase()
	// X == Fⱼ(i)
	if !PublicShare.Equal(ExpectedPublicShare) {
		return errors.New("failed to validate VSS share")
	}

	r.ShareReceived[from] = Share
	return nil
}

// Finalize implements round.Round
//
// - sum of all received shares
// - compute group public key and individual public keys
// - recompute config SSID
// - validate Config
// - write new ssid hash to old hash state
// - create proof of knowledge of secret.
func (r *round4) Finalize(out chan<- *round.Message) (round.Session, error) {
	// add all shares to our secret
	UpdatedSecretECDSA := r.Group().NewScalar()
	if r.PreviousSecretECDSA != nil {
		UpdatedSecretECDSA.Set(r.PreviousSecretECDSA)
	}
	for _, j := range r.PartyIDs() {
		UpdatedSecretECDSA.Add(r.ShareReceived[j])
	}

	// [F₁(X), …, Fₙ(X)]
	ShamirPublicPolynomials := make([]*polynomial.Exponent, 0, len(r.VSSPolynomials))
	for _, VSSPolynomial := range r.VSSPolynomials {
		ShamirPublicPolynomials = append(ShamirPublicPolynomials, VSSPolynomial)
	}

	// ShamirPublicPolynomial = F(X) = ∑Fⱼ(X)
	ShamirPublicPolynomial, err := polynomial.Sum(ShamirPublicPolynomials)
	if err != nil {
		return nil, err
	}

	// compute the new public key share Xⱼ = F(j) (+X'ⱼ if doing a refresh)
	PublicData := make(map[party.ID]*config.Public, len(r.PartyIDs()))
	for _, j := range r.PartyIDs() {
		PublicECDSAShare := ShamirPublicPolynomial.Evaluate(j.Scalar(r.Group()))
		if r.PreviousPublicSharesECDSA != nil {
			PublicECDSAShare = PublicECDSAShare.Add(r.PreviousPublicSharesECDSA[j])
		}
		PublicData[j] = &config.Public{
			ECDSA:   PublicECDSAShare,
			ElGamal: r.ElGamalPublic[j],
			N:       r.NModulus[j],
			S:       r.S[j],
			T:       r.T[j],
		}
	}

	UpdatedConfig := &config.Config{
		Group:     r.Group(),
		Threshold: r.Threshold(),
		Public:    PublicData,
		RID:       r.RID.Copy(),
		ChainKey:  r.ChainKey,
		ID:        r.SelfID(),
		ECDSA:     UpdatedSecretECDSA,
		ElGamal:   r.ElGamalSecret,
		P:         r.PaillierSecret.P(),
		Q:         r.PaillierSecret.Q(),
	}

	// write new ssid to hash, to bind the Schnorr proof to this new config
	// Write SSID, selfID to temporary hash
	h := r.Hash()
	_ = h.WriteAny(UpdatedConfig, r.SelfID())

	proof := r.SchnorrRand.Prove(h, PublicData[r.SelfID()].ECDSA, UpdatedSecretECDSA)

	// send to all
	err = r.SendMessage(out, &message5{SchnorrResponse: proof}, "")
	if err != nil {
		return r, err
	}

	r.UpdateHashState(UpdatedConfig)
	return &round5{
		round4:        r,
		UpdatedConfig: UpdatedConfig,
	}, nil
}

// MessageContent implements round.Round.
func (round4) MessageContent() round.Content { return &message4{} }

// Number implements round.Round.
func (round4) Number() round.Number { return 4 }

// Init implements round.Content.
func (message4) Init(curve curve.Curve) {}
