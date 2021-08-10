package polynomial

import (
	"crypto/rand"
	"math/big"
	mrand "math/rand"
	"testing"

	"github.com/cronokirby/safenum"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/taurusgroup/multi-party-sig/pkg/math/curve"
	"github.com/taurusgroup/multi-party-sig/pkg/math/sample"
)

func TestPolynomial_Constant(t *testing.T) {
	var group curve.Curve
	deg := 10
	secret := sample.Scalar(rand.Reader, group)
	poly := NewPolynomial(group, deg, secret)
	require.True(t, poly.Constant().Equal(secret))
}

func TestPolynomial_Evaluate(t *testing.T) {
	var group curve.Curve
	polynomial := &Polynomial{group, make([]curve.Scalar, 3)}
	polynomial.coefficients[0].SetUInt32(1)
	polynomial.coefficients[1].SetUInt32(0)
	polynomial.coefficients[2].SetUInt32(1)

	for index := 0; index < 100; index++ {
		x := big.NewInt(int64(mrand.Uint32()))
		result := new(big.Int).Set(x)
		result.Mul(result, result)
		result.Add(result, big.NewInt(1))
		xScalar := group.NewScalar().SetNat(new(safenum.Nat).SetBig(x, x.BitLen()))
		computedResult := polynomial.Evaluate(xScalar)
		expectedResult := group.NewScalar().SetNat(new(safenum.Nat).SetBig(result, result.BitLen()))
		assert.True(t, expectedResult.Equal(computedResult))
	}
}
