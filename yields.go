package fixedincome

import (
	"math"

	"github.com/khezen/rootfinding"
	"github.com/konimarti/fixedincome/pkg/term"
)

var (
	Precision = 6
)

// irrBrackets are the intervals (continuously compounded rates in percent)
// that are tried successively to bracket the internal rate of return
var irrBrackets = [][2]float64{
	{-20.0, 20.0},
	{-50.0, 100.0},
	{-99.0, 500.0},
}

// findRoot searches the root of f in the given intervals (in this order).
// An interval is only used if f changes its sign over it; a root exactly at
// an endpoint is returned directly. An error is returned if no interval
// brackets a root.
func findRoot(f func(float64) float64, brackets [][2]float64) (float64, error) {
	for _, bracket := range brackets {
		a, b := bracket[0], bracket[1]
		fa, fb := f(a), f(b)
		switch {
		case math.IsNaN(fa) || math.IsNaN(fb):
			continue
		case fa == 0.0:
			return a, nil
		case fb == 0.0:
			return b, nil
		case fa*fb > 0.0:
			continue
		}
		return rootfinding.Brent(f, a, b, Precision)
	}
	return 0.0, rootfinding.ErrRootIsNotBracketed
}

// Irr calculates the internal rate of return of a security as a continuously
// compounded rate in percent. The search interval starts at [-20%, 20%] and is
// widened up to [-99%, 500%] if the root is not bracketed.
func Irr(investment float64, s Security) (float64, error) {
	f := func(irr float64) float64 {
		return s.PresentValue(&term.Flat{R: irr, Spread: 0.0}) - investment
	}
	return findRoot(f, irrBrackets)
}

// spreadGetter is implemented by term structures that expose their current
// spread (in bps)
type spreadGetter interface {
	GetSpread() float64
}

// Spread calculates the implied static (zero-volatility) spread in bps.
// The spread of the term structure ts is restored before returning if ts
// implements GetSpread() float64 (as all term structures of the term package
// do); otherwise, the spread is reset to 0.
func Spread(investment float64, s Security, ts term.Structure) (float64, error) {
	original := 0.0
	if g, ok := ts.(spreadGetter); ok {
		original = g.GetSpread()
	}
	defer ts.SetSpread(original)

	f := func(spread float64) float64 {
		value := s.PresentValue(ts.SetSpread(spread))
		return value - investment
	}

	return findRoot(f, [][2]float64{{-10000.0, 10000.0}})
}

// volaGetter is implemented by options that expose their current volatility
type volaGetter interface {
	GetVola() float64
}

// ImpliedVola calculates the implied volatility for a given option price.
// On success, the volatility of the option is set to the implied volatility.
// On error, the original volatility is restored if the option implements
// GetVola() float64 (as option.European does).
func ImpliedVola(price float64, o Option, ts term.Structure) (float64, error) {
	f := func(vola float64) float64 {
		o.SetVola(vola)
		value := o.PresentValue(ts)
		return value - price
	}

	g, hasGetter := o.(volaGetter)
	original := 0.0
	if hasGetter {
		original = g.GetVola()
	}

	root, err := findRoot(f, [][2]float64{{1e-6, 1000.0}})
	if err != nil {
		if hasGetter {
			o.SetVola(original)
		}
		return root, err
	}
	o.SetVola(root)
	return root, nil
}
