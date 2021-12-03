package swap

import (
	"fmt"

	"github.com/konimarti/bonds/pkg/maturity"
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

// InterestRate returns the swap rate, paying a fixed rate and receiving the floating rate
func InterestRate(schedule maturity.Schedule, ts term.Structure) (float64, error) {
	var sum float64
	for _, t := range schedule.M() {
		sum += ts.Z(t)
	}
	if sum == 0.0 {
		return 0.0, fmt.Errorf("sum of zero-coupon bonds across maturities is zero")
	}
	n := float64(schedule.Compounding())
	swaprate := n * ((1.0 - ts.Z(schedule.YearsToMaturity())) / sum) * 100.0
	return swaprate, nil
}
