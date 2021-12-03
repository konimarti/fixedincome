package swap_test

import (
	"math"
	"testing"
	"time"

	"github.com/konimarti/bonds/pkg/bond"
	"github.com/konimarti/bonds/pkg/maturity"
	"github.com/konimarti/bonds/pkg/rate"
	"github.com/konimarti/bonds/pkg/swap"
	"github.com/konimarti/bonds/pkg/term"
)

func TestSwapValue(t *testing.T) {
	// Parameters for CHF yield curve at Nov-30-2021
	// added 9bps z-spread for counterparty risk
	term := term.NelsonSiegelSvensson{
		-0.43381,
		-0.308942,
		4.83643,
		-4.10991,
		4.65211,
		3.33637,
		9.0,
	}

	maturities := []int{
		3, 4, 5, 6, 7, 8, 9, 10,
	}
	swapRate := []float64{
		-0.50, -0.42, -0.35, -0.28, -0.21, -0.15, -0.10, -0.06,
	}

	date := time.Date(2021, 12, 3, 0, 0, 0, 0, time.UTC)
	for i, m := range maturities {
		schedule := maturity.Schedule{
			Settlement: date,
			Maturity:   date.AddDate(m, 0, 0),
			Frequency:  2,
			Basis:      "30E360",
		}

		floatingLeg := bond.Floating{
			Schedule:   schedule,
			Rate:       rate.Annual(term.Rate(0.5), 2),
			Redemption: 100.0,
		}

		fixedLeg := bond.Straight{
			Schedule:   schedule,
			Coupon:     swapRate[i],
			Redemption: 100.0,
		}

		swapSecurity := swap.Swap{
			Floating: floatingLeg,
			Fixed:    fixedLeg,
		}

		value := swapSecurity.PresentValue(&term)

		if math.Abs(value) > 0.05 {
			t.Error("value of interest rate swap is wrong; maturity:", m, "got:", value, "expected: 0.0")
		}
	}

}
