package bonds_test

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/konimarti/bonds"
)

func TestYields(t *testing.T) {

	testData := []struct {
		B           bonds.Bond
		Quote       float64
		ExpectedIRR float64
		ExpectedZsp float64
	}{
		{
			// bond details
			// ISIN CH0224396983 (quote per 2021-04-01)
			B: &bonds.FixedCouponBond{
				Schedule: bonds.Maturities{
					QuoteDate:    time.Date(2021, 4, 1, 0, 0, 0, 0, time.UTC),
					MaturityDate: time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC),
					Frequency:    1,
				},
				RedemptionValue: 100.0,
				CouponRate:      1.25,
			},
			Quote:       109.70,
			ExpectedIRR: -0.574,
			ExpectedZsp: 0.0,
		},
		{
			// ISIN CH0193265995 (quote per 2021-04-16)
			B: &bonds.FixedCouponBond{
				Schedule: bonds.Maturities{
					QuoteDate:    time.Date(2021, 4, 15, 0, 0, 0, 0, time.UTC),
					MaturityDate: time.Date(2022, 9, 21, 0, 0, 0, 0, time.UTC),
					Frequency:    1,
				},
				RedemptionValue: 100.0,
				CouponRate:      1.00,
			},
			Quote:       102.22,
			ExpectedIRR: -0.54,
			ExpectedZsp: 25.00, // EUR-AA Rating for Financial Companies with Maturity 1Y 26.26 bps
		},
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

	// loop over tests
	for nr, test := range testData {

		// IRR
		irr, err := bonds.IRR(test.Quote, test.B)
		// fmt.Println(irr)
		if err != nil {
			fmt.Println(err)
			t.Errorf("irr failed for test nr %d", nr)
		}

		// fmt.Println("Remaining years:", test.B.RemainingYears())

		if math.Abs(irr-test.ExpectedIRR) > 0.05 {
			t.Errorf("wrong IRR for test nr %d, got %f, expected %f", nr, irr, test.ExpectedIRR)
		}

		// Z-Spread
		zspread, err := bonds.ZSpread(test.Quote, test.B, &term)
		// fmt.Println(zspread)
		if err != nil {
			fmt.Println(err)
			t.Errorf("zspread failed for test nr %d", nr)
		}

		if math.Abs(zspread-test.ExpectedZsp) > 1.0 {
			t.Errorf("wrong Z-Spread for test nr %d, got %f, expected %f", nr, zspread, test.ExpectedZsp)
		}
	}
}
