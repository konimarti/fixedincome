package term

import "math"

// ConstantRate is an implementation of the TermStructure interface for constant spot rates
type ConstantRate struct {
	R      float64
	Spread float64
}

// func (c *ConstantRate) Forward(m, t float64) float64 {
// 	return c.R
// }

// SetSpread sets the spread in bps
func (c *ConstantRate) SetSpread(spread float64) Structure {
	c.Spread = spread
	return c
}

// Rate returns the annual spot rate (spread not considered)
func (c *ConstantRate) Rate(m float64) float64 {
	return c.R
}

// Z returns the discount factor for the given maturity m, spread in bps and compounding frequency
func (c *ConstantRate) Z(m float64, compounding int) float64 {
	return math.Pow((1.0 + (c.R*0.01+c.Spread*0.0001)/float64(compounding)), -m*float64(compounding))
}
