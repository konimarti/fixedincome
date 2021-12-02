package bond_test

import (
	"testing"
)

func TestFloatingPricing(t *testing.T) {
	//
	// // bond details
	// // ISIN CH0224396983 (quote per 2021-04-01)
	// bond := bond.Straight{
	// 	Schedule: maturity.Schedule{
	// 		Settlement: time.Date(2021, 4, 1, 0, 0, 0, 0, time.UTC),
	// 		Maturity:   time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC),
	// 		Frequency:  1,
	// 	},
	// 	Coupon:     1.25,
	// 	Redemption: 100.0,
	// }
	//
	// // term structure (parameters per 2021-04-01 for CH govt bonds)
	// term := term.NelsonSiegelSvensson{
	// 	-0.266372,
	// 	-0.471343,
	// 	5.68789,
	// 	-5.12324,
	// 	5.74881,
	// 	4.14426,
	// 	0.0, // spread
	// }
	//
	// clean := bond.PresentValue(&term)
	//
	// // fmt.Println("dirty bond price=", clean+bond.Accrued()
	// // fmt.Println("accrued interest=", bond.Accrued())
	// // fmt.Println("clean bond price=", clean)
	// // fmt.Println("quoted price    = 109.70")
	//
	// expected := 109.70
	// if math.Abs(clean-expected) > 0.05 {
	// 	t.Errorf("got %f, expected %f", clean, expected)
	// }

	t.Error("not implemented yet")
}

func TestFloating_AccruedInterest(t *testing.T) {

	// testData := []struct {
	// 	Quote     time.Time
	// 	Maturity  time.Time
	// 	Frequency int
	// 	Expected  float64
	// }{
	// 	{
	// 		Quote:     time.Date(2021, 4, 17, 0, 0, 0, 0, time.UTC),
	// 		Maturity:  time.Date(2026, 05, 28, 0, 0, 0, 0, time.UTC),
	// 		Frequency: 1,
	// 		Expected:  1.11,
	// 	},
	// 	{
	// 		Quote:     time.Date(2021, 4, 17, 0, 0, 0, 0, time.UTC),
	// 		Maturity:  time.Date(2026, 05, 28, 0, 0, 0, 0, time.UTC),
	// 		Frequency: 2,
	// 		Expected:  0.48,
	// 	},
	// }
	//
	// for nr, test := range testData {
	//
	// 	bond := bond.Straight{
	// 		Schedule: maturity.Schedule{
	// 			Settlement: test.Quote,
	// 			Maturity:   test.Maturity,
	// 			Frequency:  test.Frequency,
	// 		},
	// 		Redemption: 100.0,
	// 		Coupon:     1.25,
	// 	}
	//
	// 	accrued := math.Round(bond.Accrued()*100.0) / 100.0
	// 	if math.Abs(accrued-test.Expected) > 0.001 {
	// 		t.Errorf("test nr %d, got %f, expected %f", nr, accrued, test.Expected)
	// 	}
	//
	// }
	t.Error("not implemented yet")
}

func TestFloating_DurationConvexity(t *testing.T) {

	// testData := []struct {
	// 	Quote             time.Time
	// 	Maturity          time.Time
	// 	Coupon            float64
	// 	Frequency         int
	// 	ExpectedDuration  float64
	// 	ExpectedConvexity float64
	// }{
	// 	{
	// 		Quote:             time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
	// 		Maturity:          time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC),
	// 		Coupon:            0.0,
	// 		Frequency:         1,
	// 		ExpectedDuration:  -10.0,
	// 		ExpectedConvexity: 100.0,
	// 	},
	// }
	//
	// term := term.NelsonSiegelSvensson{
	// 	-0.266372,
	// 	-0.471343,
	// 	5.68789,
	// 	-5.12324,
	// 	5.74881,
	// 	4.14426,
	// 	0.0, // spread
	// }
	//
	// for nr, test := range testData {
	//
	// 	bond := bond.Straight{
	// 		Schedule: maturity.Schedule{
	// 			Settlement: test.Quote,
	// 			Maturity:   test.Maturity,
	// 			Frequency:  test.Frequency,
	// 		},
	// 		Redemption: 100.0,
	// 		Coupon:     test.Coupon,
	// 	}
	//
	// 	duration := bond.Duration(&term)
	// 	if math.Abs(duration-test.ExpectedDuration) > 0.01 {
	// 		t.Errorf("test nr %d, got %f, expected %f", nr, duration, test.ExpectedDuration)
	// 	}
	// 	convex := bond.Convexity(&term)
	// 	if math.Abs(convex-test.ExpectedConvexity) > 0.1 {
	// 		t.Errorf("test nr %d, got %f, expected %f", nr, convex, test.ExpectedConvexity)
	// 	}
	// }
	t.Error("not implemented yet")
}
