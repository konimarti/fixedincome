package bonds

type TermStructure interface {
	// annual forward rate for the given maturity
	// Forward(m float64, t float64) float64
	// annual spot rate for the given maturity
	Rate(m float64) float64
	// discount factor for the given maturity, with zero-volatility spread and payment frequency
	Z(m float64, spread float64, compounding int) float64
}
