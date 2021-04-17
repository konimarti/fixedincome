package bonds

import (
	"github.com/khezen/rootfinding"
)

// IRR calculates the internal return of the bond (i.e. yield to maturity)
func IRR(quotedprice float64, b Bond) (float64, error) {
	precision := 6

	f := func(irr float64) float64 {
		_, clean := b.Pricing(0.0, &ConstantRate{irr})
		return clean - quotedprice
	}

	root, err := rootfinding.Brent(f, -20.0, 20.0, precision)
	return root, err
}

// Spread calculates the implied static (zero-volatility) spread for a given term structure
func Spread(quotedprice float64, b Bond, ts TermStructure) (float64, error) {
	precision := 6

	f := func(spread float64) float64 {
		_, clean := b.Pricing(spread, ts)
		return clean - quotedprice
	}

	root, err := rootfinding.Brent(f, -10.0, 10000.0, precision)
	return root, err
}
