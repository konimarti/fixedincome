package cashflow_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/konimarti/fixedincome/pkg/cashflow"
	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/rate"
	"github.com/konimarti/fixedincome/pkg/term"
)

func date(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}

// snb20210331 are the NSS parameters for CH govt bonds as of 2021-03-31
var snb20210331 = term.NSS{
	B0: -0.00266372, B1: -0.00471343, B2: 0.0568789, B3: -0.0512324,
	Tau1: 5.74881, Tau2: 4.14426,
}

func mustStraight(t testing.TB, s maturity.Schedule, coupon float64) bond.Straight {
	t.Helper()
	b, err := bond.NewStraight(s, coupon)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mustCashFlows(t testing.TB, inst cashflow.Instrument, settle time.Time) []cashflow.CashFlow {
	t.Helper()
	cfs, err := inst.CashFlows(settle)
	if err != nil {
		t.Fatal(err)
	}
	return cfs
}

func TestYields(t *testing.T) {
	testData := []struct {
		Settle         time.Time
		B              bond.Straight
		Quote          float64
		ExpectedYield  float64 // decimal, continuously compounded
		ExpectedSpread float64 // decimal
	}{
		{
			// ISIN CH0224396983 (quote per 2021-04-01)
			Settle:         date(2021, 4, 1),
			B:              mustStraight(t, maturity.Schedule{Maturity: date(2026, 5, 28), Frequency: maturity.Annual}, 0.0125),
			Quote:          109.70,
			ExpectedYield:  -0.00574,
			ExpectedSpread: 0.2e-4,
		},
		{
			// ISIN CH0193265995 (quote per 2021-04-16)
			Settle:        date(2021, 4, 15),
			B:             mustStraight(t, maturity.Schedule{Maturity: date(2022, 9, 21), Frequency: maturity.Annual}, 0.01),
			Quote:         102.22,
			ExpectedYield: -0.0054,
			// 24.8bp with the former 30E/360 time axis (0.4333y, 1.4333y);
			// the ACT/365F times (159/365, 524/365) give 24.9bp
			ExpectedSpread: 24.9e-4,
		},
	}

	for nr, test := range testData {
		cfs := mustCashFlows(t, test.B, test.Settle)
		accrued, err := test.B.Accrued(test.Settle)
		if err != nil {
			t.Fatal(err)
		}
		dirty := test.Quote + accrued

		y, err := cashflow.YieldCC(dirty, cfs)
		if err != nil {
			t.Errorf("yield failed for test nr %d: %v", nr, err)
		}
		if math.Abs(y-test.ExpectedYield) > 0.001 {
			t.Errorf("wrong yield for test nr %d, got %f, expected %f", nr, y, test.ExpectedYield)
		}
		// annually compounded equivalent
		ya, err := cashflow.Yield(dirty, cfs, 1)
		if err != nil || math.Abs(ya-(math.Exp(y)-1)) > 1e-15 {
			t.Errorf("annual yield for test nr %d: got %v, %v", nr, ya, err)
		}

		spread, err := cashflow.ZSpread(dirty, cfs, snb20210331)
		if err != nil {
			t.Errorf("zspread failed for test nr %d: %v", nr, err)
		}
		if math.Abs(spread-test.ExpectedSpread) > 0.1e-4 {
			t.Errorf("wrong z-spread for test nr %d, got %f bp, expected %f bp", nr, spread*1e4, test.ExpectedSpread*1e4)
		}
		// reprice with the spread
		if pv := cashflow.PV(cfs, term.WithSpread(snb20210331, spread)); math.Abs(pv-dirty) > 1e-9 {
			t.Errorf("z-spread does not reprice test nr %d: %v vs %v", nr, pv, dirty)
		}
	}
}

// TestYield_RootAtBracketEndpoint checks that yields at the end of the
// initial search interval are found (and not 0)
func TestYield_RootAtBracketEndpoint(t *testing.T) {
	cfs := []cashflow.CashFlow{{T: 1.0, Amount: 100.0}}
	for _, r := range []float64{-0.2, 0.2} {
		price := cashflow.PV(cfs, term.Flat{Rate: r})
		y, err := cashflow.YieldCC(price, cfs)
		if err != nil {
			t.Errorf("rate %v: %v", r, err)
			continue
		}
		if math.Abs(y-r) > 1e-12 {
			t.Errorf("got yield %v, expected %v", y, r)
		}
	}
}

// TestZSpread_RootAtBracketEndpoint checks the same for the z-spread
func TestZSpread_RootAtBracketEndpoint(t *testing.T) {
	cfs := []cashflow.CashFlow{{T: 1.0, Amount: 100.0}}
	ts := term.Flat{Rate: 0.0}
	price := cashflow.PV(cfs, term.WithSpread(ts, 1.0))
	spread, err := cashflow.ZSpread(price, cfs, ts)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(spread-1.0) > 1e-12 {
		t.Errorf("got spread %v, expected %v", spread, 1.0)
	}
}

// TestYield_Invalid checks that an error is returned for cash flows without a
// valid present value, without cash flows and for invalid prices
func TestYield_Invalid(t *testing.T) {
	nan := []cashflow.CashFlow{{T: 1.0, Amount: math.NaN()}}
	if y, err := cashflow.YieldCC(100.0, nan); err == nil {
		t.Errorf("expected error, got yield %v", y)
	}
	if s, err := cashflow.ZSpread(100.0, nan, term.Flat{}); err == nil {
		t.Errorf("expected error, got spread %v", s)
	}
	if _, err := cashflow.YieldCC(100.0, nil); err == nil {
		t.Errorf("expected error for no cash flows")
	}
	cfs := []cashflow.CashFlow{{T: 1.0, Amount: 100.0}}
	if _, err := cashflow.YieldCC(math.NaN(), cfs); err == nil {
		t.Errorf("expected error for NaN price")
	}
	// a negative price cannot be reached by positive cash flows
	if y, err := cashflow.YieldCC(-1.0, cfs); err == nil {
		t.Errorf("expected error for negative price, got %v", y)
	}
	if _, err := cashflow.Yield(100.0, cfs, 0); err == nil {
		t.Errorf("expected error for compounding frequency 0")
	}
	// zero price: no yield (previously spurious roots from underflow)
	for _, flows := range [][]cashflow.CashFlow{
		{{T: 30, Amount: 100}},
		{{T: 0.5, Amount: 5}, {T: 1, Amount: 105}},
	} {
		if y, err := cashflow.YieldCC(0, flows); !errors.Is(err, cashflow.ErrNoYield) {
			t.Errorf("zero price: expected ErrNoYield, got %v, %v", y, err)
		}
	}
	// negative cash flows and a positive price
	if _, err := cashflow.YieldCC(10, []cashflow.CashFlow{{T: 1, Amount: -100}}); !errors.Is(err, cashflow.ErrNoYield) {
		t.Errorf("expected ErrNoYield for negative cash flows, got %v", err)
	}
	// short-position (negative) cash flows with a negative price are fine
	if y, err := cashflow.YieldCC(-100*math.Exp(-0.02), []cashflow.CashFlow{{T: 1, Amount: -100}}); err != nil || math.Abs(y-0.02) > 1e-12 {
		t.Errorf("negative flows: got %v, %v", y, err)
	}
	// a tiny positive price still has a (huge but representable) yield
	if y, err := cashflow.YieldCC(1e-300, []cashflow.CashFlow{{T: 30, Amount: 100}}); err != nil || math.Abs(y-math.Log(1e302)/30) > 1e-9 {
		t.Errorf("tiny price: got %v, %v", y, err)
	}
	if _, err := cashflow.YieldCC(100, nil); !errors.Is(err, cashflow.ErrNoCashFlows) {
		t.Errorf("expected ErrNoCashFlows, got %v", err)
	}
	// curve returning NaN
	if _, err := cashflow.ZSpread(100, cfs, term.NSS{}); err == nil {
		t.Errorf("expected error for NaN curve")
	}
	if _, err := cashflow.KeyRateDurations(nil, term.Flat{}, []float64{1, 2}, 1e-4); !errors.Is(err, cashflow.ErrNoCashFlows) {
		t.Errorf("expected ErrNoCashFlows from KeyRateDurations, got %v", err)
	}
}

// TestYield_HighYield checks that the yield is found also for very high
// yields outside of the initial bracket (distressed bonds)
func TestYield_HighYield(t *testing.T) {
	b := mustStraight(t, maturity.Schedule{Maturity: date(2026, 1, 1), Frequency: maturity.Annual}, 0.05)
	cfs := mustCashFlows(t, b, date(2021, 1, 1))

	for _, price := range []float64{40.0, 10.0, 1.0, 300.0} {
		y, err := cashflow.YieldCC(price, cfs)
		if err != nil {
			t.Errorf("price %v: %v", price, err)
			continue
		}
		// reprice independently: 5 cash flows discounted with the
		// continuously compounded yield
		pv := 0.0
		for k := 1; k <= 5; k++ {
			pv += 5.0 * math.Exp(-y*cfs[k-1].T)
		}
		pv += 100.0 * math.Exp(-y*cfs[4].T)
		if math.Abs(pv-price) > 1e-9 {
			t.Errorf("price %v: yield %v reprices to %v", price, y, pv)
		}
	}
	// very short maturity, distressed: one day to maturity at half the value
	short := []cashflow.CashFlow{{T: 1.0 / 365.0, Amount: 105.0}}
	y, err := cashflow.YieldCC(52.5, short)
	if err != nil || math.Abs(y-math.Ln2*365) > 1e-9 {
		t.Errorf("short distressed: got %v, %v, expected %v", y, err, math.Ln2*365)
	}
}

// TestZSpread_SplineVsNss checks that the z-spread of a bond is the same for
// an NSS curve and a cubic spline built from the NSS discount factors
func TestZSpread_SplineVsNss(t *testing.T) {
	nss := snb20210331

	var m, z []float64
	for i := 1; i <= 300; i++ {
		x := float64(i) * 0.05
		m = append(m, x)
		z = append(z, nss.Discount(x))
	}
	spline, err := term.NewSpline(m, z)
	if err != nil {
		t.Fatal(err)
	}

	b := mustStraight(t, maturity.Schedule{Maturity: date(2026, 5, 28), Frequency: maturity.Annual, DayCount: maturity.Thirty360E}, 0.0125)
	cfs := mustCashFlows(t, b, date(2021, 4, 1))

	// price the bond with a known spread of 50 bps on the NSS curve
	price := cashflow.PV(cfs, term.WithSpread(nss, 0.005))

	spreadNss, err := cashflow.ZSpread(price, cfs, nss)
	if err != nil {
		t.Fatal(err)
	}
	spreadSpline, err := cashflow.ZSpread(price, cfs, spline)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(spreadNss-0.005) > 1e-12 {
		t.Errorf("nss spread; got: %v, expected: %v", spreadNss, 0.005)
	}
	if math.Abs(spreadSpline-spreadNss) > 0.01e-4 {
		t.Errorf("spline spread does not agree with nss spread; got: %v, expected: %v", spreadSpline, spreadNss)
	}
}

// TestDV01 checks the DV01 against the second-order approximation with
// duration and convexity
func TestDV01(t *testing.T) {
	// zero bond
	b := mustStraight(t, maturity.Schedule{Maturity: date(2031, 1, 1), Frequency: maturity.Annual, DayCount: maturity.Thirty360E}, 0)
	cfs := mustCashFlows(t, b, date(2021, 1, 1))
	ts := term.Flat{Rate: 0.02}

	dv01 := cashflow.DV01(cfs, ts)
	approx := cashflow.Sensitivity(cfs, ts, 0.0001) * cashflow.PV(cfs, ts)
	if dv01 >= 0 {
		t.Errorf("DV01 of a long bond must be negative: %v", dv01)
	}
	if math.Abs(dv01-approx) > 1e-6 {
		t.Errorf("dv01 calculation failed; got: %v, expected: %v", dv01, approx)
	}
	// independent repricing of the zero bond
	T := 3652.0 / 365.0
	expected := 100*math.Exp(-0.0201*T) - 100*math.Exp(-0.02*T)
	if math.Abs(dv01-expected) > 1e-12 {
		t.Errorf("dv01; got %v, expected %v", dv01, expected)
	}
}

func TestDurationConvexity_FiniteDifference(t *testing.T) {
	b := mustStraight(t, maturity.Schedule{Maturity: date(2036, 5, 28), Frequency: maturity.SemiAnnual}, 0.03)
	cfs := mustCashFlows(t, b, date(2021, 4, 1))
	c := snb20210331
	h := 1e-4
	p0, pu, pd := cashflow.PV(cfs, c), cashflow.PV(cfs, term.WithSpread(c, h)), cashflow.PV(cfs, term.WithSpread(c, -h))
	if d, fd := cashflow.Duration(cfs, c), -(pu-pd)/(2*h*p0); math.Abs(d-fd) > 1e-5 {
		t.Errorf("duration %v, finite difference %v", d, fd)
	}
	if cx, fd := cashflow.Convexity(cfs, c), (pu-2*p0+pd)/(h*h*p0); math.Abs(cx-fd) > 1e-3 {
		t.Errorf("convexity %v, finite difference %v", cx, fd)
	}
}

func TestKeyRateDurations(t *testing.T) {
	b := mustStraight(t, maturity.Schedule{Maturity: date(2036, 5, 28), Frequency: maturity.SemiAnnual}, 0.03)
	cfs := mustCashFlows(t, b, date(2021, 4, 1))
	keys := []float64{0.5, 1, 2, 5, 10, 20, 30}
	for _, c := range []term.Curve{snb20210331, term.Flat{Rate: 0.04}} {
		krd, err := cashflow.KeyRateDurations(cfs, c, keys, 1e-4)
		if err != nil {
			t.Fatal(err)
		}
		sum := 0.0
		for _, k := range krd {
			sum += k
		}
		// equal up to the O(bump²) error of the central differences
		if d := cashflow.Duration(cfs, c); math.Abs(sum-d) > 1e-5 {
			t.Errorf("sum of key rate durations %v != duration %v", sum, d)
		}
		// no exposure beyond the last cash flow (15.2y)
		if krd[5] <= 0 || krd[6] != 0 {
			t.Errorf("unexpected key rate durations %v", krd)
		}
	}
	// zero bond with maturity on a key: all exposure at that key
	zero := []cashflow.CashFlow{{T: 5, Amount: 100}}
	krd, _ := cashflow.KeyRateDurations(zero, snb20210331, keys, 1e-4)
	for i, k := range krd {
		expected := 0.0
		if keys[i] == 5 {
			expected = 5
		}
		if math.Abs(k-expected) > 1e-6 {
			t.Errorf("zero bond key %v: got %v, expected %v", keys[i], k, expected)
		}
	}
	for _, bad := range [][]float64{nil, {1, 1}, {2, 1}, {math.NaN()}} {
		if _, err := cashflow.KeyRateDurations(cfs, snb20210331, bad, 1e-4); err == nil {
			t.Errorf("expected error for keys %v", bad)
		}
	}
	if _, err := cashflow.KeyRateDurations(cfs, snb20210331, keys, 0); err == nil {
		t.Errorf("expected error for bump 0")
	}
}

// TestYield_AnnualFlat checks Yield against an annually compounded flat curve
func TestYield_AnnualFlat(t *testing.T) {
	rc, _ := rate.ToContinuous(0.03, 1)
	cfs := []cashflow.CashFlow{{T: 1, Amount: 3}, {T: 2, Amount: 3}, {T: 3, Amount: 103}}
	price := cashflow.PV(cfs, term.Flat{Rate: rc})
	if math.Abs(price-100) > 1e-12 {
		t.Fatalf("par bond: %v", price)
	}
	y, err := cashflow.Yield(price, cfs, 1)
	if err != nil || math.Abs(y-0.03) > 1e-14 {
		t.Errorf("got %v, %v", y, err)
	}
}

func BenchmarkYieldCC(b *testing.B) {
	bnd := mustStraight(b, maturity.Schedule{Maturity: date(2036, 5, 28), Frequency: maturity.SemiAnnual}, 0.03)
	cfs := mustCashFlows(b, bnd, date(2021, 4, 1))
	price := cashflow.PV(cfs, snb20210331)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := cashflow.YieldCC(price, cfs); err != nil {
			b.Fatal(err)
		}
	}
}
