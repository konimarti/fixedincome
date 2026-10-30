package fit

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/konimarti/fixedincome/pkg/term"
)

// Bounds of the NSS decay parameters (years) and their minimum separation.
const (
	// MinTau and MaxTau bound the NSS decay parameters (years)
	MinTau, MaxTau = 0.1, 30.0
	// MinTauRatio is the minimum ratio max(Tau1,Tau2)/min(Tau1,Tau2)
	// enforced by a soft penalty; closer decay parameters make B2 and B3
	// collinear
	MinTauRatio = 1.25
	// MaxBeta is the magnitude (decimal, 0.5 = 50 percentage points) of
	// B2 or B3 above which FitNSS warns of a beta blow-up
	MaxBeta = 0.5
	// MaxB0StdErr is the standard error of B0 (decimal) above which FitNSS
	// warns that the long end is poorly determined
	MaxB0StdErr = 0.0025
	// AltSolutionTol is the relative objective difference within which a
	// different local NSS solution is reported
	AltSolutionTol = 0.05
)

// tauGrid are the decay parameters (years) of the multi-start grid.
var tauGrid = []float64{0.2, 0.35, 0.6, 1, 1.7, 3, 5, 8.5, 15, 25}

// logSep is ln(MinTauRatio) (a variable for tests).
var logSep = math.Log(MinTauRatio)

// nssModel maps the internal parameter vector x to an NSS curve: x = (B0,
// B1, B2, B3, ln Tau1, ln Tau2), or (B0, B2, B3, ln Tau1, ln Tau2) with B1
// = on - B0 under the overnight constraint. Extra residuals: ridge on B2,
// B3 and the tau separation penalty.
type nssModel struct {
	e      *engine
	on     *float64
	nx, nr int
	ridge  float64 // weight of the ridge residuals
	sep    float64 // weight of the separation residual
	grad   [term.NumParams]float64
}

func (m *nssModel) curve(x []float64) term.NSS {
	if m.on != nil {
		return term.NSS{B0: x[0], B1: *m.on - x[0], B2: x[1], B3: x[2], Tau1: math.Exp(x[3]), Tau2: math.Exp(x[4])}
	}
	return term.NSS{B0: x[0], B1: x[1], B2: x[2], B3: x[3], Tau1: math.Exp(x[4]), Tau2: math.Exp(x[5])}
}

// params returns the internal parameters of the curve n.
func (m *nssModel) params(n term.NSS) []float64 {
	if m.on != nil {
		return []float64{n.B0, n.B2, n.B3, math.Log(n.Tau1), math.Log(n.Tau2)}
	}
	return []float64{n.B0, n.B1, n.B2, n.B3, math.Log(n.Tau1), math.Log(n.Tau2)}
}

func (m *nssModel) eval(x, r, jac []float64) bool {
	n := m.curve(x)
	if n.Validate() != nil {
		return false
	}
	e, nx := m.e, m.nx
	g := m.grad[:]
	for k, t := range e.t {
		z := n.Discount(t)
		e.z[k] = z
		if jac == nil {
			continue
		}
		n.DiscountGrad(t, g)
		row := e.dz[k*nx : (k+1)*nx]
		if m.on != nil {
			row[0], row[1], row[2], row[3], row[4] = g[0]-g[1], g[2], g[3], g[4]*n.Tau1, g[5]*n.Tau2
		} else {
			row[0], row[1], row[2], row[3], row[4], row[5] = g[0], g[1], g[2], g[3], g[4]*n.Tau1, g[5]*n.Tau2
		}
	}
	if !e.residuals(r, jac, nx) {
		return false
	}
	nb := len(e.p.bonds)
	ib2, ia1 := 2, 4
	if m.on != nil {
		ib2, ia1 = 1, 3
	}
	if jac != nil {
		for k := nb * nx; k < len(jac); k++ {
			jac[k] = 0
		}
	}
	// ridge rows
	r[nb], r[nb+1] = m.ridge*x[ib2], m.ridge*x[ib2+1]
	// separation row
	d := x[ia1] - x[ia1+1]
	r[nb+2] = 0
	if v := logSep - math.Abs(d); v > 0 {
		r[nb+2] = m.sep * v
	}
	if jac != nil {
		jac[nb*nx+ib2] = m.ridge
		jac[(nb+1)*nx+ib2+1] = m.ridge
		if r[nb+2] > 0 {
			s := 1.0
			if d < 0 {
				s = -1
			}
			jac[(nb+2)*nx+ia1] = -m.sep * s
			jac[(nb+2)*nx+ia1+1] = m.sep * s
		}
	}
	return true
}

// betaProblem returns the least-squares problem in the betas only (the
// first nx-2 parameters), with the log decay parameters fixed at the last
// two entries of full (stage 1 of the multi-start).
func (m *nssModel) betaProblem(full []float64) lmProblem {
	nx, nr := m.nx, m.nr
	nbeta := nx - 2
	jfull := make([]float64, nr*nx)
	x := make([]float64, nx)
	copy(x, full)
	eval := func(beta, r, jac []float64) bool {
		copy(x, beta)
		var jj []float64
		if jac != nil {
			jj = jfull
		}
		if !m.eval(x, r, jj) {
			return false
		}
		if jac != nil {
			for k := 0; k < nr; k++ {
				copy(jac[k*nbeta:(k+1)*nbeta], jfull[k*nx:k*nx+nbeta])
			}
		}
		return true
	}
	return lmProblem{nx: nbeta, nr: nr, eval: eval}
}

// linearStart returns the betas for fixed decay parameters from the linear
// least-squares fit of the zero rates at the bond durations to the market
// yields, y_i ≈ r(D_i) (a cheap approximation used to seed the multi-start).
func (m *nssModel) linearStart(tau1, tau2 float64) []float64 {
	nb := len(m.e.p.bonds)
	cols := 4
	if m.on != nil {
		cols = 3
	}
	ata := make([]float64, cols*cols)
	atb := make([]float64, cols)
	row := make([]float64, cols)
	for i, b := range m.e.p.bonds {
		n := term.NSS{B0: 1, Tau1: tau1, Tau2: tau2}
		var g [term.NumParams]float64
		n.ZeroRateGrad(b.duration, g[:])
		y := b.yMkt
		if m.on != nil {
			row[0], row[1], row[2] = g[0]-g[1], g[2], g[3]
			y -= *m.on * g[1]
		} else {
			copy(row, g[:4])
		}
		w := m.e.w[i] * m.e.w[i] * m.e.hw[i] * m.e.hw[i]
		for j := 0; j < cols; j++ {
			atb[j] += w * row[j] * y
			for k := 0; k < cols; k++ {
				ata[j*cols+k] += w * row[j] * row[k]
			}
		}
	}
	// light ridge on all betas for a well-posed system
	for j := 0; j < cols; j++ {
		ata[j*cols+j] += float64(nb)*1e-8 + m.ridge*m.ridge
	}
	l := make([]float64, cols*cols)
	if !cholSolve(ata, atb, cols, l) {
		atb = make([]float64, cols)
	}
	x := make([]float64, 0, m.nx)
	x = append(x, atb...)
	return append(x, math.Log(tau1), math.Log(tau2))
}

type nssCandidate struct {
	x    []float64
	cost float64
	lr   lmResult
	ok   bool
}

// FitNSS fits a Nelson-Siegel-Svensson curve to the prepared bonds by
// minimising the weighted squared yield errors (see Options.Objective).
//
// The decay parameters are fitted as ln(Tau) within [MinTau, MaxTau]
// (bound-constrained Levenberg-Marquardt) with a soft penalty keeping Tau1
// and Tau2 at least a factor MinTauRatio apart; B2 and B3 are lightly
// ridge-regularised (Options.Ridge). With Options.OvernightRate, B1 = on -
// B0 is eliminated. The optimiser is deterministic: for each pair of decay
// parameters on a grid the betas are seeded by linear least squares and
// refined by Levenberg-Marquardt in the betas only; the best starts (plus
// Options.Start, if given) are polished by Levenberg-Marquardt on all
// parameters with the analytic Jacobian. With Options.Huber the best
// solution is refined by IRLS. At least as many bonds as free parameters are
// required.
//
// Warnings report dropped bonds, implausible market yields, decay
// parameters at a bound, an active separation penalty (B2 and B3 then
// collinear), beta blow-ups, a poorly determined long end (B0), a
// different local solution within AltSolutionTol of the objective and
// outliers (|error| > OutlierFactor x RMSE).
func FitNSS(p *Prepared, o Options) (*Result, error) {
	if p == nil || len(p.bonds) == 0 {
		return nil, errors.New("fit: no bonds")
	}
	nx := term.NumParams
	if o.OvernightRate != nil {
		if !finiteValue(*o.OvernightRate) {
			return nil, fmt.Errorf("fit: invalid overnight rate %v", *o.OvernightRate)
		}
		nx--
	}
	nb := len(p.bonds)
	if nb < nx {
		return nil, fmt.Errorf("fit: too few bonds for the NSS fit: %d, need at least %d", nb, nx)
	}
	if !finiteValue(o.Ridge) {
		return nil, fmt.Errorf("fit: invalid ridge %v", o.Ridge)
	}
	if !finiteValue(o.Huber) || o.Huber < 0 {
		return nil, fmt.Errorf("fit: invalid Huber threshold %v", o.Huber)
	}
	ridge := o.Ridge
	switch {
	case ridge == 0:
		ridge = DefaultRidge
	case ridge < 0:
		ridge = 0
	}
	e, err := newEngine(p, o, nx)
	if err != nil {
		return nil, err
	}
	scale := math.Sqrt(float64(nb))
	if o.Objective == ObjectivePrice {
		scale *= 1e2 // price residuals (percent of face) per yield unit ~ duration*100
	}
	m := &nssModel{e: e, on: o.OvernightRate, nx: nx, nr: nb + 3, ridge: ridge * scale, sep: 1e-2 * scale}
	lower := make([]float64, nx)
	upper := make([]float64, nx)
	for i := range lower {
		lower[i], upper[i] = math.Inf(-1), math.Inf(1)
	}
	lower[nx-2], lower[nx-1] = math.Log(MinTau), math.Log(MinTau)
	upper[nx-2], upper[nx-1] = math.Log(MaxTau), math.Log(MaxTau)
	prob := lmProblem{nx: nx, nr: m.nr, eval: m.eval, lower: lower, upper: upper}
	work := newLMWork(nx, m.nr)
	res := &Result{Warnings: p.Warnings()}

	var start []float64
	if o.Start != nil {
		s := *o.Start
		if err := s.Validate(); err != nil {
			return nil, fmt.Errorf("fit: invalid start: %w", err)
		}
		s.Tau1 = math.Min(math.Max(s.Tau1, MinTau), MaxTau)
		s.Tau2 = math.Min(math.Max(s.Tau2, MinTau), MaxTau)
		if o.OvernightRate != nil {
			s.B1 = *o.OvernightRate - s.B0
		}
		start = m.params(s)
	}
	settings := lmSettings{maxIter: o.maxIter(), xtol: 1e-12, ftol: 1e-15, gtol: 1e-20}
	betaWork := newLMWork(nx-2, m.nr)

	// search runs the multi-start with the current (robust) weights and
	// returns the polished candidates and the index of the best one.
	search := func() ([]nssCandidate, int, error) {
		// stage 1: grid over (tau1, tau2), betas only
		var cands []nssCandidate
		for _, t1 := range tauGrid {
			for _, t2 := range tauGrid {
				if math.Abs(math.Log(t1/t2)) < logSep {
					continue
				}
				x := m.linearStart(t1, t2)
				lr, err := levenbergMarquardt(m.betaProblem(x), x[:nx-2], lmSettings{maxIter: 10, xtol: 1e-8, ftol: 1e-6}, betaWork)
				res.Iterations += lr.iterations
				res.Evaluations += lr.evaluations
				if err != nil {
					continue
				}
				cands = append(cands, nssCandidate{x: x, cost: lr.cost})
			}
		}
		cands = selectStarts(cands, nx)
		if start != nil {
			cands = append(cands, nssCandidate{x: append([]float64(nil), start...)})
		}
		// stage 2: polish
		best := -1
		for i := range cands {
			c := &cands[i]
			lr, err := levenbergMarquardt(prob, c.x, settings, work)
			res.Iterations += lr.iterations
			res.Evaluations += lr.evaluations
			if err != nil {
				continue
			}
			c.cost, c.lr, c.ok = lr.cost, lr, true
			if best < 0 || lr.cost < cands[best].cost {
				best = i
			}
		}
		if best < 0 {
			return nil, -1, errors.New("fit: NSS optimisation failed from all starting points")
		}
		return cands, best, nil
	}
	cands, best, err := search()
	if err != nil {
		return nil, err
	}
	if o.Huber > 0 {
		// IRLS from the least-squares solution, then a new global search
		// with the robust weights (the least-squares basin may be a wrong
		// local minimum under the robust loss), then IRLS again
		x := cands[best].x
		if _, err := e.irls(o.Huber, func() bool { return m.eval(x, work.r, nil) },
			func() (lmResult, error) { return levenbergMarquardt(prob, x, settings, work) }, cands[best].lr); err != nil {
			return nil, err
		}
		if cands, best, err = search(); err != nil {
			return nil, err
		}
	}
	bestX, bestRes := cands[best].x, cands[best].lr
	tMax := p.bonds[nb-1].ttm
	altWarning := alternativeWarning(m, cands, best, tMax)

	// robust refinement
	bestRes, err = e.irls(o.Huber,
		func() bool { return m.eval(bestX, work.r, nil) },
		func() (lmResult, error) {
			lr, err := levenbergMarquardt(prob, bestX, settings, work)
			res.Iterations += lr.iterations
			res.Evaluations += lr.evaluations
			return lr, err
		},
		bestRes)
	if err != nil {
		return nil, err
	}

	curve := m.curve(bestX)
	res.Curve = curve
	res.Params = curve.Params()
	res.Objective = bestRes.cost
	res.Converged = bestRes.converged
	res.Residuals, res.RMSEbp, err = report(p, curve)
	if err != nil {
		return nil, err
	}
	res.Warnings = append(res.Warnings, nssWarnings(curve, bestRes.converged, tMax)...)
	if se := m.b0StdErr(bestX, work); se > MaxB0StdErr {
		res.Warnings = append(res.Warnings, fmt.Sprintf("nss: B0 (long-run rate) poorly determined (standard error %.0fbp): extrapolation beyond the longest maturity %.1fy is unidentified", se*1e4, tMax))
	}
	if altWarning != "" {
		res.Warnings = append(res.Warnings, altWarning)
	}
	res.Warnings = append(res.Warnings, outlierWarnings(res.Residuals, res.RMSEbp)...)
	return res, nil
}

// Numbers of grid starts polished by FitNSS.
const (
	polishBest        = 6 // best stage-1 objectives overall
	polishOrientation = 3 // best per orientation (tau1 < tau2, tau1 > tau2)
)

// selectStarts returns the grid starts to polish: the polishBest best
// stage-1 candidates and the polishOrientation best of each ordering of
// the decay parameters (the local minima of NSS typically differ by which
// decay parameter models the short end), in order of the stage-1 cost.
func selectStarts(cands []nssCandidate, nx int) []nssCandidate {
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].cost < cands[j].cost })
	var out []nssCandidate
	perOrient := [2]int{}
	for i, c := range cands {
		o := 0
		if c.x[nx-2] > c.x[nx-1] {
			o = 1
		}
		if i < polishBest || perOrient[o] < polishOrientation {
			out = append(out, c)
		}
		perOrient[o]++
	}
	return out
}

// alternativeWarning reports the best polished candidate whose curve
// differs from the best one (zero rates by more than 0.5bp within the
// maturity range) and whose objective is within AltSolutionTol.
func alternativeWarning(m *nssModel, cands []nssCandidate, best int, tMax float64) string {
	b := m.curve(cands[best].x)
	alt := -1
	for i, c := range cands {
		if i == best || !c.ok || c.cost > cands[best].cost*(1+AltSolutionTol) {
			continue
		}
		if d, _ := maxZeroDiff(m.curve(c.x), b, MinTau, tMax); d <= 0.5 {
			continue
		}
		if alt < 0 || c.cost < cands[alt].cost {
			alt = i
		}
	}
	if alt < 0 {
		return ""
	}
	a := m.curve(cands[alt].x)
	d, at := maxZeroDiff(a, b, MinTau, tMax)
	return fmt.Sprintf("nss: alternative local solution with objective %.6g (best %.6g, +%.2f%%), zero rates differ by up to %.1fbp at %.1fy: b = (%.4f, %.4f, %.4f, %.4f)%%, tau = (%.3f, %.3f)",
		cands[alt].cost, cands[best].cost, (cands[alt].cost/cands[best].cost-1)*100, d, at,
		a.B0*100, a.B1*100, a.B2*100, a.B3*100, a.Tau1, a.Tau2)
}

// maxZeroDiff returns the maximum zero-rate difference (bp) of a and b on
// [t0, t1] and its location.
func maxZeroDiff(a, b term.Curve, t0, t1 float64) (float64, float64) {
	worst, at := 0.0, t0
	for i := 0; i <= 200; i++ {
		t := t0 + (t1-t0)*float64(i)/200
		if d := math.Abs(a.ZeroRate(t)-b.ZeroRate(t)) * 1e4; d > worst {
			worst, at = d, t
		}
	}
	return worst, at
}

// b0StdErr returns the standard error of B0 from the linearised fit at x,
// sigma² (J'J)^-1 with sigma² = |r_bonds|²/(n - p); NaN if undetermined.
func (m *nssModel) b0StdErr(x []float64, work *lmWork) float64 {
	nx, nr, nb := m.nx, m.nr, len(m.e.p.bonds)
	if nb <= nx || !m.eval(x, work.r, work.jac) {
		return math.NaN()
	}
	jtj := make([]float64, nx*nx)
	for i := 0; i < nx; i++ {
		for j := 0; j < nx; j++ {
			v := 0.0
			for k := 0; k < nr; k++ {
				v += work.jac[k*nx+i] * work.jac[k*nx+j]
			}
			jtj[i*nx+j] = v
		}
	}
	inv, ok := invertSPD(jtj, nx)
	if !ok {
		return math.Inf(1)
	}
	// residual variance in the units of the (weighted) residuals, which
	// matches the units of the Jacobian
	sigma2 := 0.0
	for i := 0; i < nb; i++ {
		sigma2 += work.r[i] * work.r[i]
	}
	sigma2 /= float64(nb - nx)
	return math.Sqrt(sigma2 * inv[0])
}

// nssWarnings returns diagnostics of the fitted NSS parameters.
func nssWarnings(n term.NSS, converged bool, tMax float64) []string {
	var w []string
	if !converged {
		w = append(w, "nss: iteration limit reached before convergence")
	}
	for i, tau := range []float64{n.Tau1, n.Tau2} {
		switch {
		case tau <= MinTau*(1+1e-6):
			w = append(w, fmt.Sprintf("nss: tau%d = %.4g at the lower bound %g", i+1, tau, MinTau))
		case tau >= MaxTau*(1-1e-6):
			w = append(w, fmt.Sprintf("nss: tau%d = %.4g at the upper bound %g: the long end beyond the longest maturity %.1fy is set by the bound, not the data", i+1, tau, MaxTau, tMax))
		}
	}
	if r := math.Abs(math.Log(n.Tau1 / n.Tau2)); r <= logSep*(1+1e-3) {
		w = append(w, fmt.Sprintf("nss: separation constraint active: tau1 = %.4g and tau2 = %.4g at the minimum ratio %g; B2 and B3 are collinear and not individually interpretable", n.Tau1, n.Tau2, MinTauRatio))
	}
	if math.Abs(n.B2) > MaxBeta || math.Abs(n.B3) > MaxBeta {
		w = append(w, fmt.Sprintf("nss: beta blow-up: b2 = %.4g%%, b3 = %.4g%%", n.B2*100, n.B3*100))
	}
	return w
}
