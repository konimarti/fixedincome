package fit

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/konimarti/fixedincome/pkg/rate"
	"github.com/konimarti/fixedincome/pkg/term"
)

// engine evaluates the bond residuals of a fit from the discount factors
// (and their parameter gradients) at the cash-flow times. Each fit owns
// its engine; the Prepared set is only read.
type engine struct {
	p   *Prepared
	w   []float64 // residual weights (mean 1)
	obj Objective
	np  int // number of curve parameters
	// flattened cash flows: bond i owns the index range off[i]:off[i+1]
	t, amt []float64
	off    []int
	// z are the discount factors at t, dz their gradients (len(t) x np)
	z, dz []float64
	// ywarm are the model yields of the last evaluation (warm start)
	ywarm []float64
	// errBp are the yield errors (bp) of the last evaluation (approximated
	// by the price error / (D P) for the price objective)
	errBp []float64
	// hw are the robust (Huber IRLS) residual multipliers sqrt(h_i)
	hw   []float64
	grad []float64
}

func newEngine(p *Prepared, o Options, np int) (*engine, error) {
	w, err := p.weights(o.Weights)
	if err != nil {
		return nil, err
	}
	if o.Objective != ObjectiveYield && o.Objective != ObjectivePrice {
		return nil, fmt.Errorf("fit: unknown objective %d", o.Objective)
	}
	e := &engine{p: p, w: w, obj: o.Objective, np: np, off: make([]int, len(p.bonds)+1)}
	for i, b := range p.bonds {
		e.t = append(e.t, b.t...)
		e.amt = append(e.amt, b.amt...)
		e.off[i+1] = len(e.t)
	}
	nb := len(p.bonds)
	e.z = make([]float64, len(e.t))
	e.dz = make([]float64, len(e.t)*np)
	e.ywarm = make([]float64, nb)
	e.errBp = make([]float64, nb)
	e.hw = make([]float64, nb)
	for i, b := range p.bonds {
		e.ywarm[i] = b.yMkt
		e.hw[i] = 1
	}
	e.grad = make([]float64, np)
	return e, nil
}

// residuals writes the n bond residuals to r and, if jac is not nil, the
// rows of the Jacobian with respect to the curve parameters (row stride
// nx >= np, the first np columns) from e.z and e.dz. It returns false if a
// model price is not positive and finite.
//
// Yield objective: r_i = w_i (y_i(theta) - y_i^mkt) with the model yield y_i
// of the model dirty price P_i = sum(CF Z(t)) and, by the implicit function
// theorem, dy_i/dtheta = (dP_i/dtheta) / (dP_i/dy) with dP/dy = -D_i P_i
// (Macaulay duration D_i at y_i). Price objective: r_i = w_i (P_i -
// P_i^mkt) * 100/Face (percent of face). Both are multiplied by the robust
// weights hw_i (1 unless Options.Huber is set).
func (e *engine) residuals(r, jac []float64, nx int) bool {
	np := e.np
	for i, b := range e.p.bonds {
		price := 0.0
		for k := e.off[i]; k < e.off[i+1]; k++ {
			price += e.amt[k] * e.z[k]
		}
		if !(price > 0) || !finiteValue(price) {
			return false
		}
		var scale float64
		w := e.w[i] * e.hw[i]
		switch e.obj {
		case ObjectivePrice:
			r[i] = w * (price - b.dirty) * b.faceFactor
			scale = w * b.faceFactor
			e.errBp[i] = -(price - b.dirty) / (b.duration * b.dirty) * 1e4
		default:
			y, d, ok := modelYield(b.t, b.la, price, e.ywarm[i])
			if !ok {
				return false
			}
			e.ywarm[i] = y
			r[i] = w * (y - b.yMkt)
			scale = -w / (d * price)
			e.errBp[i] = (y - b.yMkt) * 1e4
		}
		if jac == nil {
			continue
		}
		g := e.grad
		for j := range g {
			g[j] = 0
		}
		for k := e.off[i]; k < e.off[i+1]; k++ {
			a := e.amt[k]
			row := e.dz[k*np : (k+1)*np]
			for j := range g {
				g[j] += a * row[j]
			}
		}
		row := jac[i*nx : i*nx+np]
		for j := range g {
			row[j] = scale * g[j]
		}
	}
	return true
}

// updateHuber sets the robust multipliers hw_i = sqrt(min(1, delta/|e_i|))
// from the yield errors e_i (bp) of the last evaluation (iteratively
// reweighted least squares for the Huber loss with threshold delta bp) and
// returns the largest change of a multiplier.
func (e *engine) updateHuber(delta float64) float64 {
	change := 0.0
	for i, eb := range e.errBp {
		h := 1.0
		if a := math.Abs(eb); a > delta {
			h = math.Sqrt(delta / a)
		}
		change = math.Max(change, math.Abs(h-e.hw[i]))
		e.hw[i] = h
	}
	return change
}

// irls runs the Huber IRLS loop: evalAt(x) evaluates the residuals at the
// current solution (updating e.errBp), polish re-solves the weighted least
// squares problem from it. Without Huber (delta <= 0) it does nothing.
func (e *engine) irls(delta float64, evalAt func() bool, polish func() (lmResult, error), last lmResult) (lmResult, error) {
	if delta <= 0 {
		return last, nil
	}
	for it := 0; it < 50; it++ {
		if !evalAt() {
			return last, errors.New("fit: robust reweighting failed")
		}
		if e.updateHuber(delta) < 1e-6 && it > 0 {
			break
		}
		lr, err := polish()
		if err != nil {
			return last, err
		}
		last = lr
	}
	return last, nil
}

// report returns the per-bond residuals and the RMSE of the yield errors
// (bp) of the curve c.
func report(p *Prepared, c term.Curve) ([]Residual, float64, error) {
	out := make([]Residual, len(p.bonds))
	sse := 0.0
	for i, b := range p.bonds {
		dirty := 0.0
		for k := range b.t {
			dirty += b.amt[k] * c.Discount(b.t[k])
		}
		y, _, ok := modelYield(b.t, b.la, dirty, b.yMkt)
		if !ok {
			return nil, math.NaN(), fmt.Errorf("fit: %s: no model yield for the model price %v", b.name(i), dirty)
		}
		ya, _ := rate.FromContinuous(y, 1)
		ff := b.faceFactor
		res := Residual{
			ID: b.ID, Maturity: b.Maturity, TTM: b.ttm, Duration: b.duration,
			CleanMarket: b.CleanPrice, CleanModel: (dirty - b.accrued) * ff,
			DirtyMarket: b.dirty * ff, DirtyModel: dirty * ff,
			YieldMarket: b.yMkt, YieldModel: y,
			YieldMarketAnnual: b.yMktAnnual, YieldModelAnnual: ya,
			ErrBp: (y - b.yMkt) * 1e4,
		}
		out[i] = res
		sse += res.ErrBp * res.ErrBp
	}
	return out, math.Sqrt(sse / float64(len(out))), nil
}

// OutlierFactor is the multiple of the RMSE above which the yield error of
// a bond is reported as a possible outlier.
const OutlierFactor = 4.0

// outlierWarnings flags bonds with |ErrBp| > OutlierFactor*RMSE.
func outlierWarnings(res []Residual, rmse float64) []string {
	var w []string
	if len(res) < 5 || !(rmse > 0) {
		return nil
	}
	for _, r := range res {
		if math.Abs(r.ErrBp) > OutlierFactor*rmse {
			w = append(w, fmt.Sprintf("%s: yield error %.2fbp exceeds %g x RMSE (%.2fbp): possible outlier or stale price", r.ID, r.ErrBp, OutlierFactor, rmse))
		}
	}
	return w
}

// Residual is the fit result for one bond. Prices are in percent of face
// (as quoted); yields are decimals.
type Residual struct {
	// ID is the bond ID
	ID string
	// Maturity is the maturity date
	Maturity time.Time
	// TTM is the time to maturity in years (ACT/365F)
	TTM float64
	// Duration is the Macaulay duration at the market yield (years)
	Duration float64
	// CleanMarket/CleanModel are the quoted and model clean prices
	CleanMarket, CleanModel float64
	// DirtyMarket/DirtyModel are the market and model dirty prices
	DirtyMarket, DirtyModel float64
	// YieldMarket/YieldModel are the continuously compounded yields
	// (decimal) of the market and model dirty prices
	YieldMarket, YieldModel float64
	// YieldMarketAnnual/YieldModelAnnual are their annually compounded
	// equivalents (decimal)
	YieldMarketAnnual, YieldModelAnnual float64
	// ErrBp is the yield error YieldModel - YieldMarket in basis points
	ErrBp float64
}

// Result is the outcome of a fit.
type Result struct {
	// Curve is the fitted curve (term.NSS or *term.Spline)
	Curve term.Curve
	// Params are the fitted parameters: (B0, B1, B2, B3, Tau1, Tau2) for
	// NSS (decimals, years), the continuously compounded zero rates
	// (decimal) at Knots for a spline
	Params []float64
	// Knots are the spline knots in years (nil for NSS)
	Knots []float64
	// Lambda is the smoothing parameter used by FitSpline
	Lambda float64
	// Residuals are the per-bond results, sorted by time to maturity
	Residuals []Residual
	// RMSEbp is the root mean squared yield error (bp, unweighted)
	RMSEbp float64
	// Objective is the final value of the minimised objective 1/2 sum(r²)
	// including penalties (yield residuals in decimals; price residuals in
	// percent of face)
	Objective float64
	// Iterations and Evaluations count the Levenberg-Marquardt iterations
	// and residual evaluations of all runs (multi-start, cross-validation)
	Iterations, Evaluations int
	// Converged reports whether the final optimisation met a convergence
	// criterion (rather than the iteration limit)
	Converged bool
	// Warnings are diagnostics: dropped bonds, parameters at bounds,
	// implausible parameters
	Warnings []string
}
