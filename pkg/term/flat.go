package term

import "math"

// Flat represents a flat term structure, i.e. constant rate across maturities
type Flat struct {
	R      float64 `json:"r"`
	Spread float64 `json:"spread"`
}

// SetSpread sets the spread in bps
func (f *Flat) SetSpread(spread float64) Structure {
	f.Spread = spread
	return f
}

// Rate returns the continuously compounded spot rate in percent
func (f *Flat) Rate(t float64) float64 {
	return f.R + f.Spread*0.01
}

// Z returns the discount factor for the given maturity t
func (f *Flat) Z(t float64) float64 {
	return math.Exp(-(f.Rate(t) * 0.01) * t)
}
