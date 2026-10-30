package swap

import (
	"fmt"

	"github.com/konimarti/fixedincome/pkg/term"
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
// The maturities can be given in any order; the final maturity T is the
// largest one.
func InterestRate(maturities []float64, compounding int, ts term.Structure) (float64, error) {
	if len(maturities) == 0 {
		return 0.0, fmt.Errorf("no maturities given")
	}
	var sum float64
	last := maturities[0]
	for _, t := range maturities {
		sum += ts.Z(t)
		if t > last {
			last = t
		}
	}
	if sum == 0.0 {
		return 0.0, fmt.Errorf("sum of zero-coupon bonds across maturities is zero")
	}
	swaprate := float64(compounding) * ((1.0 - ts.Z(last)) / sum) * 100.0
	return swaprate, nil
}
