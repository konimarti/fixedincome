package bonds

import (
	"github.com/khezen/rootfinding"
	"github.com/konimarti/bonds/pkg/term"
)

var (
	Precision = 6
)

// Irr calculates the internal rate of return of a security
func Irr(investment float64, s Security) (float64, error) {
	f := func(irr float64) float64 {
		return s.PresentValue(&term.ConstantRate{irr, 0.0}) - investment
	}

	root, err := rootfinding.Brent(f, -20.0, 20.0, Precision)
	return root, err
}

// Spread calculates the implied static (zero-volatility) spread
func Spread(investment float64, s Security, ts term.Structure) (float64, error) {
	f := func(spread float64) float64 {
		value := s.PresentValue(ts.SetSpread(spread))
		return value - investment
	}

	root, err := rootfinding.Brent(f, -10.0, 10000.0, Precision)
	return root, err
}
