package portfolio_test

import (
	"math"
	"testing"
	"time"

	"github.com/konimarti/fixedincome/pkg/cashflow"
	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/portfolio"
	"github.com/konimarti/fixedincome/pkg/term"
)

var settle = time.Date(2021, 3, 31, 0, 0, 0, 0, time.UTC)

var snb = term.NSS{B0: -0.00266372, B1: -0.00471343, B2: 0.0568789, B3: -0.0512324, Tau1: 5.74881, Tau2: 4.14426}

func mkBond(t testing.TB, years int, coupon float64) bond.Straight {
	t.Helper()
	b, err := bond.NewStraight(maturity.Schedule{Maturity: settle.AddDate(years, 0, 0), Frequency: maturity.Annual}, coupon)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// straightPos is a straight bond priced at a z-spread of 40bp.
func straightPos(t testing.TB, id string, years int, coupon, factor float64) portfolio.Position {
	t.Helper()
	b := mkBond(t, years, coupon)
	cfs, err := b.CashFlows(settle)
	if err != nil {
		t.Fatal(err)
	}
	acc, _ := b.Accrued(settle)
	return portfolio.Position{ID: id, CashFlows: cfs, Accrued: acc, Factor: factor, Spread: 0.004,
		Groups: map[string]string{"rating": map[bool]string{true: "AAA", false: "A"}[years > 8]}}
}

func callablePos(t testing.TB, factor float64) portfolio.Position {
	t.Helper()
	b := mkBond(t, 10, 0.02)
	calls, err := b.CallsOnCoupons(settle.AddDate(5, 0, 0), 100)
	if err != nil {
		t.Fatal(err)
	}
	c, err := bond.NewCallable(b, calls)
	if err != nil {
		t.Fatal(err)
	}
	return portfolio.Position{ID: "CALL", Callable: &portfolio.Callable{Bond: c, Settle: settle, MeanReversion: 0.03, Vol: 0.01},
		Factor: factor, Spread: 0.002}
}

func near(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s: got %.10g, want %.10g (tol %g)", name, got, want, tol)
	}
}

func sum(x []float64) float64 {
	s := 0.0
	for _, v := range x {
		s += v
	}
	return s
}

func mustEval(t testing.TB, p portfolio.Position, cfg portfolio.Config) portfolio.Result {
	t.Helper()
	r, err := portfolio.Evaluate(snb, p, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestKRDSumEqualsDuration(t *testing.T) {
	// cash flows before the first key (1y bond, keys from 3m) and beyond the
	// last (40y bond, last key 30y), and a zero-like short bond
	for _, keys := range [][]float64{nil, {5}, {2, 10}, {0.5, 45}} {
		for _, y := range []int{1, 3, 10, 40} {
			p := straightPos(t, "x", y, 0.02, 1)
			r := mustEval(t, p, portfolio.Config{Keys: keys})
			near(t, "sum KRD", sum(r.KRD), r.Duration, 1e-5*r.Duration)
			near(t, "sum key dv01", sum(r.KeyDV01), -r.Duration*r.Dirty*1e-4, 1e-5*math.Abs(r.Dirty*r.Duration*1e-4))
		}
	}
	// the hat bumps match cashflow.KeyRateDurations
	p := straightPos(t, "x", 7, 0.03, 1)
	keys := portfolio.DefaultKeyTenors
	want, _ := cashflow.KeyRateDurations(p.CashFlows, term.WithSpread(snb, p.Spread), keys, 1e-4)
	got := mustEval(t, p, portfolio.Config{}).KRD
	for k := range want {
		near(t, "krd", got[k], want[k], 1e-12)
	}
}

func TestLinearityAndGroups(t *testing.T) {
	cfg := portfolio.Config{Scenarios: []portfolio.Scenario{portfolio.ParallelScenario("up", 0.01)}}
	ps := []portfolio.Position{straightPos(t, "a", 3, 0.01, 2), straightPos(t, "b", 10, 0.03, -0.7), straightPos(t, "c", 5, 0.02, 1.3)}
	var rs []portfolio.Result
	for _, p := range ps {
		rs = append(rs, mustEval(t, p, cfg))
	}
	tot, err := portfolio.Aggregate(cfg, rs)
	if err != nil {
		t.Fatal(err)
	}
	for k := range tot.KRD {
		w := 0.0
		for _, r := range rs {
			w += r.Dirty * r.KRD[k]
		}
		near(t, "portfolio KRD", tot.KRD[k], w/tot.Dirty, 1e-12)
	}
	near(t, "sum KRD = D", sum(tot.KRD), tot.Duration, 1e-6*math.Abs(tot.Duration))
	// groups sum to the total
	gs, err := portfolio.GroupBy(cfg, rs, "rating")
	if err != nil || len(gs) != 2 {
		t.Fatalf("groups %v %v", gs, err)
	}
	var dirty, dv01, chg float64
	keyd := make([]float64, len(tot.KeyDV01))
	for _, g := range gs {
		dirty += g.Total.Dirty
		dv01 += g.Total.DV01
		chg += g.Total.Change[0]
		for k := range keyd {
			keyd[k] += g.Total.KeyDV01[k]
		}
	}
	near(t, "group dirty", dirty, tot.Dirty, 1e-9)
	near(t, "group dv01", dv01, tot.DV01, 1e-12)
	near(t, "group change", chg, tot.Change[0], 1e-9)
	for k := range keyd {
		near(t, "group key dv01", keyd[k], tot.KeyDV01[k], 1e-12)
	}
}

func TestLongShortNetsToZero(t *testing.T) {
	for _, mk := range []func(float64) portfolio.Position{
		func(f float64) portfolio.Position { return straightPos(t, "s", 7, 0.025, f) },
		func(f float64) portfolio.Position { return callablePos(t, f) },
	} {
		cfg := portfolio.Config{Scenarios: portfolio.IRRBB(portfolio.IRRBBParams{Parallel: 0.01, Short: 0.015, Long: 0.01})}
		long, short := mustEval(t, mk(1), cfg), mustEval(t, mk(-1), cfg)
		tot, _ := portfolio.Aggregate(cfg, []portfolio.Result{long, short})
		near(t, "dirty", tot.Dirty, 0, 1e-9)
		near(t, "dv01", tot.DV01, 0, 1e-9)
		for k := range tot.KeyDV01 {
			near(t, "key dv01", tot.KeyDV01[k], 0, 1e-9)
		}
		for i, c := range tot.Change {
			near(t, "change", c, 0, 1e-9)
			_ = i
		}
		// signs
		if !(long.Dirty > 0 && short.Dirty < 0 && long.DV01 < 0 && short.DV01 > 0 && long.Duration > 0) {
			t.Errorf("signs: long %+v short %+v", long.DV01, short.DV01)
		}
		near(t, "duration is a ratio", short.Duration, long.Duration, 1e-12)
		for k := range long.KRD {
			near(t, "krd is a ratio", short.KRD[k], long.KRD[k], 1e-12)
		}
	}
}

func TestScenarioVsApproximation(t *testing.T) {
	scen := []portfolio.Scenario{portfolio.ParallelScenario("+1", 0.0001), portfolio.ParallelScenario("-1", -0.0001),
		portfolio.ParallelScenario("+100", 0.01), portfolio.ParallelScenario("-100", -0.01)}
	cfg := portfolio.Config{Scenarios: scen}
	for _, p := range []portfolio.Position{straightPos(t, "s", 10, 0.02, 1), callablePos(t, 1)} {
		r := mustEval(t, p, cfg)
		tot, _ := portfolio.Aggregate(cfg, []portfolio.Result{r})
		for i := 0; i < 2; i++ {
			near(t, p.ID+" small shock", tot.Change[i], tot.Approx(scen[i]), 0.02*math.Abs(tot.Change[i])) // callable: effective D is a 50bp difference
		}
		// asymmetry: up + down = convexity term (> 0 for positive convexity)
		asym := tot.Change[2] + tot.Change[3]
		if (asym > 0) != (tot.Convexity > 0) {
			t.Errorf("%s: asymmetry %v with convexity %v", p.ID, asym, tot.Convexity)
		}
		// second order approximation of the large shock
		near(t, p.ID+" 100bp", tot.Change[2], tot.Approx(scen[2]), 0.1*math.Abs(tot.Change[2]))
	}
	// short flips the sign of the change
	l, s := mustEval(t, straightPos(t, "s", 10, 0.02, 1), cfg), mustEval(t, straightPos(t, "s", 10, 0.02, -1), cfg)
	for i := range scen {
		near(t, "short change", s.Change[i], -l.Change[i], 1e-12)
	}
}

func TestKRDParallelScenarioConsistency(t *testing.T) {
	// the parallel shock equals a sum of the key shocks to first order
	p := straightPos(t, "s", 12, 0.02, 1)
	cfg := portfolio.Config{Scenarios: []portfolio.Scenario{portfolio.ParallelScenario("+1bp", 1e-4)}}
	r := mustEval(t, p, cfg)
	near(t, "sum key dv01 vs change", sum(r.KeyDV01), r.Change[0], 1e-3*math.Abs(r.Change[0]))
	near(t, "dv01 vs change", r.DV01, r.Change[0], 1e-5*math.Abs(r.Dirty*r.Duration*1e-4))
}

func TestCallableKRD(t *testing.T) {
	p := callablePos(t, 1)
	r := mustEval(t, p, portfolio.Config{})
	t.Logf("effective duration %.4f, sum KRD %.4f", r.Duration, sum(r.KRD))
	near(t, "callable sum KRD", sum(r.KRD), r.Duration, 0.01*r.Duration)
	if !(r.Duration > 0) || r.DV01 >= 0 {
		t.Errorf("callable measures %+v", r)
	}
	for _, bump := range []float64{1e-4, 2e-4, 5e-4, 10e-4, 25e-4, 50e-4} {
		rr := mustEval(t, p, portfolio.Config{CallableBump: bump})
		t.Logf("bump %.1fbp: sum KRD %.4f", bump*1e4, sum(rr.KRD))
	}
	// key rates beyond the call date carry less than the straight bond
	s := mustEval(t, straightPos(t, "s", 10, 0.02, 1), portfolio.Config{})
	if !(r.Duration < s.Duration) {
		t.Errorf("callable duration %v >= straight %v", r.Duration, s.Duration)
	}
}

func TestIRRBBShapes(t *testing.T) {
	sc := portfolio.IRRBB(portfolio.IRRBBParams{Parallel: 0.01, Short: 0.02, Long: 0.01})
	if len(sc) != 6 {
		t.Fatalf("%d scenarios", len(sc))
	}
	by := map[string]portfolio.Scenario{}
	for _, s := range sc {
		by[s.Name] = s
	}
	near(t, "up", by["parallel_up"].Shift(3), 0.01, 0)
	near(t, "down", by["parallel_down"].Shift(3), -0.01, 0)
	near(t, "short_up(0)", by["short_up"].Shift(0), 0.02, 1e-15)
	near(t, "steepener(0)", by["steepener"].Shift(0), -0.65*0.02, 1e-15)
	near(t, "steepener(inf)", by["steepener"].Shift(1e3), 0.9*0.01, 1e-12)
	near(t, "flattener(0)", by["flattener"].Shift(0), 0.8*0.02, 1e-15)
	near(t, "flattener(inf)", by["flattener"].Shift(1e3), -0.6*0.01, 1e-12)
	// shifted curve
	c := portfolio.Shift(snb, by["short_up"].Shift)
	near(t, "zero", c.ZeroRate(2), snb.ZeroRate(2)+by["short_up"].Shift(2), 1e-15)
	near(t, "discount", c.Discount(2), math.Exp(-c.ZeroRate(2)*2), 1e-14)
}

func TestErrors(t *testing.T) {
	if _, err := portfolio.Evaluate(snb, portfolio.Position{ID: "x", Factor: 0}, portfolio.Config{}); err == nil {
		t.Error("zero factor")
	}
	if _, err := portfolio.Evaluate(snb, portfolio.Position{ID: "x", Factor: 1}, portfolio.Config{}); err == nil {
		t.Error("no cash flows")
	}
	p := straightPos(t, "x", 3, 0.01, 1)
	if _, err := portfolio.Evaluate(snb, p, portfolio.Config{Keys: []float64{2, 1}}); err == nil {
		t.Error("keys")
	}
}

func TestHedgedBookDegenerate(t *testing.T) {
	cfg := portfolio.Config{Scenarios: []portfolio.Scenario{portfolio.ParallelScenario("+1", 0.01)}}
	a := mustEval(t, straightPos(t, "a", 7, 0.02, 1), cfg)
	b := mustEval(t, straightPos(t, "b", 7, 0.02, -1), cfg)
	b.Dirty += 1e-10 // net 1e-10 on gross 2e6
	tot, _ := portfolio.Aggregate(cfg, []portfolio.Result{a, b})
	if tot.Dirty != 0 || !tot.Degenerate || tot.Duration != 0 || tot.Gross < 1 {
		t.Errorf("total %+v", tot)
	}
	// the approximation still works with a zero net value: V*D terms cancel
	near(t, "approx", tot.Approx(cfg.Scenarios[0]), 0, 1e-6)
	// a net value of 1e-3 of gross is not degenerate
	b.Dirty = -a.Dirty * 0.999
	tot, _ = portfolio.Aggregate(cfg, []portfolio.Result{a, b})
	if tot.Degenerate {
		t.Error("not degenerate")
	}
}
