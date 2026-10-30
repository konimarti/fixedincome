// Package rate converts interest rates between compounding conventions.
//
// All rates are decimals (0.02 = 2%). Inside the fixedincome library rates
// are continuously compounded; the functions of this package convert to and
// from periodically compounded and simple rates, e.g. for reporting.
package rate

import (
	"fmt"
	"math"
)

// ToContinuous converts the rate r (decimal) compounded n times per year
// (n=1: annual, n=2: semi-annual, ...) into a continuously compounded rate:
// rc = n * ln(1 + r/n). It returns an error if n <= 0 or 1 + r/n <= 0.
func ToContinuous(r float64, n int) (float64, error) {
	if n <= 0 {
		return math.NaN(), fmt.Errorf("rate: invalid compounding frequency %d: must be positive", n)
	}
	x := 1.0 + r/float64(n)
	if !(x > 0) {
		return math.NaN(), fmt.Errorf("rate: rate %v with compounding frequency %d out of domain (1 + r/n <= 0)", r, n)
	}
	return float64(n) * math.Log1p(r/float64(n)), nil
}

// FromContinuous converts the continuously compounded rate rc (decimal) into
// the equivalent rate compounded n times per year: r = n * (exp(rc/n) - 1).
// It returns an error if n <= 0 or rc is not finite.
func FromContinuous(rc float64, n int) (float64, error) {
	if n <= 0 {
		return math.NaN(), fmt.Errorf("rate: invalid compounding frequency %d: must be positive", n)
	}
	if math.IsNaN(rc) || math.IsInf(rc, 0) {
		return math.NaN(), fmt.Errorf("rate: invalid rate %v", rc)
	}
	return float64(n) * math.Expm1(rc/float64(n)), nil
}

// EffectiveAnnual returns the effective annual rate (decimal) of the rate r
// compounded n times per year: (1 + r/n)^n - 1. It returns an error if
// n <= 0 or 1 + r/n <= 0.
func EffectiveAnnual(r float64, n int) (float64, error) {
	rc, err := ToContinuous(r, n)
	if err != nil {
		return math.NaN(), err
	}
	return math.Expm1(rc), nil
}

// ToSimple converts the continuously compounded rate rc (decimal) over a
// period of tau years into the simple (money-market) rate
// rs = (exp(rc*tau) - 1) / tau. For tau = 0 the limit rc is returned.
func ToSimple(rc, tau float64) float64 {
	if tau == 0 {
		return rc
	}
	return math.Expm1(rc*tau) / tau
}

// FromSimple converts the simple rate rs (decimal) over a period of tau
// years into a continuously compounded rate rc = ln(1 + rs*tau) / tau. It
// returns an error if 1 + rs*tau <= 0. For tau = 0 the limit rs is returned.
func FromSimple(rs, tau float64) (float64, error) {
	if tau == 0 {
		return rs, nil
	}
	if !(1+rs*tau > 0) {
		return math.NaN(), fmt.Errorf("rate: simple rate %v over %v years out of domain (1 + rs*tau <= 0)", rs, tau)
	}
	return math.Log1p(rs*tau) / tau, nil
}
