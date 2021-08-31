package sign

import (
	"fmt"

	"github.com/taurusgroup/multi-party-sig/internal/round"
	"github.com/taurusgroup/multi-party-sig/pkg/hash"
	"github.com/taurusgroup/multi-party-sig/pkg/math/curve"
	"github.com/taurusgroup/multi-party-sig/pkg/party"
	"github.com/taurusgroup/multi-party-sig/pkg/protocol"
	"github.com/taurusgroup/multi-party-sig/pkg/protocol/types"
	"github.com/taurusgroup/multi-party-sig/protocols/frost/keygen"
)

const (
	// Frost Sign with Threshold.
	protocolID types.ProtocolID = "frost/sign-threshold"
	// This protocol has 3 concrete rounds.
	protocolRounds types.RoundNumber = 3
)

func startSignCommon(taproot bool, err error, result *keygen.Result, signers []party.ID, messageHash []byte) protocol.StartFunc {
	return func() (round.Round, protocol.Info, error) {
		group := result.Curve()
		// This is a bit of a hack, so that the Taproot can tell this function that the public key
		// is invalid.
		if err != nil {
			return nil, nil, err
		}
		sortedIDs := party.NewIDSlice(signers)
		var taprootFlag byte
		if taproot {
			taprootFlag = 1
		}
		helper, err := round.NewHelper(
			protocolID,
			group,
			protocolRounds,
			result.ID,
			sortedIDs,
			&hash.BytesWithDomain{
				TheDomain: "Taproot Flag",
				Bytes:     []byte{taprootFlag},
			},
		)
		if err != nil {
			return nil, nil, fmt.Errorf("sign.StartSign: %w", err)
		}
		// We delay this until *after* creating the helper, that way we know that
		// sortedIDs contains no duplicates.
		if result.Threshold+1 > sortedIDs.Len() {
			return nil, nil, fmt.Errorf("sign.StartSign: insufficient number of signers")
		}
		return &round1{
			Helper:  helper,
			taproot: taproot,
			M:       messageHash,
			Y:       result.PublicKey,
			YShares: result.VerificationShares.Points,
			s_i:     result.PrivateShare,
		}, helper, nil
	}
}

// StartSign initiates the protocol for producing a threshold signature, with Frost.
//
// result is the result of the key generation phase, for this participant.
//
// signers is the list of all participants generating a signature together, including
// this participant.
//
// messageHash is the hash of the message a signature should be generated for.
//
// This protocol merges Figures 2 and 3 from the Frost paper:
//   https://eprint.iacr.org/2020/852.pdf
//
//
// We merge the pre-processing and signing protocols into a single signing protocol
// which doesn't require any pre-processing.
//
// Another major difference is that there's no central "Signing Authority".
// Instead, each participant independently verifies and broadcasts items as necessary.
//
// Differences stemming from this change are commented throughout the protocol.
func StartSign(result *keygen.Result, signers []party.ID, messageHash []byte) protocol.StartFunc {
	return startSignCommon(false, nil, result, signers, messageHash)
}

// StartSignTaproot is like StartSign, but will generate a Taproot / BIP-340 compatible signature.
//
// This needs to result of a Taproot compatible key generation phase, naturally.
//
// See: https://github.com/bitcoin/bips/blob/master/bip-0340.mediawiki
func StartSignTaproot(result *keygen.TaprootResult, signers []party.ID, messageHash []byte) protocol.StartFunc {
	publicKey, err := curve.Secp256k1{}.LiftX(result.PublicKey)
	genericVerificationShares := make(map[party.ID]curve.Point)
	for k, v := range result.VerificationShares {
		genericVerificationShares[k] = v
	}
	normalResult := &keygen.Result{
		ID:                 result.ID,
		Threshold:          result.Threshold,
		PrivateShare:       result.PrivateShare,
		PublicKey:          publicKey,
		VerificationShares: party.NewPointMap(genericVerificationShares),
	}
	return startSignCommon(true, err, normalResult, signers, messageHash)
}
