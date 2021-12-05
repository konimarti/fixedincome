package swap

import (
	"fmt"

	"github.com/konimarti/bonds/pkg/term"
)

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

// InterestRate returns the swap rate. The swap rate c is given by the number that makes
// the value of the swap V(0;c,T) equal to zero at initiation.
func InterestRate(maturities []float64, compounding int, ts term.Structure) (float64, error) {
	var sum float64
	for _, t := range maturities {
		sum += ts.Z(t)
	}
	if sum == 0.0 {
		return 0.0, fmt.Errorf("sum of zero-coupon bonds across maturities is zero")
	}
	swaprate := float64(compounding) * ((1.0 - ts.Z(maturities[len(maturities)-1])) / sum) * 100.0
	return swaprate, nil
}
