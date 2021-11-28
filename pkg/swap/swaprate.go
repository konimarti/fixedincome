package swap

import (
	"github.com/konimarti/bonds/pkg/term"
)

type Swap struct {
}

// PresentValue returns value of the swap
func (s *Swap) PresentValue(ts term.Structure) float64 {
	panic("not implemented yet")
	return 0.0
}

// FxRate returns the swap rate which equals the current exchange rate multiplied
// by the ratio of the relative borrowing costs in the two currencies
func FxRate(currentFx float64, cashflows, maturities []float64, tsLong, tsShort term.Structure) (float64, error) {
	var short, long float64
	for i, t := range maturities {
		long += cashflows[i] * tsLong.Z(t)
		short += cashflows[i] * tsShort.Z(t)
	}
	return currentFx * short / long, nil
}
