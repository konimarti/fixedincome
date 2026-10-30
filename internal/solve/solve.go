// Package solve implements bracket-safe one-dimensional root finders used by
// the yield, spread and implied volatility calculations.
package solve

import (
	"errors"
	"math"
)

var (
	// ErrNotBracketed is returned if no interval brackets a root, i.e. the
	// function does not change its sign over any of the given intervals.
	ErrNotBracketed = errors.New("solve: root is not bracketed")
	// ErrNaN is returned if the function returns NaN inside the bracket.
	ErrNaN = errors.New("solve: function returned NaN")
	// ErrNoConvergence is returned if the maximum number of iterations is
	// exceeded.
	ErrNoConvergence = errors.New("solve: no convergence")
)

// DefaultTol is the default absolute tolerance in x.
const DefaultTol = 1e-12

// DefaultMaxIter is the default maximum number of iterations.
const DefaultMaxIter = 200

// Brent finds a root of f in the interval [a, b] with Brent's method. f(a)
// and f(b) must have opposite signs; if one of them is exactly zero, the
// corresponding endpoint is returned. tol is the absolute tolerance in x
// (DefaultTol if tol <= 0) and maxIter the maximum number of iterations
// (DefaultMaxIter if maxIter <= 0). An error is returned if the root is not
// bracketed, if f returns NaN or if the method does not converge.
func Brent(f func(float64) float64, a, b, tol float64, maxIter int) (float64, error) {
	if tol <= 0 {
		tol = DefaultTol
	}
	if maxIter <= 0 {
		maxIter = DefaultMaxIter
	}
	fa, fb := f(a), f(b)
	switch {
	case math.IsNaN(fa) || math.IsNaN(fb):
		return math.NaN(), ErrNaN
	case fa == 0:
		return a, nil
	case fb == 0:
		return b, nil
	case (fa > 0) == (fb > 0):
		return math.NaN(), ErrNotBracketed
	}

	// Brent's method as in Numerical Recipes (zbrent)
	c, fc := b, fb
	var d, e float64
	for i := 0; i < maxIter; i++ {
		if (fb > 0) == (fc > 0) {
			c, fc = a, fa
			d = b - a
			e = d
		}
		if math.Abs(fc) < math.Abs(fb) {
			a, b, c = b, c, b
			fa, fb, fc = fb, fc, fb
		}
		tol1 := 2*epsilon*math.Abs(b) + 0.5*tol
		xm := 0.5 * (c - b)
		if math.Abs(xm) <= tol1 || fb == 0 {
			return b, nil
		}
		if math.Abs(e) >= tol1 && math.Abs(fa) > math.Abs(fb) {
			// inverse quadratic interpolation or secant step
			s := fb / fa
			var p, q float64
			if a == c {
				p = 2 * xm * s
				q = 1 - s
			} else {
				q = fa / fc
				r := fb / fc
				p = s * (2*xm*q*(q-r) - (b-a)*(r-1))
				q = (q - 1) * (r - 1) * (s - 1)
			}
			if p > 0 {
				q = -q
			}
			p = math.Abs(p)
			min1 := 3*xm*q - math.Abs(tol1*q)
			min2 := math.Abs(e * q)
			if 2*p < math.Min(min1, min2) {
				e = d
				d = p / q
			} else {
				d = xm
				e = d
			}
		} else {
			// bisection
			d = xm
			e = d
		}
		a, fa = b, fb
		if math.Abs(d) > tol1 {
			b += d
		} else {
			b += math.Copysign(tol1, xm)
		}
		fb = f(b)
		if math.IsNaN(fb) {
			return math.NaN(), ErrNaN
		}
	}
	return b, ErrNoConvergence
}

const epsilon = 2.220446049250313e-16

// FindRoot searches a root of f in the given intervals (in this order) with
// Brent's method. An interval is skipped if f returns NaN at one of its
// endpoints or if f does not change its sign over it; a root exactly at an
// endpoint is returned directly. ErrNotBracketed is returned if no interval
// brackets a root.
func FindRoot(f func(float64) float64, brackets [][2]float64, tol float64) (float64, error) {
	for _, bracket := range brackets {
		a, b := bracket[0], bracket[1]
		fa, fb := f(a), f(b)
		switch {
		case math.IsNaN(fa) || math.IsNaN(fb):
			continue
		case fa == 0:
			return a, nil
		case fb == 0:
			return b, nil
		case (fa > 0) == (fb > 0):
			continue
		}
		return Brent(f, a, b, tol, 0)
	}
	return math.NaN(), ErrNotBracketed
}

// Newton finds a root of f with Newton's method starting at x0. fdf returns
// the function value and its derivative at x. The iteration stops
// successfully when the step is below tol (absolute, DefaultTol if tol <= 0).
// An error is returned if a value or derivative is not finite, the
// derivative vanishes, the iterate leaves [lo, hi] or the method does not
// converge in maxIter iterations (DefaultMaxIter if maxIter <= 0). Callers
// typically fall back to FindRoot on error.
func Newton(fdf func(x float64) (f, df float64), x0, lo, hi, tol float64, maxIter int) (float64, error) {
	if tol <= 0 {
		tol = DefaultTol
	}
	if maxIter <= 0 {
		maxIter = DefaultMaxIter
	}
	x := x0
	for i := 0; i < maxIter; i++ {
		f, df := fdf(x)
		if f == 0 {
			return x, nil
		}
		if math.IsNaN(f) || math.IsInf(f, 0) || math.IsNaN(df) || math.IsInf(df, 0) {
			return math.NaN(), ErrNaN
		}
		if df == 0 {
			return math.NaN(), ErrNoConvergence
		}
		dx := f / df
		x -= dx
		if !(x >= lo && x <= hi) {
			return math.NaN(), ErrNoConvergence
		}
		if math.Abs(dx) <= tol {
			return x, nil
		}
	}
	return math.NaN(), ErrNoConvergence
}
