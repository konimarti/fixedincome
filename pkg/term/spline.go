package term

import (
	"fmt"
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

// GetSpread returns the spread in bps
func (s *Spline) GetSpread() float64 {
	return s.Spread
}

// SetSpread sets the spread in bps
func (s *Spline) SetSpread(spread float64) Structure {
	s.Spread = spread
	return s
}

// Rate returns the continuously compounded spot rate in percent.
// Below the first maturity the zero rate is extrapolated flat, i.e.
// Rate(t) = Rate(t0) for t < t0 (this includes t <= 0). If the first maturity
// is not positive, the short-rate limit for t <= 0 is approximated at a small
// maturity (consistent with the Nelson-Siegel-Svensson model).
func (s *Spline) Rate(t float64) float64 {
	if s.spline == nil {
		panic("term structure is not properly initialized")
	}
	if t0 := s.Maturities[0]; t0 > 0.0 && t < t0 {
		return s.Rate(t0)
	}
	if t <= 0.0 {
		t = 1e-7
	}
	return -math.Log(s.Z(t)) / t * 100.0
}

// Z returns the discount factor for the given maturity t.
// Below the first maturity t0 > 0, the discount factor is consistent with a
// flat extrapolation of the zero rate: Z(t) = exp(-Rate(t0) * t).
func (s *Spline) Z(t float64) float64 {
	if s.spline == nil {
		panic("term structure is not properly initialized")
	}
	if t0 := s.Maturities[0]; t0 > 0.0 && t < t0 {
		return math.Exp(-s.Rate(t0) * 0.01 * t)
	}
	return s.spline.At(t) * math.Exp(-s.Spread*0.0001*t)
}

// Init sorts the maturities in increasing order and fits the cubic spline.
// It returns an error if the maturities and discount factors do not have the
// same length, if there are less than two points, if the maturities are not
// distinct (strictly increasing after sorting) or if a discount factor is not
// positive.
func (s *Spline) Init() error {
	if len(s.Maturities) != len(s.DiscountFactors) {
		return fmt.Errorf("spline: number of maturities (%d) and discount factors (%d) differ",
			len(s.Maturities), len(s.DiscountFactors))
	}
	if len(s.Maturities) < 2 {
		return fmt.Errorf("spline: at least two points are required, got %d", len(s.Maturities))
	}
	for i, t := range s.Maturities {
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return fmt.Errorf("spline: invalid maturity %v at index %d", t, i)
		}
		if z := s.DiscountFactors[i]; !(z > 0.0) || math.IsInf(z, 0) {
			return fmt.Errorf("spline: discount factor %v for maturity %v is not positive", z, t)
		}
	}
	sort.Sort(s)
	for i := 1; i < len(s.Maturities); i++ {
		if !(s.Maturities[i] > s.Maturities[i-1]) {
			return fmt.Errorf("spline: maturities are not strictly increasing (duplicate maturity %v)", s.Maturities[i])
		}
	}
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

// NewSpline returns a new spline term structure where t are the maturities
// with the corresponding discount factors z. The input slices are copied and
// sorted by maturity. NewSpline panics if the input is invalid (see Init);
// use Spline.Init directly to handle the error instead.
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

	if err := spline.Init(); err != nil {
		panic(err)
	}

	return &spline
}
