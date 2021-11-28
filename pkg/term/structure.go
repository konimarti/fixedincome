package term

// Structure implements the interface for the spot rate term structure of interest
type Structure interface {

	// Rate is the continuously compounded spot rate for the given maturity
	Rate(t float64) float64

	// Z returns the discount factor for the given maturity
	Z(t float64) float64

	// SetSpread sets the risk spread (in bps) on-top of term structure
	SetSpread(s float64) Structure
}
