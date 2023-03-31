package term

import (
	"math"
	"sort"

	"github.com/cnkei/gospline"
)

// Spline represents the term structure as cubic splines
type Spline struct {
	spline          gospline.Spline `json:"-"`
	Maturities      []float64       `json:"maturities"`
	DiscountFactors []float64       `json:"discountfactors"`
	Spread          float64         `json:"spread"`
}

// SetSpread sets the spread in bps
func (s *Spline) SetSpread(spread float64) Structure {
	s.Spread = spread
	return s
}

// Rate returns the continuously compounded spot rate in percent
func (s *Spline) Rate(t float64) float64 {
	return -math.Log(s.Z(t)) / t * 100.0
}

// Z returns the discount factor for the given maturity t
func (s *Spline) Z(t float64) float64 {
	if s.spline == nil {
		panic("term structure is not properly initialized")
	}
	return s.spline.At(t) * math.Exp(s.Spread*0.0001*t)
}

func (s *Spline) Init() error {
	sort.Sort(s)
	s.spline = gospline.NewCubicSpline(s.Maturities, s.DiscountFactors)
	return nil
}

func (s *Spline) Len() int {
	return len(s.Maturities)
}

func (s *Spline) Less(i, j int) bool {
	return s.Maturities[i] < s.Maturities[j]
}

func (s *Spline) Swap(i, j int) {
	swap(s.Maturities, i, j)
	swap(s.DiscountFactors, i, j)
}

func swap(arr []float64, i, j int) {
	tmp := arr[j]
	arr[j] = arr[i]
	arr[i] = tmp
}

// NewSpline returns a new spline term structure where t are the maturities in
// increasing order with the corresponding discount factors z
func NewSpline(t, z []float64, spread float64) Structure {
	maturities := make([]float64, len(t))
	copy(maturities, t)

	factors := make([]float64, len(z))
	copy(factors, z)

	spline := Spline{
		Maturities:      maturities,
		DiscountFactors: factors,
		Spread:          spread,
	}

	spline.Init()

	return &spline
}
