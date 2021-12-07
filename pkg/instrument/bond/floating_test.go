package bond_test

import (
	"math"
	"testing"
	"time"

	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/rate"
	"github.com/konimarti/fixedincome/pkg/term"
)

var (
	date         = time.Date(2021, 4, 1, 0, 0, 0, 0, time.UTC)
	floatingBond = bond.Floating{
		Schedule: maturity.Schedule{
			Settlement: date,
			Maturity:   date.AddDate(0, 12, 0),
			Frequency:  2,
		},
		Rate:       2.00,
		Redemption: 100.0,
	}
	floatTwo = bond.Floating{
		Schedule: maturity.Schedule{
			Settlement: date.AddDate(0, 3, 0),
			Maturity:   date.AddDate(0, 12, 0),
			Frequency:  2,
		},
		Rate:       2.00,
		Redemption: 100.0,
	}
	floatingTerm = term.Flat{
		rate.Continuous(2.0, floatingBond.Schedule.Compounding()),
		0.0,
	}
)

func TestFloating_PresentValue(t *testing.T) {

	testData := []struct {
		Floating bond.Floating
		Expected float64
	}{
		{
			Floating: floatingBond,
			Expected: 100.0,
		},
		{
			Floating: floatTwo,
			Expected: 100.4975,
		},
	}

	for i, test := range testData {
		// fmt.Println("maturities=", floatingBond.Schedule.M())
		// fmt.Println("dirty bond price=", clean+bond.Accrued()
		// fmt.Println("accrued interest=", floatingBond.Accrued())
		// fmt.Println("rate=", term.Rate(0.5), term.Rate(1.0))
		// fmt.Println("clean bond price=", clean)

		// PresentValue delivers the "dirty" price
		dirty := test.Floating.PresentValue(&floatingTerm)
		expected := test.Expected
		if math.Abs(dirty-expected) > 0.01 {
			t.Errorf("test nr %d, got %f, expected %f", i, dirty, expected)
		}

	}

}

func TestFloating_Accrued(t *testing.T) {
	accrued := floatTwo.Accrued()
	expected := 0.5
	if math.Abs(accrued-expected) > 0.001 {
		t.Errorf("got %f, expected %f", accrued, expected)
	}
}

func TestFloating_DurationConvexity(t *testing.T) {

	testData := []struct {
		Floating          bond.Floating
		ExpectedDuration  float64
		ExpectedConvexity float64
	}{
		{
			Floating:          floatingBond,
			ExpectedDuration:  -0.5,
			ExpectedConvexity: 0.25,
		},
		{
			Floating:          floatTwo,
			ExpectedDuration:  -0.25,
			ExpectedConvexity: 0.06,
		},
	}

	for nr, test := range testData {

		duration := test.Floating.Duration(&floatingTerm)
		if math.Abs(duration-test.ExpectedDuration) > 0.01 {
			t.Errorf("test nr %d, duration failed, got %f, expected %f", nr, duration, test.ExpectedDuration)
		}
		convex := test.Floating.Convexity(&floatingTerm)
		if math.Abs(convex-test.ExpectedConvexity) > 0.01 {
			t.Errorf("test nr %d, convexity failed, got %f, expected %f", nr, convex, test.ExpectedConvexity)
		}
	}
}
