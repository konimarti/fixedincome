package term

import (
	"math"

	"github.com/cnkei/gospline"
)

// Spline represents the term structure as cubic splines
type Spline struct {
	Spline gospline.Spline
	Spread float64
}

// SetSpread sets the spread in bps
func (s *Spline) SetSpread(spread float64) Structure {
	s.Spread = spread
	return s
}

// Rate returns the continuously compounded spot rate in percent
func (s *Spline) Rate(t float64) float64 {
	return s.Spline.At(t) + s.Spread*0.01
}

// Z returns the discount factor for the given maturity t
func (s *Spline) Z(t float64) float64 {
	return math.Exp(-(s.Rate(t) * 0.01) * t)
}

// NewSpline returns a new spline term structure where x are the maturities in
// increasing order with the corresponding continuously compounded rates in y
func NewSpline(x, y []float64, spread float64) Structure {
	spline := Spline{
		Spline: gospline.NewCubicSpline(x, y),
		Spread: spread,
	}
	return &spline
}
