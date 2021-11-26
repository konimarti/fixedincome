package term

// Structure implements the interface for the spot rate term structure of interest
type Structure interface {

	// Rate is the annual spot rate for the given maturity
	Rate(m float64) float64

	// Z returns the discount factor for the given maturity, with zero-volatility spread and payment frequency
	Z(m float64, compounding int) float64

	// annual forward rate for the given maturity
	// Forward(m float64, t float64) float64

	// SetSpread sets the risk spread (in bps) on-top of term structure
	// Spread is the static (zero-volatility) annual spread in bps
	// Spread is considered in the calculation of the discount factor Z
	SetSpread(spread float64) Structure
}
