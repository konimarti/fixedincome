package fixedincome_test

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/konimarti/fixedincome"
	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/instrument/option"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/term"
)

func TestYields(t *testing.T) {

	testData := []struct {
		B              *bond.Straight
		Quote          float64
		ExpectedIRR    float64
		ExpectedSpread float64
	}{
		{
			// bond details
			// ISIN CH0224396983 (quote per 2021-04-01)
			B: &bond.Straight{
				Schedule: maturity.Schedule{
					Settlement: time.Date(2021, 4, 1, 0, 0, 0, 0, time.UTC),
					Maturity:   time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC),
					Frequency:  1,
				},
				Redemption: 100.0,
				Coupon:     1.25,
			},
			Quote:          109.70,
			ExpectedIRR:    -0.574,
			ExpectedSpread: 0.2,
		},
		{
			// ISIN CH0193265995 (quote per 2021-04-16)
			B: &bond.Straight{
				Schedule: maturity.Schedule{
					Settlement: time.Date(2021, 4, 15, 0, 0, 0, 0, time.UTC),
					Maturity:   time.Date(2022, 9, 21, 0, 0, 0, 0, time.UTC),
					Frequency:  1,
				},
				Redemption: 100.0,
				Coupon:     1.00,
			},
			Quote:          102.22,
			ExpectedIRR:    -0.54,
			ExpectedSpread: 24.8,
		},
	}

	// term structure (parameters per 2021-04-01 for CH govt bonds)
	term := term.NelsonSiegelSvensson{
		B0:     -0.266372,
		B1:     -0.471343,
		B2:     5.68789,
		B3:     -5.12324,
		T1:     5.74881,
		T2:     4.14426,
		Spread: 0.0,
	}

	// loop over tests
	for nr, test := range testData {

		// IRR
		irr, err := fixedincome.Irr(test.Quote+test.B.Accrued(), test.B)
		// fmt.Println(irr)
		if err != nil {
			fmt.Println(err)
			t.Errorf("irr failed for test nr %d", nr)
		}

		// fmt.Println("Remaining years:", test.B.RemainingYears())

		if math.Abs(irr-test.ExpectedIRR) > 0.1 {
			t.Errorf("wrong IRR for test nr %d, got %f, expected %f", nr, irr, test.ExpectedIRR)
		}

		// Z-Spread
		spread, err := fixedincome.Spread(test.Quote+test.B.Accrued(), test.B, &term)
		// fmt.Println(zspread)
		if err != nil {
			fmt.Println(err)
			t.Errorf("zspread failed for test nr %d", nr)
		}

		if math.Abs(spread-test.ExpectedSpread) > 0.1 {
			t.Errorf("wrong Z-Spread for test nr %d, got %f, expected %f", nr, spread, test.ExpectedSpread)
		}
	}
}

func TestImpliedVola(t *testing.T) {
	testOption := option.European{
		Type: option.Call,
		S:    110.0,
		K:    100.0,
		T:    2.0,
		Q:    0.0,
		Vola: math.Pi,
	}
	ts := term.Flat{R: 2.0, Spread: 0.0}

	expected := 0.3

	tests := []struct {
		OptionType int
		Price      float64
	}{
		{
			OptionType: option.Call,
			Price:      25.1291,
		},
		{
			OptionType: option.Put,
			Price:      11.2080,
		},
	}

	for _, test := range tests {
		testOption.Type = test.OptionType
		vola, err := fixedincome.ImpliedVola(test.Price, &testOption, &ts)
		if err != nil || math.Abs(vola-expected) > 0.0001 {
			t.Error("Type", test.OptionType, "Got", vola, "Expected", expected)
		}
	}
}

// TestImpliedVola_SetsRoot checks that the option's volatility is set to the
// implied volatility on success and restored on failure
func TestImpliedVola_SetsRoot(t *testing.T) {
	ts := term.Flat{R: 2.0, Spread: 0.0}
	opt := option.European{Type: option.Call, S: 110.0, K: 100.0, T: 2.0, Q: 0.0, Vola: 0.3}
	price := opt.PresentValue(&ts)

	opt.Vola = 0.9
	vola, err := fixedincome.ImpliedVola(price, &opt, &ts)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(vola-0.3) > 1e-6 {
		t.Errorf("wrong implied vola; got %v, expected %v", vola, 0.3)
	}
	if opt.Vola != vola {
		t.Errorf("option vola not set to the root; got %v, expected %v", opt.Vola, vola)
	}

	// price below the lower arbitrage bound S - K*Z(T): no solution
	opt.Vola = 0.9
	_, err = fixedincome.ImpliedVola(1.0, &opt, &ts)
	if err == nil {
		t.Errorf("expected error for price below arbitrage bound")
	}
	if opt.Vola != 0.9 {
		t.Errorf("option vola not restored on error; got %v, expected %v", opt.Vola, 0.9)
	}
}
