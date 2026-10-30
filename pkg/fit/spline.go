package fit

import (
	"errors"
	"fmt"
	"math"

	"github.com/konimarti/fixedincome/pkg/term"
)

// lambdaGrid are the smoothing parameters searched by generalized
// cross-validation (Options.Lambda < 0).
var lambdaGrid = func() []float64 {
	g := make([]float64, 0, 25)
	for e := -8.0; e <= 4.0+1e-9; e += 0.5 {
		g = append(g, math.Pow(10, e))
	}
	return g
}()

// splineModel is a natural cubic spline on the log-discount factors (as
// term.Spline) with the zero rates x_k at the knots t_k as parameters.
// Since spline interpolation is linear in the knot values, ln Z(t) =
// -sum_k G[t,k] x_k with the basis G[t,k] = t_k * B_k(t), where B_k is the
// term.Spline through the unit log-discount factors e_k (computed once).
type splineModel struct {
	e     *engine
	knots []float64
	g     []float64 // basis at the cash-flow times (len(t) x nx)
	f0    []float64 // short rate r(0) = sum f0_k x_k
	on    *float64
	sOn   float64
	nx    int
	// penalty rows (second divided differences of the zero rates)
	pen    []float64 // (nx-2) x nx, unscaled
	lambda float64
	nPen   int
}

func newSplineModel(e *engine, knots []float64, on *float64) (*splineModel, error) {
	nx := len(knots)
	m := &splineModel{e: e, knots: knots, nx: nx, on: on, g: make([]float64, len(e.t)*nx), f0: make([]float64, nx)}
	z := make([]float64, nx)
	for k := 0; k < nx; k++ {
		for j := range z {
			z[j] = 1
		}
		z[k] = math.Exp(-1)
		s, err := term.NewSpline(knots, z)
		if err != nil {
			return nil, err
		}
		for i, t := range e.t {
			m.g[i*nx+k] = -math.Log(s.Discount(t)) * knots[k]
		}
		m.f0[k] = s.InstForward(0) * knots[k]
	}
	if nx >= 3 {
		m.nPen = nx - 2
		m.pen = make([]float64, m.nPen*nx)
		for j := 1; j < nx-1; j++ {
			h0, h1 := knots[j]-knots[j-1], knots[j+1]-knots[j]
			c := 2 / (h0 + h1)
			w := math.Sqrt((h0 + h1) / 2) // trapezoidal weight of the integral
			row := m.pen[(j-1)*nx : j*nx]
			row[j-1] = w * c / h0
			row[j] = -w * c * (1/h0 + 1/h1)
			row[j+1] = w * c / h1
		}
	}
	m.sOn = 10 * math.Sqrt(float64(len(e.p.bonds)))
	return m, nil
}

func (m *splineModel) nr() int {
	n := len(m.e.p.bonds) + m.nPen
	if m.on != nil {
		n++
	}
	return n
}

func (m *splineModel) eval(x, r, jac []float64) bool {
	e, nx := m.e, m.nx
	for i := range e.t {
		row := m.g[i*nx : (i+1)*nx]
		ly := 0.0
		for k, v := range row {
			ly += v * x[k]
		}
		z := math.Exp(-ly)
		if !(z > 0) || math.IsInf(z, 0) {
			return false
		}
		e.z[i] = z
		if jac != nil {
			drow := e.dz[i*nx : (i+1)*nx]
			for k, v := range row {
				drow[k] = -z * v
			}
		}
	}
	if !e.residuals(r, jac, nx) {
		return false
	}
	nb := len(e.p.bonds)
	sl := math.Sqrt(m.lambda)
	for j := 0; j < m.nPen; j++ {
		row := m.pen[j*nx : (j+1)*nx]
		v := 0.0
		for k := range row {
			v += row[k] * x[k]
		}
		r[nb+j] = sl * v
		if jac != nil {
			for k := range row {
				jac[(nb+j)*nx+k] = sl * row[k]
			}
		}
	}
	if m.on != nil {
		i := nb + m.nPen
		v := 0.0
		for k := range m.f0 {
			v += m.f0[k] * x[k]
		}
		r[i] = m.sOn * (v - *m.on)
		if jac != nil {
			for k := range m.f0 {
				jac[i*nx+k] = m.sOn * m.f0[k]
			}
		}
	}
	return true
}

// curve returns the term.Spline of the parameters x.
func (m *splineModel) curve(x []float64) (*term.Spline, error) {
	z := make([]float64, m.nx)
	for k, t := range m.knots {
		z[k] = math.Exp(-x[k] * t)
	}
	return term.NewSpline(m.knots, z)
}

// gcv returns the generalized cross-validation score n*RSS/(n - tr H)² of
// the linearised fit at x, with the hat matrix H = Jb (J'J)^-1 Jb' of the
// bond rows Jb (J includes the penalty rows).
func (m *splineModel) gcv(x []float64, work *lmWork) float64 {
	nx, nr, nb := m.nx, m.nr(), len(m.e.p.bonds)
	if !m.eval(x, work.r, work.jac) {
		return math.Inf(1)
	}
	a := make([]float64, nx*nx)
	b := make([]float64, nx*nx)
	for i := 0; i < nx; i++ {
		for j := 0; j < nx; j++ {
			sa, sb := 0.0, 0.0
			for k := 0; k < nr; k++ {
				v := work.jac[k*nx+i] * work.jac[k*nx+j]
				sa += v
				if k < nb {
					sb += v
				}
			}
			a[i*nx+j], b[i*nx+j] = sa, sb
		}
	}
	inv, ok := invertSPD(a, nx)
	if !ok {
		return math.Inf(1)
	}
	tr := 0.0
	for i := 0; i < nx; i++ {
		for j := 0; j < nx; j++ {
			tr += inv[i*nx+j] * b[j*nx+i]
		}
	}
	rss := sumSq(work.r[:nb])
	df := float64(nb) - tr
	if !(df > 0.5) {
		return math.Inf(1)
	}
	return float64(nb) * rss / (df * df)
}

// splineKnots returns the knots for the sorted times to maturity: a short
// knot at half the shortest maturity (if that is at least minSpacing) and
// n knots from the shortest to the longest maturity, at least minSpacing
// years apart. Knot j (j = 0..n-1) is the geometric mean of the j/(n-1)
// quantile of the maturities and the point j/(n-1) of the way from the
// shortest to the longest maturity on a logarithmic scale; the blend keeps
// knots where the bonds are while avoiding large gaps in log maturity
// (e.g. at the short end of a data set with few bills).
//
// The short knot relaxes the natural boundary condition of the spline at
// t = 0 (zero slope of the forward curve), which otherwise biases steep
// short ends.
func splineKnots(ttm []float64, n int) []float64 {
	const minSpacing = 0.05
	var knots []float64
	if ttm[0]/2 >= minSpacing && n >= 2 {
		knots = append(knots, ttm[0]/2)
	}
	if n < 2 {
		n = 2
	}
	lo, hi := math.Log(ttm[0]), math.Log(ttm[len(ttm)-1])
	for j := 0; j < n; j++ {
		f := float64(j) / float64(n-1)
		pos := f * float64(len(ttm)-1)
		i := int(math.Floor(pos))
		if i >= len(ttm)-1 {
			i = len(ttm) - 1
		}
		q := ttm[i]
		if i+1 < len(ttm) {
			q += (pos - float64(i)) * (ttm[i+1] - ttm[i])
		}
		k := math.Sqrt(q * math.Exp(lo+f*(hi-lo)))
		if j == 0 {
			k = ttm[0]
		}
		if j == n-1 {
			k = ttm[len(ttm)-1]
		}
		if len(knots) > 0 && k-knots[len(knots)-1] < minSpacing {
			if j == n-1 && len(knots) > 1 {
				// keep the longest maturity as the last knot
				knots[len(knots)-1] = math.Max(k, knots[len(knots)-1])
			}
			continue
		}
		knots = append(knots, k)
	}
	return knots
}

// FitSpline fits a natural cubic spline on the log-discount factors (see
// term.Spline) to the prepared bonds by minimising the weighted squared
// yield errors plus the smoothing penalty Lambda * sum_j w_j (r”_j)²,
// where r”_j are the second divided differences of the zero rates at the
// knots and w_j the trapezoidal weights (an approximation of the integral of
// the squared second derivative of the zero-rate curve). The parameters are
// the continuously compounded zero rates at the knots.
//
// Knots: Options.Knots, or Options.NumKnots (default n/3 clamped to [5, 8])
// knots from the shortest to the longest maturity, placed at a blend of the
// maturity quantiles and a logarithmic grid (see splineKnots), plus a
// short knot at half the shortest maturity; with the anchor (0, 1) of
// term.Spline the curve has up to 8 + 1 + 1 nodes. Beyond the last knot
// the forward rate is flat.
//
// Options.Lambda nil or < 0 (default) selects the smoothing by generalized
// cross-validation on a logarithmic grid from 1e-8 to 1e4, 0 disables it.
// With Options.OvernightRate the short rate r(0) is tied to it by a stiff
// penalty. With Options.Huber the solution at the selected Lambda is
// refined by IRLS. The fit is solved by Levenberg-Marquardt with the
// analytic Jacobian (the spline is linear in the log-discount factors).
func FitSpline(p *Prepared, o Options) (*Result, error) {
	if p == nil || len(p.bonds) == 0 {
		return nil, errors.New("fit: no bonds")
	}
	lambda := LambdaGCV
	if o.Lambda != nil {
		lambda = *o.Lambda
	}
	if !finiteValue(lambda) {
		return nil, fmt.Errorf("fit: invalid lambda %v", lambda)
	}
	if !finiteValue(o.Huber) || o.Huber < 0 {
		return nil, fmt.Errorf("fit: invalid Huber threshold %v", o.Huber)
	}
	if o.OvernightRate != nil && !finiteValue(*o.OvernightRate) {
		return nil, fmt.Errorf("fit: invalid overnight rate %v", *o.OvernightRate)
	}
	nb := len(p.bonds)
	knots := append([]float64(nil), o.Knots...)
	if len(knots) == 0 {
		n := o.NumKnots
		if n <= 0 {
			n = nb / 3
			if n < 5 {
				n = 5
			}
			if n > 8 {
				n = 8
			}
		}
		if n > nb-2 {
			n = nb - 2
		}
		ttm := make([]float64, nb)
		for i, b := range p.bonds {
			ttm[i] = b.ttm
		}
		knots = splineKnots(ttm, n)
	}
	for i, k := range knots {
		if !finiteValue(k) || !(k > 0) || (i > 0 && !(k-knots[i-1] >= term.MinKnotSpacing)) {
			return nil, fmt.Errorf("fit: knots must be positive and strictly increasing: %v", knots)
		}
	}
	if len(knots) < 2 {
		return nil, fmt.Errorf("fit: at least 2 spline knots required, got %v", knots)
	}
	if nb < len(knots) && lambda == 0 {
		return nil, fmt.Errorf("fit: too few bonds (%d) for an unpenalised spline with %d knots", nb, len(knots))
	}
	nx := len(knots)
	e, err := newEngine(p, o, nx)
	if err != nil {
		return nil, err
	}
	m, err := newSplineModel(e, knots, o.OvernightRate)
	if err != nil {
		return nil, err
	}
	nr := m.nr()
	prob := lmProblem{nx: nx, nr: nr, eval: m.eval}
	work := newLMWork(nx, nr)
	settings := lmSettings{maxIter: o.maxIter(), xtol: 1e-12, ftol: 1e-15, gtol: 1e-22}
	res := &Result{Warnings: p.Warnings(), Knots: append([]float64(nil), knots...)}

	// start: market yields interpolated at the knots over the durations
	x0 := make([]float64, nx)
	for k, t := range knots {
		x0[k] = interpYield(p, t)
	}

	solve := func(lambda float64, x []float64) (lmResult, error) {
		m.lambda = lambda
		lr, err := levenbergMarquardt(prob, x, settings, work)
		res.Iterations += lr.iterations
		res.Evaluations += lr.evaluations
		return lr, err
	}

	// fitAll solves for the fixed lambda or selects it by GCV (with the
	// current robust weights)
	fitAll := func() ([]float64, lmResult, float64, error) {
		if lambda >= 0 {
			x := append([]float64(nil), x0...)
			lr, err := solve(lambda, x)
			if err != nil {
				return nil, lr, 0, fmt.Errorf("fit: spline optimisation failed: %w", err)
			}
			return x, lr, lambda, nil
		}
		var x []float64
		var lr lmResult
		bestLambda, bestScore := 0.0, math.Inf(1)
		xw := append([]float64(nil), x0...)
		// from smooth to rough: warm starts follow the path of solutions
		for i := len(lambdaGrid) - 1; i >= 0; i-- {
			lam := lambdaGrid[i]
			l, err := solve(lam, xw)
			if err != nil {
				copy(xw, x0)
				continue
			}
			if score := m.gcv(xw, work); score < bestScore {
				bestScore = score
				x = append(x[:0], xw...)
				lr = l
				bestLambda = lam
			}
		}
		if x == nil {
			return nil, lr, 0, errors.New("fit: spline optimisation failed for all smoothing parameters")
		}
		m.lambda = bestLambda
		return x, lr, bestLambda, nil
	}
	x, lr, lam, err := fitAll()
	if err != nil {
		return nil, err
	}
	polish := func() (lmResult, error) { return solve(lam, x) }
	evalAt := func() bool { return m.eval(x, work.r, nil) }
	if o.Huber > 0 && lambda < 0 {
		// select lambda again with the robust weights of a first IRLS pass
		if _, err := e.irls(o.Huber, evalAt, polish, lr); err != nil {
			return nil, err
		}
		if x, lr, lam, err = fitAll(); err != nil {
			return nil, err
		}
	}
	res.Lambda = lam
	lr, err = e.irls(o.Huber, evalAt, polish, lr)
	if err != nil {
		return nil, err
	}
	curve, err := m.curve(x)
	if err != nil {
		return nil, fmt.Errorf("fit: invalid fitted spline: %w", err)
	}
	res.Curve = curve
	res.Params = append([]float64(nil), x...)
	res.Objective = lr.cost
	res.Converged = lr.converged
	res.Residuals, res.RMSEbp, err = report(p, curve)
	if err != nil {
		return nil, err
	}
	if !lr.converged {
		res.Warnings = append(res.Warnings, "spline: iteration limit reached before convergence")
	}
	res.Warnings = append(res.Warnings, curveWarnings(curve, knots[len(knots)-1])...)
	res.Warnings = append(res.Warnings, outlierWarnings(res.Residuals, res.RMSEbp)...)
	return res, nil
}

// interpYield interpolates the market yields linearly over the bond
// durations at t (flat outside).
func interpYield(p *Prepared, t float64) float64 {
	bs := p.bonds
	lo, hi := -1, -1
	for i, b := range bs {
		if b.duration <= t && (lo < 0 || b.duration > bs[lo].duration) {
			lo = i
		}
		if b.duration >= t && (hi < 0 || b.duration < bs[hi].duration) {
			hi = i
		}
	}
	switch {
	case lo < 0:
		return bs[hi].yMkt
	case hi < 0 || bs[hi].duration == bs[lo].duration:
		return bs[lo].yMkt
	}
	w := (t - bs[lo].duration) / (bs[hi].duration - bs[lo].duration)
	return bs[lo].yMkt + w*(bs[hi].yMkt-bs[lo].yMkt)
}

// curveWarnings checks the discount factors and forward rates of c on
// [0, tMax]: non-positive or non-finite discount factors, implausible
// forward rates (|f| > 25%) and an oscillating forward curve.
func curveWarnings(c term.Curve, tMax float64) []string {
	var w []string
	const steps = 400
	signChanges := 0
	prevSlope := 0.0
	prevF := term.InstForward(c, 0)
	maxF := math.Abs(prevF)
	for i := 1; i <= steps; i++ {
		t := tMax * float64(i) / steps
		if z := c.Discount(t); !(z > 0) || math.IsInf(z, 0) {
			return append(w, fmt.Sprintf("spline: invalid discount factor %v at t = %.3g", z, t))
		}
		f := term.InstForward(c, t)
		maxF = math.Max(maxF, math.Abs(f))
		if slope := f - prevF; math.Abs(slope) > 1e-9 {
			if prevSlope != 0 && (slope > 0) != (prevSlope > 0) {
				signChanges++
			}
			prevSlope = slope
		}
		prevF = f
	}
	if maxF > 0.25 {
		w = append(w, fmt.Sprintf("spline: implausible forward rate %.3g%% within the maturity range", maxF*100))
	}
	if signChanges > 3 {
		w = append(w, fmt.Sprintf("spline: the forward curve oscillates (%d turning points)", signChanges))
	}
	return w
}
