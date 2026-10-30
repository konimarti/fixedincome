package bond_test

import (
	"errors"
	"math"
	"testing"

	"github.com/konimarti/fixedincome/pkg/cashflow"
	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/rate"
	"github.com/konimarti/fixedincome/pkg/term"
)

var (
	floatSettle   = date(2021, 4, 1)
	floatSchedule = maturity.Schedule{
		Maturity:  floatSettle.AddDate(0, 12, 0),
		Frequency: maturity.SemiAnnual,
	}
	// settled 3 months later, inside the first coupon period
	floatTwoSettle = floatSettle.AddDate(0, 3, 0)
)

func mustFloating(t testing.TB, s maturity.Schedule, margin float64, opts ...bond.Option) bond.Floating {
	t.Helper()
	f, err := bond.NewFloating(s, margin, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func floatingTerm() term.Curve {
	r, err := rate.ToContinuous(0.02, 2)
	if err != nil {
		panic(err)
	}
	return term.Flat{Rate: r}
}

func TestFloating_PresentValue(t *testing.T) {
	ts := floatingTerm()
	fixed := mustFloating(t, floatSchedule, 0, bond.WithFixing(0.02))
	projected := mustFloating(t, floatSchedule, 0)
	testData := []struct {
		Name     string
		Bond     bond.Floating
		Settle   int
		Expected float64
	}{
		{"at reset, projected", projected, 0, 100.0},
		{"at reset, fixed", fixed, 0, 100.0},
		{"inside period", fixed, 1, 100.4975},
	}
	for _, test := range testData {
		settle := floatSettle
		if test.Settle == 1 {
			settle = floatTwoSettle
		}
		dirty, err := test.Bond.PresentValue(settle, ts, ts)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(dirty-test.Expected) > 0.01 {
			t.Errorf("%s: got %f, expected %f", test.Name, dirty, test.Expected)
		}
	}
	// exactly par at a reset date when the index is projected
	if dirty, _ := projected.PresentValue(floatSettle, ts, ts); math.Abs(dirty-100.0) > 1e-12 {
		t.Errorf("not at par on reset date: %v", dirty)
	}
	// the fixing of a period starting on the settlement date is used if set
	high := mustFloating(t, floatSchedule, 0, bond.WithFixing(0.05))
	dirty, _ := high.PresentValue(floatSettle, ts, ts)
	expected := (100 + 100*0.05*0.5) * ts.Discount(183.0/365.0)
	if math.Abs(dirty-expected) > 1e-9 {
		t.Errorf("fixing on reset date ignored: got %v, expected %v", dirty, expected)
	}
	// inside a period without fixing: error
	if _, err := projected.PresentValue(floatTwoSettle, ts, ts); !errors.Is(err, bond.ErrNoFixing) {
		t.Errorf("expected ErrNoFixing, got %v", err)
	}
	if _, err := projected.Accrued(floatTwoSettle); !errors.Is(err, bond.ErrNoFixing) {
		t.Errorf("expected ErrNoFixing from Accrued, got %v", err)
	}
}

func TestFloating_Accrued(t *testing.T) {
	f := mustFloating(t, floatSchedule, 0, bond.WithFixing(0.02))
	accrued, err := f.Accrued(floatTwoSettle)
	if err != nil {
		t.Fatal(err)
	}
	expected := 0.5
	if math.Abs(accrued-expected) > 1e-12 {
		t.Errorf("got %f, expected %f", accrued, expected)
	}
	clean, err := f.CleanPrice(floatTwoSettle, floatingTerm(), floatingTerm())
	dirty, _ := f.DirtyPrice(floatTwoSettle, floatingTerm(), floatingTerm())
	if err != nil || math.Abs(dirty-clean-0.5) > 1e-12 {
		t.Errorf("clean price %v, dirty %v, %v", clean, dirty, err)
	}
}

// TestFloating_ParAtReset checks the identity PV = (Face + current coupon) *
// Z(next reset) for forecast == discount and zero margin, and that a spread
// on the discount curve only (credit risk) lowers the value below it
func TestFloating_ParAtReset(t *testing.T) {
	s := maturity.Schedule{
		Maturity:  date(2028, 2, 15),
		Frequency: maturity.Quarterly,
		DayCount:  maturity.Act360,
	}
	frn := mustFloating(t, s, 0, bond.WithFixing(0.013), bond.WithFace(1e6))
	settle := date(2021, 4, 1)
	// current period 2021-02-15 .. 2021-05-15 (89 days)
	coupon := 1e6 * 0.013 * 89.0 / 360.0
	tNext := 44.0 / 365.0
	for _, c := range []term.Curve{snb20210331, term.WithSpread(snb20210331, 0.02), term.Flat{Rate: 0.05}} {
		pv, err := frn.PresentValue(settle, c, c)
		if err != nil {
			t.Fatal(err)
		}
		expected := (1e6 + coupon) * c.Discount(tNext)
		if math.Abs(pv-expected) > 1e-6 {
			t.Errorf("par at reset: got %v, expected %v", pv, expected)
		}
		// discount with an additional credit spread: below par-at-reset
		disc := term.WithSpread(c, 0.01)
		pvRisky, err := frn.PresentValue(settle, disc, c)
		if err != nil {
			t.Fatal(err)
		}
		if !(pvRisky < (1e6+coupon)*disc.Discount(tNext)-1000) {
			t.Errorf("risky FRN not below par-at-reset value: %v vs %v", pvRisky, (1e6+coupon)*disc.Discount(tNext))
		}
		// with a margin equal to the spread the FRN is close to par at reset
		frnMargin := frn
		frnMargin.Margin = 0.01
		pvMargin, _ := frnMargin.PresentValue(settle, disc, c)
		if math.Abs(pvMargin/((1e6+coupon+1e6*0.01*89.0/360.0)*disc.Discount(tNext))-1) > 1e-3 {
			t.Errorf("FRN with margin = spread not close to par: %v", pvMargin)
		}
	}
}

func TestFloating_DurationConvexity(t *testing.T) {
	ts := floatingTerm()
	testData := []struct {
		Name              string
		Bond              bond.Floating
		Settle            int
		ExpectedDuration  float64
		ExpectedConvexity float64
	}{
		// on a reset date with a projected index the FRN is at par for any
		// parallel shift, i.e. it has no rate risk
		{"at reset, projected", mustFloating(t, floatSchedule, 0), 0, 0.0, 0.0},
		// with the fixing known on the reset date (or one day after it)
		// the duration is close to the time to the next payment
		{"at reset, fixed", mustFloating(t, floatSchedule, 0, bond.WithFixing(0.02)), 0, 0.5, 0.25},
		{"day after reset", mustFloating(t, floatSchedule, 0, bond.WithFixing(0.02)), 2, 0.5, 0.25},
		{"inside period", mustFloating(t, floatSchedule, 0, bond.WithFixing(0.02)), 1, 0.25, 0.06},
	}

	for _, test := range testData {
		settle := floatSettle
		switch test.Settle {
		case 1:
			settle = floatTwoSettle
		case 2:
			settle = floatSettle.AddDate(0, 0, 1)
		}
		duration, err := bond.FloatingDuration(test.Bond, settle, ts, ts)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(duration-test.ExpectedDuration) > 0.01 {
			t.Errorf("%s: duration failed, got %f, expected %f", test.Name, duration, test.ExpectedDuration)
		}
		convex, err := bond.FloatingConvexity(test.Bond, settle, ts, ts)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(convex-test.ExpectedConvexity) > 0.01 {
			t.Errorf("%s: convexity failed, got %f, expected %f", test.Name, convex, test.ExpectedConvexity)
		}
	}
}

// TestFloating_Projected checks the generic analytics on projected cash flows
// (discount-curve sensitivity with the forecast held fixed)
func TestFloating_Projected(t *testing.T) {
	var p cashflow.Projected = mustFloating(t, maturity.Schedule{Maturity: date(2026, 4, 1), Frequency: 4, DayCount: maturity.Act360}, 0.002)
	cfs, err := p.CashFlows(date(2021, 4, 1), snb20210331)
	if err != nil {
		t.Fatal(err)
	}
	disc := term.WithSpread(snb20210331, 0.004)
	price := cashflow.PV(cfs, disc)
	s, err := cashflow.ZSpread(price, cfs, snb20210331)
	if err != nil || math.Abs(s-0.004) > 1e-12 {
		t.Errorf("z-spread of projected flows: %v, %v", s, err)
	}
	// spread duration ~ 5y (slightly above with the negative coupons of
	// 2021), much longer than the rate duration
	if d := cashflow.Duration(cfs, disc); d < 4.5 || d > 5.1 {
		t.Errorf("spread duration %v", d)
	}
}
