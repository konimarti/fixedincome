package term

import (
	"math"

	"github.com/cnkei/gospline"
)

// Spline represents the term structure as cubic splines
type Spline struct {
	Spline gospline.Spline `json:"spline"`
	Spread float64         `json:"spread"`
}

// SetSpread sets the spread in bps
func (s *Spline) SetSpread(spread float64) Structure {
	s.Spread = spread
	return s
}

// Rate returns the continuously compounded spot rate in percent
func (s *Spline) Rate(t float64) float64 {
	//return s.Spline.At(t) + s.Spread*0.01
	return -math.Log(s.Z(t)) / t * 100.0
}

// Z returns the discount factor for the given maturity t
func (s *Spline) Z(t float64) float64 {
	//return math.Exp(-(s.Rate(t) * 0.01) * t)
	return s.Spline.At(t) * math.Exp(s.Spread*0.0001*t)
}

// NewSpline returns a new spline term structure where t are the maturities in
// increasing order with the corresponding discount factors z
func NewSpline(t, z []float64, spread float64) Structure {
	spline := Spline{
		Spline: gospline.NewCubicSpline(t, z),
		Spread: spread,
	}
	return &spline
}
