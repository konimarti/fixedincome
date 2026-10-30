package bond_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/konimarti/fixedincome/pkg/cashflow"
	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/lattice"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/term"
)

var cbSettle = date(2021, 4, 1)

const (
	cbA     = 0.05
	cbSigma = 0.01
)

func cbStraight(t testing.TB, coupon float64) bond.Straight {
	t.Helper()
	return mustStraight(t, maturity.Schedule{Maturity: date(2031, 4, 1), Frequency: maturity.Annual}, coupon)
}

func mustCallable(t testing.TB, s bond.Straight, calls []bond.Call, opts ...bond.CallableOption) bond.Callable {
	t.Helper()
	c, err := bond.NewCallable(s, calls, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func mustValue(t testing.TB, c bond.Callable, curve term.Curve, a, sigma, oas float64, opts ...lattice.Option) float64 {
	t.Helper()
	v, err := c.Value(cbSettle, curve, a, sigma, oas, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestCallable_NoCallsIsStraight(t *testing.T) {
	s := cbStraight(t, 0.03)
	c := mustCallable(t, s, nil)
	cfs, _ := s.CashFlows(cbSettle)
	for _, a := range []float64{0, 0.05, 0.3} {
		for _, oas := range []float64{0, 0.004} {
			got := mustValue(t, c, snb20210331, a, cbSigma, oas)
			want := cashflow.PV(cfs, term.WithSpread(snb20210331, oas))
			if math.Abs(got/want-1) > 1e-9 {
				t.Errorf("a=%v oas=%v: got %.10f, want %.10f", a, oas, got, want)
			}
		}
	}
	// matured bond
	v, err := c.Value(date(2031, 4, 1), snb20210331, cbA, cbSigma, 0)
	if err != nil || v != 0 {
		t.Errorf("matured: got %v, %v", v, err)
	}
}

func TestCallable_ZeroCouponEuropeanCall(t *testing.T) {
	// zero coupon bond (redemption only) callable once at 2023-04-01 at 90
	s := mustStraight(t, maturity.Schedule{Maturity: date(2026, 4, 1), Frequency: maturity.Annual}, 0, bond.WithFace(100))
	c := mustCallable(t, s, []bond.Call{{Date: date(2023, 4, 1), Price: 90}})
	T, S := 730.0/365, 1826.0/365
	a, sigma := 0.1, 0.015
	sp := sigma * math.Sqrt((1-math.Exp(-2*a*T))/(2*a)) * (1 - math.Exp(-a*(S-T))) / a
	zT, zS := snb20210331.Discount(T), snb20210331.Discount(S)
	k := 0.9
	h := math.Log(zS/(k*zT))/sp + sp/2
	cdf := func(x float64) float64 { return 0.5 * math.Erfc(-x/math.Sqrt2) }
	want := 100 * (zS - (zS*cdf(h) - k*zT*cdf(h-sp)))
	prev := math.Inf(1)
	for _, spy := range []int{25, 50, 100, 200} {
		got := mustValue(t, c, snb20210331, a, sigma, 0, lattice.WithStepsPerYear(spy))
		e := math.Abs(got - want)
		t.Logf("spy=%d got %.6f want %.6f err %.2e", spy, got, want, e)
		if e > 1e-3 {
			t.Errorf("spy=%d: error %v", spy, e)
		}
		prev = e
	}
	if prev > 2e-4 {
		t.Errorf("not converged: %v", prev)
	}
}

func TestCallable_Monotonicity(t *testing.T) {
	s := cbStraight(t, 0.04)
	calls := func(p float64) []bond.Call {
		cs, err := s.CallsOnCoupons(date(2024, 4, 1), p)
		if err != nil {
			t.Fatal(err)
		}
		return cs
	}
	straight := mustValue(t, mustCallable(t, s, nil), snb20210331, cbA, cbSigma, 0)
	prev := straight
	for _, p := range []float64{110, 105, 100, 95} {
		v := mustValue(t, mustCallable(t, s, calls(p)), snb20210331, cbA, cbSigma, 0)
		if v > prev+1e-12 {
			t.Errorf("call price %v: value %v exceeds %v", p, v, prev)
		}
		prev = v
	}
	if !(prev < straight-1) {
		t.Errorf("callable %v not below straight %v", prev, straight)
	}
	// callable value decreases with volatility
	c := mustCallable(t, s, calls(100))
	prev = math.Inf(1)
	for _, sigma := range []float64{0.001, 0.005, 0.01, 0.02} {
		v := mustValue(t, c, snb20210331, cbA, sigma, 0)
		if v > prev+1e-12 {
			t.Errorf("sigma %v: value %v increases from %v", sigma, v, prev)
		}
		prev = v
	}
	// putable >= straight, increasing with volatility
	pcalls := mustCallable(t, s, nil, bond.WithPuts(bond.Put{Date: date(2025, 4, 1), Price: 100}, bond.Put{Date: date(2027, 4, 1), Price: 100}))
	prev = straight
	for _, sigma := range []float64{0.001, 0.01, 0.02} {
		v := mustValue(t, pcalls, snb20210331, cbA, sigma, 0)
		if v < straight-1e-9 || v < prev-1e-12 {
			t.Errorf("sigma %v: putable %v, straight %v, prev %v", sigma, v, straight, prev)
		}
		prev = v
	}
	// option value
	ov, err := c.OptionValue(cbSettle, snb20210331, cbA, cbSigma, 0)
	cv := mustValue(t, c, snb20210331, cbA, cbSigma, 0)
	if err != nil || math.Abs(ov-(straight-cv)) > 1e-9 || ov <= 0 {
		t.Errorf("option value %v, %v (straight %v, callable %v)", ov, err, straight, cv)
	}
}

func TestCallable_TinySigma(t *testing.T) {
	// deterministic rates: the issuer picks the call date with the lowest
	// present value of the remaining payments (clean call price at coupon
	// dates), the value is the minimum over all paths
	s := cbStraight(t, 0.06)
	calls, _ := s.CallsOnCoupons(date(2024, 4, 1), 100)
	c := mustCallable(t, s, calls)
	cfs, _ := s.CashFlows(cbSettle)
	best := cashflow.PV(cfs, snb20210331)
	for _, call := range calls {
		var path []cashflow.CashFlow
		for _, cf := range cfs {
			if cf.Date.After(call.Date) {
				break
			}
			path = append(path, cf)
		}
		path[len(path)-1].Amount += 100
		best = math.Min(best, cashflow.PV(path, snb20210331))
	}
	got := mustValue(t, c, snb20210331, cbA, 1e-6, 0)
	if math.Abs(got-best) > 1e-3 {
		t.Errorf("got %v, want %v", got, best)
	}
}

func TestCallable_OASRoundTrip(t *testing.T) {
	s := cbStraight(t, 0.04)
	calls, _ := s.CallsOnCoupons(date(2024, 4, 1), 100)
	for _, a := range []float64{0, 0.05} {
		c := mustCallable(t, s, calls, bond.WithPuts(bond.Put{Date: date(2023, 4, 1), Price: 98}))
		for _, oas := range []float64{-0.01, 0, 0.0123, 0.05} {
			price := mustValue(t, c, snb20210331, a, cbSigma, oas)
			got, err := c.OAS(price, cbSettle, snb20210331, a, cbSigma)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(got-oas) > 1e-8 {
				t.Errorf("a=%v: got %v, want %v", a, got, oas)
			}
		}
	}
	c := mustCallable(t, s, calls)
	if _, err := c.OAS(-5, cbSettle, snb20210331, cbA, cbSigma); !errors.Is(err, cashflow.ErrNoYield) {
		t.Errorf("negative price: got %v", err)
	}
	if _, err := c.OAS(math.NaN(), cbSettle, snb20210331, cbA, cbSigma); err == nil {
		t.Error("nan price: expected error")
	}
}

func TestCallable_EffectiveRisk(t *testing.T) {
	// deep in the money: call at 80 from 2023-04-01 on, market far above
	s := cbStraight(t, 0.06)
	calls, _ := s.CallsOnCoupons(date(2023, 4, 1), 80)
	c := mustCallable(t, s, calls)
	straight := mustCallable(t, s, nil)
	small := 0.001
	dur, err := c.EffectiveDuration(cbSettle, snb20210331, cbA, small, 0)
	if err != nil {
		t.Fatal(err)
	}
	durS, _ := straight.EffectiveDuration(cbSettle, snb20210331, cbA, cbSigma, 0)
	cfs, _ := s.CashFlows(cbSettle)
	if want := cashflow.Duration(cfs, snb20210331); math.Abs(durS-want) > 1e-2 {
		t.Errorf("straight effective duration %v, cashflow.Duration %v", durS, want)
	}
	toCall := 2.0 + 1.0/365
	if dur < 1.5 || dur > 3 || dur >= durS/2 {
		t.Errorf("deep ITM duration %v (to call %v), straight %v", dur, toCall, durS)
	}
	cx, err := c.EffectiveConvexity(cbSettle, snb20210331, cbA, cbSigma, 0)
	if err != nil || cx <= 0 {
		t.Errorf("convexity %v, %v", cx, err)
	}
	// DV01 sign and consistency with duration
	dv, err := straight.DV01(cbSettle, snb20210331, cbA, cbSigma, 0)
	if want := cashflow.DV01(cfs, snb20210331); err != nil || dv >= 0 || math.Abs(dv/want-1) > 1e-6 {
		t.Errorf("DV01 %v, %v; cashflow.DV01 %v", dv, err, want)
	}
	// negative convexity of a callable close to its call price
	s5 := cbStraight(t, 0.005)
	atm := mustCallable(t, s5, mustCalls(t, s5, date(2024, 4, 1), 100))
	cx, _ = atm.EffectiveConvexity(cbSettle, snb20210331, cbA, cbSigma, 0)
	if cx >= 0 {
		t.Errorf("callable convexity %v should be negative", cx)
	}
}

func mustCalls(t testing.TB, s bond.Straight, first time.Time, p float64) []bond.Call {
	t.Helper()
	cs, err := s.CallsOnCoupons(first, p)
	if err != nil {
		t.Fatal(err)
	}
	return cs
}

func TestCallable_Edges(t *testing.T) {
	s := cbStraight(t, 0.04)
	// call on the settlement date at 95: worth min(V, 95) = 95 as the bond is worth more
	c := mustCallable(t, s, []bond.Call{{Date: cbSettle, Price: 95}})
	if v := mustValue(t, c, snb20210331, cbA, cbSigma, 0); math.Abs(v-95) > 1e-9 {
		t.Errorf("call on settle: got %v", v)
	}
	// calls before settle are ignored
	c = mustCallable(t, s, []bond.Call{{Date: date(2019, 4, 1), Price: 50}})
	straight := mustValue(t, mustCallable(t, s, nil), snb20210331, cbA, cbSigma, 0)
	if v := mustValue(t, c, snb20210331, cbA, cbSigma, 0); math.Abs(v-straight) > 1e-9 {
		t.Errorf("past call: got %v, want %v", v, straight)
	}
	// call on a coupon date: coupon is paid in any case, strike has no accrued
	c = mustCallable(t, s, []bond.Call{{Date: date(2022, 4, 1), Price: 0.0001}})
	want := 4 * snb20210331.Discount(365.0/365)
	if v := mustValue(t, c, snb20210331, cbA, cbSigma, 0); math.Abs(v-want) > 1e-3 {
		t.Errorf("call at ~0 on a coupon date: got %v, want %v", v, want)
	}
	// call between coupons: accrued is added
	mid := mustCallable(t, s, []bond.Call{{Date: date(2022, 10, 1), Price: 0.0001}})
	acc, _ := s.Accrued(date(2022, 10, 1))
	if acc <= 0 {
		t.Fatalf("accrued %v", acc)
	}
	want = 4*snb20210331.Discount(365.0/365) + acc*snb20210331.Discount(548.0/365)
	if v := mustValue(t, mid, snb20210331, cbA, cbSigma, 0); math.Abs(v-want) > 1e-3 {
		t.Errorf("call at ~0 between coupons: got %v, want %v", v, want)
	}
	if v, err := mid.Accrued(date(2022, 10, 1)); err != nil || v != acc {
		t.Errorf("Accrued %v, %v", v, err)
	}
	// a == 0 works and sigma tiny works
	c = mustCallable(t, s, mustCalls(t, s, date(2024, 4, 1), 100))
	if v := mustValue(t, c, snb20210331, 0, cbSigma, 0); !(v > 0 && v < straight) {
		t.Errorf("a=0: value %v, straight %v", v, straight)
	}
	// calls on coupons helper
	cs := mustCalls(t, s, date(2029, 4, 1), 101)
	if len(cs) != 2 || !cs[0].Date.Equal(date(2029, 4, 1)) || !cs[1].Date.Equal(date(2030, 4, 1)) || cs[0].Price != 101 {
		t.Errorf("CallsOnCoupons: %+v", cs)
	}
	if _, err := s.CallsOnCoupons(date(2029, 4, 1), 0); err == nil {
		t.Error("CallsOnCoupons with price 0: expected error")
	}
}

func TestNewCallable_Invalid(t *testing.T) {
	s := cbStraight(t, 0.04)
	d1, d2 := date(2025, 4, 1), date(2026, 4, 1)
	tests := []struct {
		name  string
		calls []bond.Call
		opts  []bond.CallableOption
	}{
		{"call at maturity", []bond.Call{{Date: date(2031, 4, 1), Price: 100}}, nil},
		{"call after maturity", []bond.Call{{Date: date(2032, 4, 1), Price: 100}}, nil},
		{"unsorted", []bond.Call{{Date: d2, Price: 100}, {Date: d1, Price: 100}}, nil},
		{"duplicate", []bond.Call{{Date: d1, Price: 100}, {Date: d1, Price: 100}}, nil},
		{"zero price", []bond.Call{{Date: d1, Price: 0}}, nil},
		{"nan price", []bond.Call{{Date: d1, Price: math.NaN()}}, nil},
		{"no date", []bond.Call{{Price: 100}}, nil},
		{"put at maturity", nil, []bond.CallableOption{bond.WithPuts(bond.Put{Date: date(2031, 4, 1), Price: 100})}},
		{"put price", nil, []bond.CallableOption{bond.WithPuts(bond.Put{Date: d1, Price: -1})}},
		{"call and put date", []bond.Call{{Date: d1, Price: 100}}, []bond.CallableOption{bond.WithPuts(bond.Put{Date: d1, Price: 99})}},
	}
	for _, tt := range tests {
		if _, err := bond.NewCallable(s, tt.calls, tt.opts...); err == nil {
			t.Errorf("%s: expected error", tt.name)
		}
	}
	if _, err := bond.NewCallable(bond.Straight{}, nil); err == nil {
		t.Error("invalid bond: expected error")
	}
	c := mustCallable(t, s, nil)
	if _, err := c.Value(cbSettle, snb20210331, -1, 0.01, 0); err == nil {
		t.Error("negative a: expected error")
	}
	if _, err := c.Value(cbSettle, snb20210331, 0.1, 0, 0); err == nil {
		t.Error("zero sigma: expected error")
	}
	if _, err := c.Value(time.Time{}, snb20210331, 0.1, 0.01, 0); err == nil {
		t.Error("no settle: expected error")
	}
}

func TestCallable_YieldToWorst(t *testing.T) {
	s := cbStraight(t, 0.06)
	c := mustCallable(t, s, mustCalls(t, s, date(2024, 4, 1), 100))
	cfs, _ := s.CashFlows(cbSettle)
	// premium bond: worst is the first call
	dirty := 112.0
	y, when, err := c.YieldToWorst(dirty, cbSettle)
	if err != nil {
		t.Fatal(err)
	}
	if !when.Equal(date(2024, 4, 1)) {
		t.Errorf("worst date %v", when)
	}
	ytm, _ := cashflow.YieldCC(dirty, cfs)
	if !(y < ytm) {
		t.Errorf("yield to worst %v not below YTM %v", y, ytm)
	}
	// check against direct computation
	path := cfs[:3]
	path[2].Amount += 100
	if want, _ := cashflow.YieldCC(dirty, path); math.Abs(y-want) > 1e-12 {
		t.Errorf("got %v, want %v", y, want)
	}
	// discount bond: worst is maturity
	y, when, err = c.YieldToWorst(90, cbSettle)
	if err != nil || !when.Equal(date(2031, 4, 1)) {
		t.Errorf("discount bond: %v %v %v", y, when, err)
	}
}

func TestCallable_BumpEqualsRecalibration(t *testing.T) {
	s := cbStraight(t, 0.005)
	c := mustCallable(t, s, mustCalls(t, s, date(2024, 4, 1), 100), bond.WithPuts(bond.Put{Date: date(2023, 4, 1), Price: 97}))
	oas := 0.002
	const h = 5e-3
	vn := mustValue(t, c, term.WithSpread(snb20210331, -h), cbA, cbSigma, oas)
	v0 := mustValue(t, c, snb20210331, cbA, cbSigma, oas)
	vu := mustValue(t, c, term.WithSpread(snb20210331, h), cbA, cbSigma, oas)
	dur, err := c.EffectiveDuration(cbSettle, snb20210331, cbA, cbSigma, oas)
	if err != nil {
		t.Fatal(err)
	}
	if want := (vn - vu) / (2 * h * v0); math.Abs(dur-want) > 1e-9 {
		t.Errorf("duration %v, recalibrated %v", dur, want)
	}
	cx, _ := c.EffectiveConvexity(cbSettle, snb20210331, cbA, cbSigma, oas)
	if want := (vn + vu - 2*v0) / (h * h * v0); math.Abs(cx-want) > 1e-6*math.Abs(want) {
		t.Errorf("convexity %v, recalibrated %v", cx, want)
	}
	dv, _ := c.DV01(cbSettle, snb20210331, cbA, cbSigma, oas)
	if want := mustValue(t, c, term.WithSpread(snb20210331, 1e-4), cbA, cbSigma, oas) - v0; math.Abs(dv-want) > 1e-9 {
		t.Errorf("DV01 %v, recalibrated %v", dv, want)
	}
}

func TestCallable_NilCurve(t *testing.T) {
	s := cbStraight(t, 0.04)
	c := mustCallable(t, s, mustCalls(t, s, date(2024, 4, 1), 100))
	if _, err := c.Value(cbSettle, nil, cbA, cbSigma, 0); err == nil {
		t.Error("Value")
	}
	if _, err := c.OAS(100, cbSettle, nil, cbA, cbSigma); err == nil {
		t.Error("OAS")
	}
	if _, err := c.EffectiveDuration(cbSettle, nil, cbA, cbSigma, 0); err == nil {
		t.Error("EffectiveDuration")
	}
	if _, err := c.EffectiveConvexity(cbSettle, nil, cbA, cbSigma, 0); err == nil {
		t.Error("EffectiveConvexity")
	}
	if _, err := c.DV01(cbSettle, nil, cbA, cbSigma, 0); err == nil {
		t.Error("DV01")
	}
	if _, err := c.OptionValue(cbSettle, nil, cbA, cbSigma, 0); err == nil {
		t.Error("OptionValue")
	}
}
