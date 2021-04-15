package bonds_test

import (
	"math"
	"testing"
	"time"

	"github.com/konimarti/bonds"
)

func TestFixedCouponBondsPricing(t *testing.T) {

	// bond details
	// ISIN CH0224396983 (quote per 2021-04-01)
	bond := bonds.FixedCouponBond{
		Schedule: bonds.Maturities{
			QuoteDate:    time.Date(2021, 4, 1, 0, 0, 0, 0, time.UTC),
			MaturityDate: time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC),
			Frequency:    1,
		},
		RedemptionValue: 100.0,
		CouponRate:      1.25,
	}

	// term structure (parameters per 2021-04-01 for CH govt bonds)
	term := bonds.NelsonSiegelSvensson{
		-0.266372,
		-0.471343,
		5.68789,
		-5.12324,
		5.74881,
		4.14426,
	}

	_, clean := bond.Pricing(0.0, &term)

	// fmt.Println("dirty bond price=", dirty)
	// fmt.Println("accrued interest=", bond.Accrued())
	// fmt.Println("clean bond price=", clean)
	// fmt.Println("quoted price    = 109.70")

	expected := 109.70
	if math.Abs(clean-expected) > 0.05 {
		t.Errorf("got %f, expected %f", clean, expected)
	}
}
