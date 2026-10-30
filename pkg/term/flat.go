package term

import "math"

// Flat is a flat term structure with the constant continuously compounded
// rate Rate (decimal, 0.02 = 2%) across all maturities.
type Flat struct {
	Rate float64
}

// Discount returns exp(-Rate*t).
func (f Flat) Discount(t float64) float64 {
	return math.Exp(-f.Rate * t)
}

// ZeroRate returns Rate for all maturities.
func (f Flat) ZeroRate(t float64) float64 {
	return f.Rate
}

// InstForward returns Rate for all maturities.
func (f Flat) InstForward(t float64) float64 {
	return f.Rate
}
