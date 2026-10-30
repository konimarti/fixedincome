package term

import (
	"fmt"
	"math"
)

// Spline is a term structure given by a natural cubic spline through the
// log-discount factors y(t) = -ln Z(t) = r(t)*t at a set of knots. The spline
// is always anchored at (t, Z) = (0, 1).
//
// Interpolating the log-discount factors (instead of the discount factors)
// guarantees positive discount factors and a continuous, smooth
// instantaneous forward rate f(t) = y'(t). Beyond the last knot the
// instantaneous forward rate is extrapolated flat at its value at the last
// knot, f(t) = f(t_n), which continues the spline with continuous first and
// second derivatives (natural boundary condition y”(t_n) = 0). For t < 0 the
// log-discount factor is extrapolated linearly with the forward rate at 0.
//
// A Spline is immutable and safe for concurrent use; Discount and ZeroRate do
// not allocate. Construct it with NewSpline; the methods of a zero-value or
// nil Spline return NaN.
type Spline struct {
	// x are the knots including the anchor at 0; y the log-discount factors
	x, y []float64
	// cubic coefficients per segment i on [x[i], x[i+1]]:
	// y(t) = y[i] + b[i]*dt + c[i]*dt^2 + d[i]*dt^3 with dt = t - x[i]
	b, c, d []float64
	// fn is the instantaneous forward at the last knot
	fn float64
	// anchored is true if the caller supplied the knot at t=0
	anchored bool
	// z are the discount factors as supplied by the caller
	z []float64
}

// NewSpline returns a cubic spline term structure through the discount
// factors z at the maturities t (in years). The maturities must be finite,
// non-negative and strictly increasing with a spacing of at least
// MinKnotSpacing (including the anchor at 0); the discount factors must be finite
// and positive. A knot at t = 0 is optional and must have a discount factor
// of 1; if it is absent, the anchor (0, 1) is inserted. At least one knot is
// required. The input slices are copied.
func NewSpline(t, z []float64) (*Spline, error) {
	if len(t) != len(z) {
		return nil, fmt.Errorf("spline: number of maturities (%d) and discount factors (%d) differ", len(t), len(z))
	}
	if len(t) == 0 {
		return nil, fmt.Errorf("spline: at least one knot is required")
	}
	s := &Spline{}
	n := len(t)
	if t[0] != 0 {
		n++
	} else {
		s.anchored = true
	}
	s.x = make([]float64, 0, n)
	s.y = make([]float64, 0, n)
	if !s.anchored {
		s.x = append(s.x, 0)
		s.y = append(s.y, 0)
	}
	for i := range t {
		ti, zi := t[i], z[i]
		if math.IsNaN(ti) || math.IsInf(ti, 0) || ti < 0 {
			return nil, fmt.Errorf("spline: invalid maturity %v at index %d: must be finite and non-negative", ti, i)
		}
		if !(zi > 0) || math.IsInf(zi, 0) {
			return nil, fmt.Errorf("spline: invalid discount factor %v for maturity %v: must be finite and positive", zi, ti)
		}
		if ti == 0 && math.Abs(zi-1) > 1e-12 {
			return nil, fmt.Errorf("spline: discount factor at maturity 0 must be 1, got %v", zi)
		}
		if i > 0 && !(ti > t[i-1]) {
			return nil, fmt.Errorf("spline: maturities are not strictly increasing at index %d (%v after %v)", i, ti, t[i-1])
		}
		prev := 0.0
		if i > 0 {
			prev = t[i-1]
		}
		if ti > 0 && ti-prev < MinKnotSpacing {
			return nil, fmt.Errorf("spline: knots %v and %v are closer than %v years", prev, ti, MinKnotSpacing)
		}
		if ti == 0 {
			s.x = append(s.x, 0)
			s.y = append(s.y, 0)
			continue
		}
		s.x = append(s.x, ti)
		s.y = append(s.y, -math.Log(zi))
	}
	s.z = append([]float64(nil), z...)
	s.fit()
	return s, nil
}

// MinKnotSpacing is the minimum distance between knots in years.
const MinKnotSpacing = 1e-8

// ok reports whether the spline was constructed by NewSpline.
func (s *Spline) ok() bool {
	return s != nil && len(s.b) > 0
}

// fit computes the coefficients of the natural cubic spline.
func (s *Spline) fit() {
	n := len(s.x) - 1 // number of segments (>= 1)
	h := make([]float64, n)
	for i := 0; i < n; i++ {
		h[i] = s.x[i+1] - s.x[i]
	}
	// second derivatives m with m[0] = m[n] = 0 (tridiagonal system,
	// Thomas algorithm)
	m := make([]float64, n+1)
	if n > 1 {
		diag := make([]float64, n+1)
		rhs := make([]float64, n+1)
		for i := 1; i < n; i++ {
			diag[i] = 2 * (h[i-1] + h[i])
			rhs[i] = 6 * ((s.y[i+1]-s.y[i])/h[i] - (s.y[i]-s.y[i-1])/h[i-1])
		}
		// forward elimination
		for i := 2; i < n; i++ {
			w := h[i-1] / diag[i-1]
			diag[i] -= w * h[i-1]
			rhs[i] -= w * rhs[i-1]
		}
		// back substitution
		for i := n - 1; i >= 1; i-- {
			m[i] = (rhs[i] - h[i]*m[i+1]) / diag[i]
		}
	}
	s.b = make([]float64, n)
	s.c = make([]float64, n)
	s.d = make([]float64, n)
	for i := 0; i < n; i++ {
		s.b[i] = (s.y[i+1]-s.y[i])/h[i] - h[i]*(2*m[i]+m[i+1])/6
		s.c[i] = m[i] / 2
		s.d[i] = (m[i+1] - m[i]) / (6 * h[i])
	}
	last := n - 1
	s.fn = s.b[last] + 2*s.c[last]*h[last] + 3*s.d[last]*h[last]*h[last]
}

// segment returns the index i of the segment [x[i], x[i+1]] containing t,
// for 0 <= t <= x[n] (binary search).
func (s *Spline) segment(t float64) int {
	lo, hi := 0, len(s.x)-2
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if s.x[mid] <= t {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// logDiscount returns y(t) = -ln Z(t).
func (s *Spline) logDiscount(t float64) float64 {
	if !s.ok() {
		return math.NaN()
	}
	last := len(s.x) - 1
	switch {
	case t <= 0:
		return s.b[0] * t
	case t >= s.x[last]:
		return s.y[last] + s.fn*(t-s.x[last])
	}
	i := s.segment(t)
	dt := t - s.x[i]
	return s.y[i] + dt*(s.b[i]+dt*(s.c[i]+dt*s.d[i]))
}

// Discount returns the discount factor Z(0,t) = exp(-y(t)).
func (s *Spline) Discount(t float64) float64 {
	return math.Exp(-s.logDiscount(t))
}

// ZeroRate returns the continuously compounded zero rate y(t)/t (decimal);
// for t <= 0 the limit t -> 0, i.e. the instantaneous forward rate at 0.
func (s *Spline) ZeroRate(t float64) float64 {
	if !s.ok() {
		return math.NaN()
	}
	if t <= 0 {
		return s.b[0]
	}
	return s.logDiscount(t) / t
}

// InstForward returns the analytic instantaneous forward rate y'(t)
// (decimal); flat beyond the last knot and for t < 0.
func (s *Spline) InstForward(t float64) float64 {
	if !s.ok() {
		return math.NaN()
	}
	last := len(s.x) - 1
	switch {
	case t <= 0:
		return s.b[0]
	case t >= s.x[last]:
		return s.fn
	}
	i := s.segment(t)
	dt := t - s.x[i]
	return s.b[i] + dt*(2*s.c[i]+3*dt*s.d[i])
}

// Knots returns a copy of the knot maturities (in years) as passed to
// NewSpline, i.e. without the implicit anchor at t = 0 unless it was given
// explicitly.
func (s *Spline) Knots() []float64 {
	if !s.ok() {
		return nil
	}
	start := 1
	if s.anchored {
		start = 0
	}
	return append([]float64(nil), s.x[start:]...)
}

// DiscountFactors returns the discount factors at the knots returned by
// Knots.
func (s *Spline) DiscountFactors() []float64 {
	if !s.ok() {
		return nil
	}
	return append([]float64(nil), s.z...)
}
