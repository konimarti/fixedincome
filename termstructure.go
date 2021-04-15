package bonds

type TermStructure interface {
	// annual forward rate for the given maturity
	Forward(m float64, t float64) float64
	// annual spot rate for the given maturity
	Rate(m float64) float64
	// discount factor for the given maturity, with zero-volatility spread and payment frequency
	D(m float64, zspread float64, freq int) float64
}
