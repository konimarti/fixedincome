package swap

import (
	"fmt"

	"github.com/konimarti/fixedincome/pkg/term"
)

// FxRate returns the fixed exchange rate K of a plain vanilla currency swap
// that exchanges the cash flows (amounts in foreign currency) at the times
// (years) at the rate K: the current exchange rate currentFx (domestic per
// foreign) multiplied by the ratio of the present values of the cash flows
// in the two currencies, currentFx * PV_foreign / PV_domestic.
func FxRate(currentFx float64, cashflows, times []float64, domestic, foreign term.Curve) (float64, error) {
	if len(cashflows) != len(times) {
		return 0.0, fmt.Errorf("swap: number of cash flows (%d) and times (%d) differ", len(cashflows), len(times))
	}
	var pvForeign, pvDomestic float64
	for i, t := range times {
		pvDomestic += cashflows[i] * domestic.Discount(t)
		pvForeign += cashflows[i] * foreign.Discount(t)
	}
	if pvDomestic == 0 {
		return 0.0, fmt.Errorf("swap: present value of the cash flows is zero")
	}
	return currentFx * pvForeign / pvDomestic, nil
}

// ParRateFromTimes returns the par swap rate (decimal) for fixed payments n
// times per year at the given payment times (years, in any order) on a
// single curve: n * (1 - Z(T)) / sum(Z(t_i)), with T the largest time. It
// assumes regular periods of 1/n years starting today.
func ParRateFromTimes(times []float64, n int, c term.Curve) (float64, error) {
	if len(times) == 0 {
		return 0.0, fmt.Errorf("swap: no payment times given")
	}
	if n <= 0 {
		return 0.0, fmt.Errorf("swap: invalid payment frequency %d", n)
	}
	var sum float64
	last := times[0]
	for _, t := range times {
		sum += c.Discount(t)
		if t > last {
			last = t
		}
	}
	if sum == 0.0 {
		return 0.0, fmt.Errorf("swap: sum of discount factors is zero")
	}
	return float64(n) * (1.0 - c.Discount(last)) / sum, nil
}
