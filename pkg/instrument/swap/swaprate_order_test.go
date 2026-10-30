package swap_test

import (
	"math"
	"sort"
	"testing"
	"time"

	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/instrument/swap"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/term"
)

// TestInterestRate_Order checks that the swap rate does not depend on the
// order of the maturities and that a swap at the par rate has zero value
func TestInterestRate_Order(t *testing.T) {
	ts := &term.NelsonSiegelSvensson{B0: 2.0, B1: -1.0, B2: 1.0, B3: 0.5, T1: 2.0, T2: 5.0}

	schedule := maturity.Schedule{
		Settlement: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
		Maturity:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Frequency:  2,
		Basis:      "30E360",
	}

	// Schedule.M() returns the maturities in descending order
	descending := schedule.M()
	ascending := append([]float64{}, descending...)
	sort.Float64s(ascending)

	rateDesc, err := swap.InterestRate(descending, 2, ts)
	if err != nil {
		t.Fatal(err)
	}
	rateAsc, err := swap.InterestRate(ascending, 2, ts)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(rateDesc-rateAsc) > 1e-12 {
		t.Errorf("swap rate depends on order of maturities; descending: %v, ascending: %v", rateDesc, rateAsc)
	}

	// floating leg at par: the next coupon is the current 6-month rate
	floatRate := 2.0 * (1.0/ts.Z(0.5) - 1.0) * 100.0

	irs := swap.InterestRateSwap{
		Floating: bond.Floating{Schedule: schedule, Rate: floatRate, Redemption: 100.0},
		Fixed:    bond.Straight{Schedule: schedule, Coupon: rateDesc, Redemption: 100.0},
	}
	if pv := irs.PresentValue(ts); math.Abs(pv) > 1e-9 {
		t.Errorf("swap at par rate %v has non-zero value %v", rateDesc, pv)
	}
}
