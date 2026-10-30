package term

import (
	"fmt"
	"math"
)

// NSS is the Nelson-Siegel-Svensson spot-rate term structure
//
//	r(t) = B0 + B1*g(t/Tau1) + B2*(g(t/Tau1) - exp(-t/Tau1)) + B3*(g(t/Tau2) - exp(-t/Tau2))
//
// with g(x) = (1 - exp(-x))/x. The betas are decimals (0.01 = 1%; note that
// central banks such as the SNB publish them in percent) and the decay
// parameters Tau1, Tau2 are in years and must be positive (see Validate).
// r(t) is a continuously compounded zero rate.
//
// Source: https://data.snb.ch/en/topics/ziredev#!/doc/explanations_ziredev#interest_rates_meth_par_siegel
type NSS struct {
	B0, B1, B2, B3 float64
	Tau1, Tau2     float64
}

// NewNSS returns a validated Nelson-Siegel-Svensson curve; betas are
// decimals, tau1 and tau2 positive and in years.
func NewNSS(b0, b1, b2, b3, tau1, tau2 float64) (NSS, error) {
	n := NSS{B0: b0, B1: b1, B2: b2, B3: b3, Tau1: tau1, Tau2: tau2}
	return n, n.Validate()
}

// Validate returns an error if a parameter is not finite or a decay
// parameter is not positive. The curve methods (ZeroRate, Discount,
// InstForward) return NaN for such invalid parameters.
func (n NSS) Validate() error {
	for i, v := range []float64{n.B0, n.B1, n.B2, n.B3, n.Tau1, n.Tau2} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("nss: parameter %d is not finite: %v", i, v)
		}
	}
	if !(n.Tau1 > 0) || !(n.Tau2 > 0) {
		return fmt.Errorf("nss: decay parameters must be positive: tau1=%v, tau2=%v", n.Tau1, n.Tau2)
	}
	return nil
}

// g returns (1 - exp(-x))/x computed in a numerically stable way (with the
// limit 1 at x = 0).
func g(x float64) float64 {
	if math.Abs(x) < 1e-6 {
		return 1 - x/2 + x*x/6
	}
	return -math.Expm1(-x) / x
}

// valid reports whether the decay parameters are positive and finite (the
// betas are checked through the result).
func (n NSS) valid() bool {
	return n.Tau1 > 0 && n.Tau2 > 0 && !math.IsInf(n.Tau1, 1) && !math.IsInf(n.Tau2, 1)
}

// checked returns r, or NaN for an invalid curve or a non-finite result.
func (n NSS) checked(r float64) float64 {
	if !n.valid() || math.IsNaN(r) || math.IsInf(r, 0) ||
		math.IsInf(n.B0, 0) || math.IsInf(n.B1, 0) || math.IsInf(n.B2, 0) || math.IsInf(n.B3, 0) {
		return math.NaN()
	}
	return r
}

// ZeroRate returns the continuously compounded zero rate (decimal) at t
// years; for t <= 0 the limit B0 + B1 is returned. It returns NaN for
// invalid parameters (see Validate).
func (n NSS) ZeroRate(t float64) float64 {
	if t < 0 {
		t = 0
	}
	x1, x2 := t/n.Tau1, t/n.Tau2
	g1, g2 := g(x1), g(x2)
	return n.checked(n.B0 + n.B1*g1 + n.B2*(g1-math.Exp(-x1)) + n.B3*(g2-math.Exp(-x2)))
}

// Discount returns the discount factor exp(-ZeroRate(t)*t).
func (n NSS) Discount(t float64) float64 {
	return math.Exp(-n.ZeroRate(t) * t)
}

// InstForward returns the analytic instantaneous forward rate (decimal)
//
//	f(t) = B0 + B1*exp(-x1) + B2*x1*exp(-x1) + B3*x2*exp(-x2)
//
// with x1 = t/Tau1 and x2 = t/Tau2; for t <= 0 the limit B0 + B1.
func (n NSS) InstForward(t float64) float64 {
	if t < 0 {
		t = 0
	}
	x1, x2 := t/n.Tau1, t/n.Tau2
	e1, e2 := math.Exp(-x1), math.Exp(-x2)
	return n.checked(n.B0 + n.B1*e1 + n.B2*x1*e1 + n.B3*x2*e2)
}

// gprime returns g'(x) = (exp(-x)(1+x) - 1)/x², computed stably.
func gprime(x float64) float64 {
	if math.Abs(x) < 1e-3 {
		return -0.5 + x/3 - x*x/8 + x*x*x/30
	}
	return (math.Expm1(-x)*(1+x) + x) / (x * x)
}

// NumParams is the number of parameters of the NSS model.
const NumParams = 6

// Params returns the parameters (B0, B1, B2, B3, Tau1, Tau2).
func (n NSS) Params() []float64 {
	return []float64{n.B0, n.B1, n.B2, n.B3, n.Tau1, n.Tau2}
}

// WithParams returns the validated NSS curve with the parameters p = (B0,
// B1, B2, B3, Tau1, Tau2).
func (n NSS) WithParams(p []float64) (Curve, error) {
	if len(p) != NumParams {
		return nil, fmt.Errorf("nss: expected %d parameters, got %d", NumParams, len(p))
	}
	return NewNSS(p[0], p[1], p[2], p[3], p[4], p[5])
}

// ZeroRateGrad writes the gradient of ZeroRate(t) with respect to (B0, B1,
// B2, B3, Tau1, Tau2) to out (len >= 6; it panics otherwise). For t <= 0
// the gradient of the limit B0 + B1 is returned.
func (n NSS) ZeroRateGrad(t float64, out []float64) {
	_ = out[NumParams-1]
	if t < 0 {
		t = 0
	}
	x1, x2 := t/n.Tau1, t/n.Tau2
	g1, g2 := g(x1), g(x2)
	e1, e2 := math.Exp(-x1), math.Exp(-x2)
	out[0] = 1
	out[1] = g1
	out[2] = g1 - e1
	out[3] = g2 - e2
	// dx/dtau = -x/tau; d(g - e)/dx = g' + e
	out[4] = ((n.B1+n.B2)*gprime(x1) + n.B2*e1) * (-x1 / n.Tau1)
	out[5] = n.B3 * (gprime(x2) + e2) * (-x2 / n.Tau2)
}

// DiscountGrad writes the gradient of Discount(t) = exp(-r(t) t) with
// respect to (B0, B1, B2, B3, Tau1, Tau2) to out (len >= 6; it panics
// otherwise): -t * Discount(t) * ZeroRateGrad(t).
func (n NSS) DiscountGrad(t float64, out []float64) {
	n.ZeroRateGrad(t, out)
	f := -t * n.Discount(t)
	for i := 0; i < NumParams; i++ {
		out[i] *= f
	}
}
