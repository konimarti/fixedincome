package fit

import (
	"math"
	"strings"
	"testing"

	"github.com/konimarti/fixedincome/pkg/rate"
	"github.com/konimarti/fixedincome/pkg/term"
)

func hasWarning(w []string, sub string) bool {
	for _, s := range w {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// RF1: a box-constrained problem whose unconstrained minimum lies outside
// the box: min (x0+x1-1)² + (x0-x1-3)² with x1 >= 0 has the solution (2, 0)
// (unconstrained: (2, -1)).
func TestLMBounds(t *testing.T) {
	eval := func(x, r, jac []float64) bool {
		r[0], r[1] = x[0]+x[1]-1, x[0]-x[1]-3
		if jac != nil {
			jac[0], jac[1], jac[2], jac[3] = 1, 1, 1, -1
		}
		return true
	}
	for _, start := range [][]float64{{0, 5}, {10, 0}, {-3, 0.001}} {
		x := append([]float64(nil), start...)
		p := lmProblem{nx: 2, nr: 2, eval: eval, lower: []float64{math.Inf(-1), 0}, upper: []float64{math.Inf(1), math.Inf(1)}}
		lr, err := levenbergMarquardt(p, x, lmSettings{maxIter: 100, xtol: 1e-14, ftol: 1e-16}, nil)
		if err != nil || !lr.converged || math.Abs(x[0]-2) > 1e-8 || x[1] != 0 {
			t.Errorf("start %v: x = %v, %+v, %v", start, x, lr, err)
		}
	}
}

// RF1: the 39-bond sample ends at tau1 = 0.1 (lower bound) with an RMSE of
// about 3.41bp; polishing from that optimum must not improve it.
func TestNSSBoundOptimum(t *testing.T) {
	bonds, settle := sampleBonds(t)
	o := Options{Settle: settle, MinTTM: -1}
	p := prepare(t, bonds, o)
	res, err := FitNSS(p, o)
	if err != nil {
		t.Fatal(err)
	}
	n := res.Curve.(term.NSS)
	if math.Abs(res.RMSEbp-3.413) > 0.01 || math.Abs(n.Tau1-MinTau) > 1e-12 || !res.Converged {
		t.Errorf("39 bonds: RMSE %.4f, params %v, converged %v", res.RMSEbp, res.Params, res.Converged)
	}
	o2 := o
	o2.Start = &n
	o2.MaxIter = 5000
	again, err := FitNSS(p, o2)
	if err != nil {
		t.Fatal(err)
	}
	if again.Objective < res.Objective*(1-1e-9) {
		t.Errorf("polish from the optimum improved it: %.10g < %.10g (%v)", again.Objective, res.Objective, again.Params)
	}
	if !hasWarning(res.Warnings, "tau1 = 0.1 at the lower bound") {
		t.Errorf("no bound warning: %v", fw(res.Warnings))
	}
}

// RF1: the betas-only stage-1 problem equals the full problem with the
// decay parameters fixed by lower == upper.
func TestNSSBetasOnlyEqualsFixedTau(t *testing.T) {
	bonds, settle := sampleBonds(t)
	o := Options{Settle: settle}
	p := prepare(t, bonds, o)
	nb, nx := p.Len(), term.NumParams
	for _, taus := range [][2]float64{{0.6, 8.5}, {4, 2}, {15, 0.35}} {
		e1, _ := newEngine(p, o, nx)
		m1 := &nssModel{e: e1, nx: nx, nr: nb + 3, ridge: 1e-4, sep: 0.05}
		x1 := m1.linearStart(taus[0], taus[1])
		lb := []float64{math.Inf(-1), math.Inf(-1), math.Inf(-1), math.Inf(-1), x1[4], x1[5]}
		ub := []float64{math.Inf(1), math.Inf(1), math.Inf(1), math.Inf(1), x1[4], x1[5]}
		s := lmSettings{maxIter: 500, xtol: 1e-15, ftol: 1e-16}
		lr1, err1 := levenbergMarquardt(lmProblem{nx: nx, nr: m1.nr, eval: m1.eval, lower: lb, upper: ub}, x1, s, nil)
		e2, _ := newEngine(p, o, nx)
		m2 := &nssModel{e: e2, nx: nx, nr: nb + 3, ridge: 1e-4, sep: 0.05}
		x2 := m2.linearStart(taus[0], taus[1])
		lr2, err2 := levenbergMarquardt(m2.betaProblem(x2), x2[:nx-2], s, nil)
		if err1 != nil || err2 != nil || math.Abs(lr1.cost-lr2.cost) > 1e-9*lr2.cost {
			t.Errorf("taus %v: fixed-tau cost %v, betas-only %v (%v %v)", taus, lr1.cost, lr2.cost, err1, err2)
		}
		for i := 0; i < 4; i++ {
			if math.Abs(x1[i]-x2[i]) > 1e-7 {
				t.Errorf("taus %v: betas %v vs %v", taus, x1[:4], x2[:4])
				break
			}
		}
	}
}

// RF2: alternative local solutions and the long-end diagnostic.
func TestNSSDiagnostics(t *testing.T) {
	bonds, settle := sampleBonds(t)
	o := Options{Settle: settle}
	p := prepare(t, bonds, o)
	res, err := FitNSS(p, o)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("35 bonds: RMSE %.3fbp, params %v, warnings %v", res.RMSEbp, res.Params, fw(res.Warnings))
	// alternativeWarning on constructed candidates
	e, _ := newEngine(p, o, term.NumParams)
	m := &nssModel{e: e, nx: term.NumParams, nr: p.Len() + 3}
	a := m.params(term.NSS{B0: 0.01, B1: -0.01, B2: 0.01, B3: 0.01, Tau1: 1, Tau2: 5})
	b := m.params(term.NSS{B0: 0.02, B1: -0.02, B2: 0.01, B3: 0.01, Tau1: 1, Tau2: 5})
	cands := []nssCandidate{{x: a, cost: 1, ok: true}, {x: b, cost: 1.04, ok: true}, {x: a, cost: 1.01, ok: true}}
	if w := alternativeWarning(m, cands, 0, 30); !strings.Contains(w, "alternative local solution") || !strings.Contains(w, "+4.00%") {
		t.Errorf("alternative warning %q", w)
	}
	cands[1].cost = 1.06
	if w := alternativeWarning(m, cands, 0, 30); w != "" {
		t.Errorf("unexpected alternative warning %q", w)
	}
	// RF2/RF3: bound and separation warnings
	w := nssWarnings(term.NSS{Tau1: 30, Tau2: 2}, true, 40)
	if !hasWarning(w, "upper bound") || !hasWarning(w, "not the data") {
		t.Errorf("upper bound warning %v", w)
	}
	w = nssWarnings(term.NSS{Tau1: 4, Tau2: 4 / MinTauRatio}, true, 40)
	if !hasWarning(w, "separation constraint active") || !hasWarning(w, "collinear") {
		t.Errorf("separation warning %v", w)
	}
	if w = nssWarnings(term.NSS{Tau1: 4, Tau2: 2}, true, 40); len(w) != 0 {
		t.Errorf("unexpected warnings %v", w)
	}
	// B0 standard error: the 35-bond sample optimum has tau2 near 30y and an
	// unidentified long-run level; synthetic bonds up to 30y do not
	if !hasWarning(res.Warnings, "B0 (long-run rate) poorly determined") {
		t.Errorf("no B0 warning on the sample: %v", res.Warnings)
	}
	full, _ := FitNSS(prepare(t, syntheticBonds(t, snb, 0.5e-4, 1), Options{Settle: testSettle}), Options{Settle: testSettle})
	if hasWarning(full.Warnings, "B0") {
		t.Errorf("B0 warning for bonds up to 30y: %v", full.Warnings)
	}
}

// RF12: the separation penalty and the ridge have an effect: polished from
// the beta blow-up that the previous termfit found on the 35-bond sample
// (b2 = 288%, b3 = -287%, tau = 3.897/3.880), Levenberg-Marquardt stays in
// the blow-up without them and leaves it with them.
func TestNSSRegularisationEffect(t *testing.T) {
	bonds, settle := sampleBonds(t)
	o := Options{Settle: settle}
	p := prepare(t, bonds, o)
	old := term.NSS{B0: -0.002977, B1: -0.004846, B2: 2.879, B3: -2.870, Tau1: 3.897, Tau2: 3.880}
	nb, nx := p.Len(), term.NumParams
	scale := math.Sqrt(float64(nb))
	lower := []float64{math.Inf(-1), math.Inf(-1), math.Inf(-1), math.Inf(-1), math.Log(MinTau), math.Log(MinTau)}
	upper := []float64{math.Inf(1), math.Inf(1), math.Inf(1), math.Inf(1), math.Log(MaxTau), math.Log(MaxTau)}
	polish := func(ridge, sep float64) term.NSS {
		e, _ := newEngine(p, o, nx)
		m := &nssModel{e: e, nx: nx, nr: nb + 3, ridge: ridge * scale, sep: sep * scale}
		x := m.params(old)
		if _, err := levenbergMarquardt(lmProblem{nx: nx, nr: m.nr, eval: m.eval, lower: lower, upper: upper}, x,
			lmSettings{maxIter: 1000, xtol: 1e-12, ftol: 1e-15}, nil); err != nil {
			t.Fatal(err)
		}
		return m.curve(x)
	}
	save := logSep
	logSep = 0
	free := polish(0, 0)
	logSep = save
	reg := polish(DefaultRidge, 1e-2)
	t.Logf("from the blow-up: without regularisation %v, with %v", free.Params(), reg.Params())
	if !(math.Abs(free.B2) > MaxBeta) {
		t.Errorf("expected the blow-up to persist without regularisation: %v", free.Params())
	}
	if math.Abs(reg.B2) > MaxBeta || math.Abs(reg.B3) > MaxBeta || math.Abs(math.Log(reg.Tau1/reg.Tau2)) < logSep*(1-1e-3) {
		t.Errorf("regularised polish still blown up: %v", reg.Params())
	}
	if w := nssWarnings(free, true, 42); !hasWarning(w, "beta blow-up") {
		t.Errorf("no blow-up warning: %v", w)
	}
}

// RF4: Huber loss limits the influence of an outlier; outliers and
// implausible yields are reported.
func TestHuberOutlier(t *testing.T) {
	bonds := syntheticBonds(t, snb, 0, 0)
	// shift bond 10 by +50bp in yield: lower price
	p0 := prepare(t, bonds, Options{Settle: testSettle})
	target := bonds[10].ID
	for _, b := range p0.Bonds() {
		if b.ID == target {
			dirty := 0.0
			for _, cf := range b.CashFlows {
				dirty += cf.Amount * math.Exp(-(b.Yield+0.005)*cf.T)
			}
			bonds[10].CleanPrice = (dirty - b.Accrued) * 100 / b.Face
		}
	}
	o := Options{Settle: testSettle}
	p := prepare(t, bonds, o)
	for _, model := range []string{"nss", "spline"} {
		fitf := FitNSS
		if model == "spline" {
			fitf = FitSpline
		}
		ls, err := fitf(p, o)
		if err != nil {
			t.Fatal(err)
		}
		oh := o
		oh.Huber = 1
		hb, err := fitf(p, oh)
		if err != nil {
			t.Fatal(err)
		}
		dls, _ := maxZeroDiff(ls.Curve, snb, 0.5, 30)
		dhb, _ := maxZeroDiff(hb.Curve, snb, 0.5, 30)
		t.Logf("%s: max zero-rate error least squares %.2fbp, Huber %.2fbp", model, dls, dhb)
		if !(dhb < 0.5*dls) {
			t.Errorf("%s: Huber does not reduce the outlier distortion: %.2f vs %.2f", model, dhb, dls)
		}
		if !hasWarning(hb.Warnings, target+": yield error") {
			t.Errorf("%s: outlier %s not reported: %v", model, target, hb.Warnings)
		}
	}
	// price quoted as a fraction of face
	bonds[3].CleanPrice /= 100
	pw := prepare(t, bonds, o)
	if !hasWarning(pw.Warnings(), bonds[3].ID+": implausible market yield") || !hasWarning(pw.Warnings(), "percent of face") {
		t.Errorf("no implausible yield warning: %v", pw.Warnings())
	}
	if _, err := FitNSS(p, Options{Settle: testSettle, Huber: math.NaN()}); err == nil {
		t.Error("expected error for a NaN Huber threshold")
	}
}

// RF5: prices are percent of face for any face value.
func TestFaceValue(t *testing.T) {
	mat := testSettle.AddDate(5, 2, 0)
	b100 := Bond{ID: "a", Maturity: mat, Coupon: 0.02, CleanPrice: 99}
	b1000 := b100
	b1000.Face = 1000
	p, err := PrepareBonds([]Bond{b100, b1000}, Options{Settle: testSettle})
	if err != nil {
		t.Fatal(err)
	}
	pb := p.Bonds()
	if math.Abs(pb[0].Yield-pb[1].Yield) > 1e-14 {
		t.Errorf("yield depends on face: %v vs %v", pb[0].Yield, pb[1].Yield)
	}
	if math.Abs(pb[1].DirtyPrice-(990+pb[1].Accrued)) > 1e-9 || math.Abs(pb[1].Accrued-10*pb[0].Accrued) > 1e-9 {
		t.Errorf("face 1000: dirty %v accrued %v", pb[1].DirtyPrice, pb[1].Accrued)
	}
	res, _, err := report(p, term.Flat{Rate: pb[0].Yield})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if math.Abs(r.CleanMarket-99) > 1e-12 || math.Abs(r.CleanModel-99) > 1e-6 || math.Abs(r.DirtyModel-r.DirtyMarket) > 1e-6 {
			t.Errorf("residual not in percent of face: %+v", r)
		}
	}
}

// RF6
func TestOvernightToCC(t *testing.T) {
	if got := OvernightToCC(-0.0075); math.Abs(got*100-(-0.7604)) > 5e-5 {
		t.Errorf("OvernightToCC(-0.75%%) = %.6f%%, want -0.7604%%", got*100)
	}
	if OvernightToCC(0) != 0 {
		t.Error("OvernightToCC(0) != 0")
	}
}

// RF9
func TestReadBondsCSVFormats(t *testing.T) {
	bonds, err := ReadBondsCSV(strings.NewReader("\xef\xbb\xbfmaturity,coupon,price\n2030-01-15,1,100\n"))
	if err != nil || len(bonds) != 1 || bonds[0].Coupon != 0.01 {
		t.Errorf("BOM: %+v %v", bonds, err)
	}
	for in, want := range map[string]string{
		"maturity,coupon,price,coupon\n2030-01-15,1,100,2\n": "duplicate column",
		"maturity,coupon\n2030-01-15,1\n":                    "missing column \"price\"",
		"maturity;coupon;price\n2030-01-15;1;100\n":          "semicolons",
		"maturity\tcoupon\tprice\n2030-01-15\t1\t100\n":      "tabs",
		"2030-01-15;1;100\n":                                 "semicolons",
	} {
		if _, err := ReadBondsCSV(strings.NewReader(in)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error %v, want %q", in, err, want)
		}
	}
}

// RF12: the analytic GCV trace of the hat matrix equals the numerical
// derivative sum_i d yhat_i / d y_i of the fitted yields.
func TestGCVTrace(t *testing.T) {
	bonds, settle := sampleBonds(t)
	for _, lam := range []float64{1e-4, 1e-2, 1} {
		o := Options{Settle: settle, Lambda: Lambda(lam)}
		p := prepare(t, bonds, o)
		base, err := FitSpline(p, o)
		if err != nil {
			t.Fatal(err)
		}
		e, _ := newEngine(p, o, len(base.Knots))
		m, _ := newSplineModel(e, base.Knots, nil)
		m.lambda = lam
		work := newLMWork(m.nx, m.nr())
		score := m.gcv(base.Params, work)
		rss := 0.0
		for _, r := range base.Residuals {
			rss += (r.YieldModel - r.YieldMarket) * (r.YieldModel - r.YieldMarket)
		}
		n := float64(p.Len())
		trA := n - math.Sqrt(n*rss/score)
		trN := 0.0
		const eps = 1e-6
		for i := range p.bonds {
			q := *p
			q.bonds = append([]prepared(nil), p.bonds...)
			q.bonds[i].yMkt += eps
			r2, err := FitSpline(&q, o)
			if err != nil {
				t.Fatal(err)
			}
			trN += (r2.Residuals[i].YieldModel - base.Residuals[i].YieldModel) / eps
		}
		t.Logf("lambda %g: trace analytic %.4f, numerical %.4f", lam, trA, trN)
		if math.Abs(trA-trN) > 0.02*trN+0.01 {
			t.Errorf("lambda %g: trace analytic %.4f, numerical %.4f", lam, trA, trN)
		}
	}
}

// RF12: sign of ErrBp and annual yields of the residuals.
func TestResidualConventions(t *testing.T) {
	p := prepare(t, syntheticBonds(t, snb, 0, 0), Options{Settle: testSettle})
	up := term.WithSpread(snb, 0.001) // model yields 10bp above market
	res, _, err := report(p, up)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if math.Abs(r.ErrBp-10) > 0.1 || !(r.CleanModel < r.CleanMarket) {
			t.Errorf("%s: ErrBp %v, clean model %v market %v", r.ID, r.ErrBp, r.CleanModel, r.CleanMarket)
		}
		if math.Abs(r.ErrBp-(r.YieldModel-r.YieldMarket)*1e4) > 1e-9 {
			t.Errorf("ErrBp %v != (model - market) yield", r.ErrBp)
		}
		ya, _ := rate.FromContinuous(r.YieldModel, 1)
		if math.Abs(r.YieldModelAnnual-math.Expm1(r.YieldModel)) > 1e-15 || r.YieldModelAnnual != ya ||
			math.Abs(r.YieldMarketAnnual-math.Expm1(r.YieldMarket)) > 1e-15 {
			t.Errorf("annual yields %v %v for cc %v %v", r.YieldModelAnnual, r.YieldMarketAnnual, r.YieldModel, r.YieldMarket)
		}
	}
}

// RF16: with inverse duration-price weights the price objective
// approximates the yield objective.
func TestInvDurationWeights(t *testing.T) {
	o := Options{Settle: testSettle}
	p := prepare(t, syntheticBonds(t, humped, 2e-4, 9), o)
	y, _ := FitNSS(p, o)
	pu, _ := FitNSS(p, Options{Settle: testSettle, Objective: ObjectivePrice})
	pd, _ := FitNSS(p, Options{Settle: testSettle, Objective: ObjectivePrice, Weights: WeightInvDuration})
	du, _ := maxZeroDiff(pu.Curve, y.Curve, 0.5, 30)
	dd, _ := maxZeroDiff(pd.Curve, y.Curve, 0.5, 30)
	t.Logf("distance to the yield fit: price unit weights %.3fbp, inverse duration-price weights %.3fbp", du, dd)
	if !(dd < du) || dd > 0.5 {
		t.Errorf("inverse duration weights: %.3fbp vs unit %.3fbp", dd, du)
	}
}

// RF13: the knots of the 39-bond sample cover the 1.6-7.2y range.
func TestSplineKnotPlacement(t *testing.T) {
	bonds, settle := sampleBonds(t)
	p := prepare(t, bonds, Options{Settle: settle, MinTTM: -1})
	res, err := FitSpline(p, Options{Settle: settle})
	if err != nil {
		t.Fatal(err)
	}
	k := res.Knots
	for i := 1; i < len(k); i++ {
		if k[i-1] <= 1.6 && k[i] >= 7.2 {
			t.Errorf("knot gap %v-%v in %v", k[i-1], k[i], k)
		}
	}
	if len(k) > 9 || k[len(k)-1] != p.bonds[p.Len()-1].ttm {
		t.Errorf("knots %v", k)
	}
}
