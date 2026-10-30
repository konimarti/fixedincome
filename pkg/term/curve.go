// Package term implements spot-rate term structures (yield curves).
//
// Conventions: time t is measured in years from the curve date (ACT/365
// Fixed, see maturity.YearFraction). All rates, spreads and model parameters
// are decimals (0.02 = 2%, a spread of 0.005 = 50bp) and continuously
// compounded. Conversion to percent and basis points only happens at the I/O
// boundary (see Parse and Marshal).
//
// Curves are immutable values that are safe for concurrent use. A z-spread
// is applied with WithSpread, which returns a new curve.
package term

import "math"

// Curve is a spot-rate term structure.
type Curve interface {
	// Discount returns the discount factor Z(0,t) for a maturity of t years;
	// Z(0,0) = 1.
	Discount(t float64) float64

	// ZeroRate returns the continuously compounded zero rate r(t) (decimal)
	// with Z(0,t) = exp(-r(t)*t). For t <= 0 the short-rate limit t -> 0 is
	// returned.
	ZeroRate(t float64) float64
}

// instForwarder is implemented by curves with an analytic instantaneous
// forward rate.
type instForwarder interface {
	InstForward(t float64) float64
}

// Forward returns the continuously compounded forward rate (decimal) between
// t1 and t2 years, f(t1,t2) = ln(Z(t1)/Z(t2)) / (t2-t1). The result is
// symmetric in t1 and t2; for t1 == t2 the instantaneous forward rate is
// returned.
func Forward(c Curve, t1, t2 float64) float64 {
	if t1 == t2 {
		return InstForward(c, t1)
	}
	return math.Log(c.Discount(t1)/c.Discount(t2)) / (t2 - t1)
}

// InstForward returns the instantaneous forward rate f(t) = -d ln Z(t)/dt
// (continuously compounded, decimal). If the curve implements
// InstForward(t float64) float64, that analytic value is used; otherwise the
// derivative is approximated by finite differences (central differences for
// t >= h, a second-order one-sided difference close to t = 0).
func InstForward(c Curve, t float64) float64 {
	if f, ok := c.(instForwarder); ok {
		return f.InstForward(t)
	}
	if t < 0 {
		t = 0
	}
	y := func(x float64) float64 { return -math.Log(c.Discount(x)) }
	h := 1e-5 * math.Max(1.0, t)
	if t >= h {
		return (y(t+h) - y(t-h)) / (2 * h)
	}
	return (-3*y(t) + 4*y(t+h) - y(t+2*h)) / (2 * h)
}

// spreadCurve is a base curve with a constant continuously compounded
// z-spread s.
type spreadCurve struct {
	base Curve
	s    float64
}

// Discount returns base.Discount(t) * exp(-s*t).
func (c spreadCurve) Discount(t float64) float64 {
	return c.base.Discount(t) * math.Exp(-c.s*t)
}

// ZeroRate returns base.ZeroRate(t) + s.
func (c spreadCurve) ZeroRate(t float64) float64 {
	return c.base.ZeroRate(t) + c.s
}

// InstForward returns the instantaneous forward of the base curve plus s.
func (c spreadCurve) InstForward(t float64) float64 {
	return InstForward(c.base, t) + c.s
}

// WithSpread returns the curve c shifted by the constant continuously
// compounded z-spread s (decimal, 0.005 = 50bp): Discount(t) =
// c.Discount(t)*exp(-s*t) and ZeroRate(t) = c.ZeroRate(t) + s. The curve c is
// not modified. Nested spreads collapse: WithSpread(WithSpread(c, a), b) has
// the base curve c and the spread a+b. A total spread with |s| < 1e-15
// (rounding residual) returns the base curve itself. WithSpread panics if c
// is nil (programmer error).
func WithSpread(c Curve, s float64) Curve {
	if c == nil {
		panic("term: WithSpread of a nil curve")
	}
	base, s0 := Spread(c)
	if total := s0 + s; math.Abs(total) >= 1e-15 || math.IsNaN(total) {
		return spreadCurve{base: base, s: total}
	}
	return base
}

// DiscountGrid writes the discount factors c.Discount(ts[i]) to out[i]. It
// panics if out is shorter than ts (programmer error).
func DiscountGrid(c Curve, ts, out []float64) {
	_ = out[:len(ts)]
	for i, t := range ts {
		out[i] = c.Discount(t)
	}
}

// Parametric is a curve given by a parameter vector (e.g. NSS), as used by
// curve fitting.
type Parametric interface {
	Curve
	// Params returns a copy of the parameters
	Params() []float64
	// WithParams returns the (validated) curve with the given parameters
	WithParams(p []float64) (Curve, error)
	// ZeroRateGrad writes the gradient of ZeroRate(t) with respect to the
	// parameters to out
	ZeroRateGrad(t float64, out []float64)
	// DiscountGrad writes the gradient of Discount(t) with respect to the
	// parameters to out
	DiscountGrad(t float64, out []float64)
}

var _ Parametric = NSS{}

// Spread unwraps a curve created by WithSpread and returns its base curve and
// spread (decimal). For any other curve it returns c and 0.
func Spread(c Curve) (base Curve, s float64) {
	if sc, ok := c.(spreadCurve); ok {
		return sc.base, sc.s
	}
	return c, 0
}
