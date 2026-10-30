package fit

import (
	"math"
	"math/rand"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/konimarti/fixedincome/pkg/cashflow"
	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/rate"
	"github.com/konimarti/fixedincome/pkg/term"
)

var (
	// SNB parameters as of 2021-03-31 (README), betas as decimals
	snb = term.NSS{B0: -0.00266372, B1: -0.00471343, B2: 0.0568789, B3: -0.0512324, Tau1: 5.74881, Tau2: 4.14426}
	// humped curve (maximum around 3y)
	humped = term.NSS{B0: 0.035, B1: -0.02, B2: 0.05, B3: 0.01, Tau1: 2.0, Tau2: 10.0}
	// inverted curve (5% short rate, 2% long rate)
	inverted = term.NSS{B0: 0.02, B1: 0.03, B2: -0.01, B3: 0.005, Tau1: 1.5, Tau2: 8.0}

	testSettle = time.Date(2021, 3, 31, 0, 0, 0, 0, time.UTC)
)

// syntheticBonds returns bonds with maturities between 0.5 and 30 years,
// various coupons and annual/semiannual frequencies, priced off curve c.
// If noise > 0, each market yield is shifted by a uniform error in
// [-noise, noise] (decimal) drawn with the seed.
func syntheticBonds(t testing.TB, c term.Curve, noise float64, seed int64) []Bond {
	t.Helper()
	months := []int{6, 9, 13, 18, 25, 31, 37, 49, 61, 74, 86, 97, 110, 122, 146, 170, 182, 205, 241, 266, 301, 337, 360}
	coupons := []float64{0, 0.005, 0.0125, 0.02, 0.0325, 0.04, 0.0075, 0.015}
	rng := rand.New(rand.NewSource(seed))
	var bonds []Bond
	for i, m := range months {
		mat := testSettle.AddDate(0, m, 3*i%17)
		freq := maturity.Annual
		if i%3 == 1 {
			freq = maturity.SemiAnnual
		}
		cpn := coupons[i%len(coupons)]
		s, err := bond.NewStraight(maturity.Schedule{Maturity: mat, Frequency: freq, DayCount: maturity.Thirty360E}, cpn)
		if err != nil {
			t.Fatal(err)
		}
		cfs, _ := s.CashFlows(testSettle)
		acc, _ := s.Accrued(testSettle)
		dirty := cashflow.PV(cfs, c)
		if noise > 0 {
			y, err := cashflow.YieldCC(dirty, cfs)
			if err != nil {
				t.Fatal(err)
			}
			y += noise * (2*rng.Float64() - 1)
			dirty = 0
			for _, cf := range cfs {
				dirty += cf.Amount * math.Exp(-y*cf.T)
			}
		}
		bonds = append(bonds, Bond{ID: mat.Format("2006-01-02"), Maturity: mat, Coupon: cpn, Frequency: freq,
			DayCount: maturity.Thirty360E, CleanPrice: dirty - acc})
	}
	return bonds
}

func prepare(t testing.TB, bonds []Bond, o Options) *Prepared {
	t.Helper()
	p, err := PrepareBonds(bonds, o)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFitNSSRoundTrip(t *testing.T) {
	for name, c := range map[string]term.NSS{"snb": snb, "humped": humped, "inverted": inverted} {
		o := Options{Settle: testSettle}
		p := prepare(t, syntheticBonds(t, c, 0, 0), o)
		res, err := FitNSS(p, o)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		d, at := maxZeroDiff(res.Curve, c, 0.25, 30)
		t.Logf("%s: max zero-rate error %.4fbp at %.2fy, RMSE %.2e bp, params %v, %d iterations", name, d, at, res.RMSEbp, res.Params, res.Iterations)
		if d > 0.1 {
			t.Errorf("%s: zero rates not recovered: max error %.4fbp at %.2fy (params %v)", name, d, at, res.Params)
		}
		if !res.Converged {
			t.Errorf("%s: not converged", name)
		}
		if res.RMSEbp > 0.05 {
			t.Errorf("%s: RMSE %.4fbp for exact prices", name, res.RMSEbp)
		}
	}
}

func TestFitNSSNoise(t *testing.T) {
	const noise = 0.5e-4 // ±0.5bp uniform
	for name, c := range map[string]term.NSS{"snb": snb, "humped": humped, "inverted": inverted} {
		o := Options{Settle: testSettle}
		p := prepare(t, syntheticBonds(t, c, noise, 42), o)
		res, err := FitNSS(p, o)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		d, at := maxZeroDiff(res.Curve, c, 0.25, 30)
		t.Logf("%s: max zero-rate error %.3fbp at %.2fy, RMSE %.3fbp (noise sd %.3fbp)", name, d, at, res.RMSEbp, noise*1e4/math.Sqrt(3))
		if d > 2 {
			t.Errorf("%s: zero rates not recovered within 2bp: %.3fbp at %.2fy", name, d, at)
		}
		// residual sd of a 6 parameter fit to 23 bonds: about sqrt(17/23)
		// of the noise sd (0.289bp)
		if res.RMSEbp < 0.1 || res.RMSEbp > 0.4 {
			t.Errorf("%s: RMSE %.3fbp not at the noise level", name, res.RMSEbp)
		}
	}
}

func TestFitNSSOvernight(t *testing.T) {
	for _, shift := range []float64{0, 0.001, -0.002} {
		on := snb.B0 + snb.B1 + shift
		o := Options{Settle: testSettle, OvernightRate: &on}
		p := prepare(t, syntheticBonds(t, snb, 0, 0), o)
		res, err := FitNSS(p, o)
		if err != nil {
			t.Fatal(err)
		}
		n := res.Curve.(term.NSS)
		if math.Abs(n.B0+n.B1-on) > 1e-15 || math.Abs(res.Curve.ZeroRate(0)-on) > 1e-15 {
			t.Errorf("shift %v: overnight constraint violated: B0+B1 = %v, want %v", shift, n.B0+n.B1, on)
		}
		if shift == 0 {
			if d, at := maxZeroDiff(res.Curve, snb, 0.25, 30); d > 0.1 {
				t.Errorf("consistent overnight rate: max zero-rate error %.4fbp at %.2fy", d, at)
			}
		}
	}
	bad := math.NaN()
	p := prepare(t, syntheticBonds(t, snb, 0, 0), Options{Settle: testSettle})
	if _, err := FitNSS(p, Options{Settle: testSettle, OvernightRate: &bad}); err == nil {
		t.Error("expected error for NaN overnight rate")
	}
}

func TestFitNSSMultiStartStability(t *testing.T) {
	o := Options{Settle: testSettle}
	p := prepare(t, syntheticBonds(t, humped, 0.5e-4, 7), o)
	ref, err := FitNSS(p, o)
	if err != nil {
		t.Fatal(err)
	}
	for seed := int64(1); seed <= 20; seed++ {
		rng := rand.New(rand.NewSource(seed))
		start := term.NSS{
			B0: humped.B0 + 0.02*(2*rng.Float64()-1), B1: humped.B1 + 0.02*(2*rng.Float64()-1),
			B2: humped.B2 + 0.05*(2*rng.Float64()-1), B3: humped.B3 + 0.05*(2*rng.Float64()-1),
			Tau1: humped.Tau1 * math.Exp(2*rng.Float64()-1), Tau2: humped.Tau2 * math.Exp(2*rng.Float64()-1),
		}
		oo := o
		oo.Start = &start
		res, err := FitNSS(p, oo)
		if err != nil {
			t.Fatal(err)
		}
		if rel := math.Abs(res.Objective-ref.Objective) / ref.Objective; rel > 1e-6 {
			t.Errorf("seed %d: objective %v differs from %v (rel %.2e), params %v vs %v", seed, res.Objective, ref.Objective, rel, res.Params, ref.Params)
		}
	}
	// deterministic
	again, _ := FitNSS(p, o)
	for i := range again.Params {
		if again.Params[i] != ref.Params[i] {
			t.Fatalf("non-deterministic fit: %v vs %v", again.Params, ref.Params)
		}
	}
}

func TestFitNSSConcurrent(t *testing.T) {
	o := Options{Settle: testSettle}
	p := prepare(t, syntheticBonds(t, snb, 0.5e-4, 3), o)
	ref, err := FitNSS(p, o)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 4)
	results := make([]*Result, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				results[i], errs[i] = FitNSS(p, o)
			} else {
				results[i], errs[i] = FitSpline(p, o)
			}
		}(i)
	}
	wg.Wait()
	for i := 0; i < 4; i += 2 {
		if errs[i] != nil || results[i].Objective != ref.Objective {
			t.Errorf("concurrent fit %d: %v, objective %v vs %v", i, errs[i], results[i].Objective, ref.Objective)
		}
	}
}

// TestFitSplineRoundTrip fits splines to bonds priced exactly off the SNB
// NSS curve (23 bonds, 0.5-30y). Measured maximum zero-rate errors on
// [0.5, 30]: 0.226bp with the default GCV smoothing (lambda 1e-3) and
// 0.225bp unpenalised, both at 25.3y between the last two knots (0.144bp
// with plain quantile knots, before the log blend); the tolerance is 0.25bp.
func TestFitSplineRoundTrip(t *testing.T) {
	p := prepare(t, syntheticBonds(t, snb, 0, 0), Options{Settle: testSettle})
	for _, lam := range []*float64{nil, Lambda(0)} {
		o := Options{Settle: testSettle, Lambda: lam}
		res, err := FitSpline(p, o)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := res.Curve.(*term.Spline); !ok {
			t.Fatalf("curve type %T", res.Curve)
		}
		d, at := maxZeroDiff(res.Curve, snb, 0.5, 30)
		t.Logf("spline round trip (lambda %g): %d knots %v, max zero-rate error %.3fbp at %.2fy, RMSE %.3fbp", res.Lambda, len(res.Knots), res.Knots, d, at, res.RMSEbp)
		if d > 0.25 {
			t.Errorf("spline round trip: max zero-rate error %.3fbp at %.2fy", d, at)
		}
		if len(res.Warnings) > 0 {
			t.Errorf("unexpected warnings %v", res.Warnings)
		}
	}
}

func TestFitSplineSmoothness(t *testing.T) {
	const noise = 0.5e-4
	o := Options{Settle: testSettle, Lambda: Lambda(LambdaGCV)}
	p := prepare(t, syntheticBonds(t, humped, noise, 11), o)
	res, err := FitSpline(p, o)
	if err != nil {
		t.Fatal(err)
	}
	s := res.Curve.(*term.Spline)
	knots, dfs := s.Knots(), s.DiscountFactors()
	t.Logf("GCV lambda %g, RMSE %.3fbp, knots %v", res.Lambda, res.RMSEbp, knots)
	for k := 1; k < len(knots); k++ {
		got := math.Abs(dfs[k] - dfs[k-1])
		want := math.Abs(humped.Discount(knots[k]) - humped.Discount(knots[k-1]))
		if math.Abs(got-want) > 2e-3 {
			t.Errorf("knots %v-%v: |dDF| = %.5f, curve %.5f (spike)", knots[k-1], knots[k], got, want)
		}
	}
	if d, at := maxZeroDiff(res.Curve, humped, 0.5, 30); d > 3 {
		t.Errorf("noisy spline: max zero-rate error %.3fbp at %.2fy", d, at)
	}
	if len(res.Warnings) > 0 {
		t.Errorf("unexpected warnings %v", res.Warnings)
	}
	if res.RMSEbp > 0.5 {
		t.Errorf("RMSE %.3fbp above the noise level", res.RMSEbp)
	}
}

func TestFitSplineLambda(t *testing.T) {
	o := Options{Settle: testSettle}
	p := prepare(t, syntheticBonds(t, humped, 0, 0), o)
	// one knot per bond maturity: lambda -> 0 interpolates the yields
	var knots []float64
	for _, b := range p.Bonds() {
		knots = append(knots, b.TTM)
	}
	o.Knots = knots
	o.Lambda = Lambda(0)
	interp, err := FitSpline(p, o)
	if err != nil {
		t.Fatal(err)
	}
	if interp.RMSEbp > 1e-6 {
		t.Errorf("lambda = 0 with a knot per bond: RMSE %.2e bp, want interpolation", interp.RMSEbp)
	}
	// lambda -> infinity: the zero rates at the knots become linear in t
	o.Lambda = Lambda(1e8)
	smooth, err := FitSpline(p, o)
	if err != nil {
		t.Fatal(err)
	}
	lin := func(x []float64) float64 {
		// max deviation of x from its least-squares line in t (bp)
		n := float64(len(knots))
		st, sx, stt, stx := 0.0, 0.0, 0.0, 0.0
		for k, tk := range knots {
			st += tk
			sx += x[k]
			stt += tk * tk
			stx += tk * x[k]
		}
		b := (n*stx - st*sx) / (n*stt - st*st)
		a := (sx - b*st) / n
		worst := 0.0
		for k, tk := range knots {
			worst = math.Max(worst, math.Abs(x[k]-a-b*tk))
		}
		return worst * 1e4
	}
	dInterp, dSmooth := lin(interp.Params), lin(smooth.Params)
	t.Logf("deviation from linear zero rates: lambda=0 %.2fbp, lambda=1e8 %.4fbp (RMSE %.1fbp)", dInterp, dSmooth, smooth.RMSEbp)
	if dSmooth > 0.05 || dSmooth > 0.01*dInterp {
		t.Errorf("large lambda: zero rates not linear (deviation %.4fbp vs %.2fbp)", dSmooth, dInterp)
	}
	// invalid knots
	o.Knots = []float64{1, 1, 2}
	if _, err := FitSpline(p, o); err == nil {
		t.Error("expected error for duplicate knots")
	}
}

func TestFitSplineOvernight(t *testing.T) {
	on := -0.008
	o := Options{Settle: testSettle, OvernightRate: &on}
	p := prepare(t, syntheticBonds(t, snb, 0, 0), o)
	res, err := FitSpline(p, o)
	if err != nil {
		t.Fatal(err)
	}
	if d := math.Abs(res.Curve.ZeroRate(0) - on); d > 1e-6 {
		t.Errorf("spline short rate %v, want %v", res.Curve.ZeroRate(0), on)
	}
}

// fdJacobian compares the analytic Jacobian of eval with central finite
// differences.
func checkJacobian(t *testing.T, name string, nx, nr int, eval func(x, r, jac []float64) bool, x []float64) {
	t.Helper()
	r := make([]float64, nr)
	jac := make([]float64, nr*nx)
	if !eval(x, r, jac) {
		t.Fatalf("%s: eval failed", name)
	}
	rp, rm := make([]float64, nr), make([]float64, nr)
	xp := make([]float64, nx)
	worst := 0.0
	for j := 0; j < nx; j++ {
		h := 1e-6 * math.Max(1, math.Abs(x[j]))
		copy(xp, x)
		xp[j] = x[j] + h
		eval(xp, rp, nil)
		xp[j] = x[j] - h
		eval(xp, rm, nil)
		colMax := 0.0
		for i := 0; i < nr; i++ {
			colMax = math.Max(colMax, math.Abs(jac[i*nx+j]))
		}
		for i := 0; i < nr; i++ {
			fd := (rp[i] - rm[i]) / (2 * h)
			d := math.Abs(fd-jac[i*nx+j]) / math.Max(colMax, 1e-12)
			worst = math.Max(worst, d)
			if d > 1e-6 {
				t.Errorf("%s: d r[%d]/d x[%d]: analytic %v, finite differences %v", name, i, j, jac[i*nx+j], fd)
			}
		}
	}
	t.Logf("%s: max relative Jacobian error %.2e", name, worst)
}

func TestJacobian(t *testing.T) {
	p := prepare(t, syntheticBonds(t, snb, 0.5e-4, 5), Options{Settle: testSettle})
	on := -0.007
	for _, obj := range []Objective{ObjectiveYield, ObjectivePrice} {
		for _, onr := range []*float64{nil, &on} {
			o := Options{Settle: testSettle, Objective: obj, OvernightRate: onr, Weights: WeightInvDuration}
			nx := term.NumParams
			if onr != nil {
				nx--
			}
			e, err := newEngine(p, o, nx)
			if err != nil {
				t.Fatal(err)
			}
			m := &nssModel{e: e, on: onr, nx: nx, nr: p.Len() + 3, ridge: 1e-3, sep: 1e-2}
			// tau ratio 1.1 < MinTauRatio: separation penalty active
			start := term.NSS{B0: 0.01, B1: -0.01, B2: 0.03, B3: -0.02, Tau1: 3, Tau2: 3.3}
			checkJacobian(t, "nss", nx, m.nr, m.eval, m.params(start))
			start.Tau2 = 0.8
			checkJacobian(t, "nss", nx, m.nr, m.eval, m.params(start))

			knots := []float64{0.7, 2, 5, 10, 25}
			es, _ := newEngine(p, o, len(knots))
			sm, err := newSplineModel(es, knots, onr)
			if err != nil {
				t.Fatal(err)
			}
			sm.lambda = 0.3
			checkJacobian(t, "spline", len(knots), sm.nr(), sm.eval, []float64{-0.006, -0.004, 0.001, 0.004, 0.006})
		}
	}
}

func TestModelYield(t *testing.T) {
	for _, y := range []float64{-0.05, -0.01, 0, 0.001, 0.03, 0.1, 0.5, 1, 2} {
		for _, b := range []struct {
			coupon float64
			freq   maturity.Frequency
			years  int
		}{{0, 1, 1}, {0, 1, 30}, {0.05, 2, 10}, {0.02, 1, 30}, {0.08, 12, 50}} {
			s, _ := bond.NewStraight(maturity.Schedule{Maturity: testSettle.AddDate(b.years, 0, 10), Frequency: b.freq, DayCount: maturity.ActAct}, b.coupon)
			cfs, _ := s.CashFlows(testSettle)
			var ts, la []float64
			price := 0.0
			for _, cf := range cfs {
				ts = append(ts, cf.T)
				la = append(la, math.Log(cf.Amount))
				price += cf.Amount * math.Exp(-y*cf.T)
			}
			for _, y0 := range []float64{y, 0, -0.5, 5} {
				got, d, ok := modelYield(ts, la, price, y0)
				if !ok || math.Abs(got-y) > 1e-12*math.Max(1, math.Abs(y)) {
					t.Errorf("y=%v %+v start %v: got %v (ok %v)", y, b, y0, got, ok)
				}
				if p2, d2 := priceDuration(ts, s2a(cfs), y); math.Abs(d-d2) > 1e-9*d2 || p2 <= 0 {
					t.Errorf("duration %v vs %v", d, d2)
				}
			}
		}
	}
	if _, _, ok := modelYield([]float64{1}, []float64{math.Log(100)}, -1, 0); ok {
		t.Error("expected failure for a negative price")
	}
}

func s2a(cfs []cashflow.CashFlow) []float64 {
	a := make([]float64, len(cfs))
	for i, cf := range cfs {
		a[i] = cf.Amount
	}
	return a
}

func TestPrepareBonds(t *testing.T) {
	bonds := syntheticBonds(t, snb, 0, 0)
	bonds = append(bonds,
		Bond{ID: "matured", Maturity: testSettle.AddDate(0, -1, 0), CleanPrice: 100},
		Bond{ID: "short", Maturity: testSettle.AddDate(0, 1, 0), CleanPrice: 100},
	)
	p, err := PrepareBonds(bonds, Options{Settle: testSettle})
	if err != nil {
		t.Fatal(err)
	}
	if p.Len() != len(bonds)-2 || len(p.Warnings()) != 2 {
		t.Errorf("got %d bonds, warnings %v", p.Len(), p.Warnings())
	}
	pb := p.Bonds()
	for i := 1; i < len(pb); i++ {
		if pb[i].TTM < pb[i-1].TTM {
			t.Fatal("bonds not sorted by maturity")
		}
	}
	// market yield consistent with annual equivalent
	if ya, _ := rate.FromContinuous(pb[0].Yield, 1); math.Abs(ya-pb[0].YieldAnnual) > 1e-15 {
		t.Error("annual yield inconsistent")
	}
	// all bonds kept with a negative MinTTM
	p2, _ := PrepareBonds(bonds, Options{Settle: testSettle, MinTTM: -1})
	if p2.Len() != len(bonds)-1 {
		t.Errorf("MinTTM < 0: got %d bonds", p2.Len())
	}
	for _, tc := range []struct {
		name  string
		bonds []Bond
		o     Options
	}{
		{"no settle", bonds, Options{}},
		{"bad price", []Bond{{Maturity: testSettle.AddDate(2, 0, 0), CleanPrice: math.NaN()}}, Options{Settle: testSettle}},
		{"zero price", []Bond{{Maturity: testSettle.AddDate(2, 0, 0)}}, Options{Settle: testSettle}},
		{"no maturity", []Bond{{CleanPrice: 100}}, Options{Settle: testSettle}},
		{"bad frequency", []Bond{{Maturity: testSettle.AddDate(2, 0, 0), CleanPrice: 100, Frequency: 5}}, Options{Settle: testSettle}},
		{"bad weight", []Bond{{Maturity: testSettle.AddDate(2, 0, 0), CleanPrice: 100}}, Options{Settle: testSettle, Weights: WeightCustom}},
		{"all dropped", []Bond{{Maturity: testSettle.AddDate(0, 1, 0), CleanPrice: 100}}, Options{Settle: testSettle}},
	} {
		if _, err := PrepareBonds(tc.bonds, tc.o); err == nil {
			t.Errorf("%s: expected error", tc.name)
		}
	}
	// too few bonds for NSS
	few, _ := PrepareBonds(bonds[:4], Options{Settle: testSettle})
	if _, err := FitNSS(few, Options{Settle: testSettle}); err == nil {
		t.Error("expected error for too few bonds")
	}
	if _, err := FitNSS(nil, Options{}); err == nil {
		t.Error("expected error for nil")
	}
	if _, err := FitSpline(nil, Options{}); err == nil {
		t.Error("expected error for nil")
	}
	// few bonds and invalid options: errors or results, no panics
	for n := 1; n <= 4; n++ {
		pn, err := PrepareBonds(bonds[:n], Options{Settle: testSettle})
		if err != nil {
			t.Fatal(err)
		}
		for _, o := range []Options{
			{Settle: testSettle}, {Settle: testSettle, Lambda: Lambda(0)}, {Settle: testSettle, Lambda: Lambda(1), NumKnots: 50},
			{Settle: testSettle, Knots: []float64{-1, 2}}, {Settle: testSettle, Lambda: Lambda(math.Inf(1))}, {Settle: testSettle, Huber: -1},
			{Settle: testSettle, Objective: 7}, {Settle: testSettle, Weights: 9},
		} {
			if res, err := FitSpline(pn, o); err == nil && (res.Curve == nil || math.IsNaN(res.RMSEbp)) {
				t.Errorf("%d bonds %+v: invalid result", n, o)
			}
			FitNSS(pn, o)
		}
	}
}

func TestReadBondsCSV(t *testing.T) {
	data := "maturity,coupon,price,id\n02.12.2021,0,100.007,A\n2030-05-27,0.5,106.036,\n\n"
	bonds, err := ReadBondsCSV(strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(bonds) != 2 || bonds[0].ID != "A" || bonds[1].ID != "row3" || bonds[1].Coupon != 0.005 ||
		!bonds[1].Maturity.Equal(time.Date(2030, 5, 27, 0, 0, 0, 0, time.UTC)) || bonds[0].CleanPrice != 100.007 {
		t.Errorf("unexpected bonds %+v", bonds)
	}
	// no header
	bonds, err = ReadBondsCSV(strings.NewReader("02.12.2021,1.5,100.007\n"))
	if err != nil || len(bonds) != 1 || bonds[0].Coupon != 0.015 || bonds[0].ID != "row1" {
		t.Errorf("no header: %+v, %v", bonds, err)
	}
	// named columns in any order with frequency and day count
	bonds, err = ReadBondsCSV(strings.NewReader("id,price,coupon,maturity,frequency,daycount\nX,99,2,2030-01-15,2,ACT/360\n"))
	if err != nil || len(bonds) != 1 || bonds[0].Frequency != maturity.SemiAnnual || bonds[0].DayCount != maturity.Act360 || bonds[0].CleanPrice != 99 {
		t.Errorf("named columns: %+v, %v", bonds, err)
	}
	for _, bad := range []string{
		"",
		"maturity,coupon,price\n31.02.2021,0,100\n",
		"maturity,coupon,price\n2021-12-02,x,100\n",
		"maturity,coupon,price\n2021-12-02,1,NaN\n",
		"maturity,coupon,price\n2021-12-02,1\n",
		"date,coupon,price\n2021-12-02,1,100\n",
		"maturity,coupon,price,frequency\n2021-12-02,1,100,5\n",
	} {
		if _, err := ReadBondsCSV(strings.NewReader(bad)); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}

func TestParseObjective(t *testing.T) {
	if o, err := ParseObjective("Price"); err != nil || o != ObjectivePrice {
		t.Error(o, err)
	}
	if _, err := ParseObjective("duration"); err == nil {
		t.Error("expected error")
	}
}

// sampleBonds returns the shipped CHF government bond sample (2021-11-26).
func sampleBonds(t testing.TB) ([]Bond, time.Time) {
	f, err := os.Open("../../cmds/termfit/bonddata.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	bonds, err := ReadBondsCSV(f)
	if err != nil {
		t.Fatal(err)
	}
	return bonds, time.Date(2021, 11, 26, 0, 0, 0, 0, time.UTC)
}

// TestSampleRegression fits the shipped sample. Reference: the previous
// termfit (gonum Nelson-Mead/BFGS on the yield errors in percent, all 39
// bonds) found b = (-1.219, 0.605, 3.074, -0.925)%, tau = (14.96, 0.425)
// with an RMSE of 4.94bp on 39 bonds and 3.85bp on the 27 bonds with at
// least 0.25y to maturity.
func TestSampleRegression(t *testing.T) {
	bonds, settle := sampleBonds(t)
	for _, tc := range []struct {
		minTTM, maxNSS, maxSpline float64
	}{{0, 2.0, 1.8}, {-1, 3.5, 3.0}} {
		o := Options{Settle: settle, MinTTM: tc.minTTM}
		p := prepare(t, bonds, o)
		start := time.Now()
		res, err := FitNSS(p, o)
		if err != nil {
			t.Fatal(err)
		}
		elapsed := time.Since(start)
		n := res.Curve.(term.NSS)
		t.Logf("NSS %d bonds: RMSE %.3fbp, b = (%.4f, %.4f, %.4f, %.4f)%%, tau = (%.3f, %.3f), %v, %d iterations, warnings %v",
			p.Len(), res.RMSEbp, n.B0*100, n.B1*100, n.B2*100, n.B3*100, n.Tau1, n.Tau2, elapsed, res.Iterations, fw(res.Warnings))
		if res.RMSEbp > tc.maxNSS || !res.Converged {
			t.Errorf("NSS %d bonds: RMSE %.3fbp (max %.1f), converged %v", p.Len(), res.RMSEbp, tc.maxNSS, res.Converged)
		}
		if math.Abs(n.B2) > MaxBeta || math.Abs(n.B3) > MaxBeta {
			t.Errorf("beta blow-up %v", res.Params)
		}
		if elapsed > time.Second {
			t.Errorf("NSS fit took %v", elapsed)
		}
		start = time.Now()
		sp, err := FitSpline(p, o)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("spline %d bonds: RMSE %.3fbp, lambda %g, knots %v, %v, warnings %v", p.Len(), sp.RMSEbp, sp.Lambda, sp.Knots, time.Since(start), fw(sp.Warnings))
		if sp.RMSEbp > tc.maxSpline {
			t.Errorf("spline %d bonds: RMSE %.3fbp", p.Len(), sp.RMSEbp)
		}
		// price objective (comparison only): finite, consistent report
		op := o
		op.Objective, op.Weights = ObjectivePrice, WeightInvDuration
		pr, err := FitNSS(p, op)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("NSS price objective (inverse duration weights): RMSE %.3fbp, params %v", pr.RMSEbp, pr.Params)
	}
}

// fw returns the warnings other than dropped bonds.
func fw(w []string) []string {
	var o []string
	for _, s := range w {
		if !strings.Contains(s, "dropped") {
			o = append(o, s)
		}
	}
	return o
}

func benchPrepared(b *testing.B) *Prepared {
	bonds, settle := sampleBonds(b)
	return prepare(b, bonds, Options{Settle: settle, MinTTM: -1})
}

func BenchmarkFitNSS(b *testing.B) {
	p := benchPrepared(b)
	o := Options{Settle: p.Settle()}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := FitNSS(p, o); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFitSpline(b *testing.B) {
	p := benchPrepared(b)
	o := Options{Settle: p.Settle(), Lambda: Lambda(0)}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := FitSpline(p, o); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFitSplineGCV(b *testing.B) {
	p := benchPrepared(b)
	o := Options{Settle: p.Settle(), Lambda: Lambda(LambdaGCV)}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := FitSpline(p, o); err != nil {
			b.Fatal(err)
		}
	}
}
