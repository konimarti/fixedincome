package cli

import (
	"flag"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/konimarti/fixedincome/pkg/mc"
	"github.com/konimarti/fixedincome/pkg/mc/model/holee"
	"github.com/konimarti/fixedincome/pkg/mc/model/hullwhite"
	"github.com/konimarti/fixedincome/pkg/mc/model/vasicek"
	"github.com/konimarti/fixedincome/pkg/term"
)

// simParams are the settings of a zero bond simulation (decimals).
type simParams struct {
	model      string
	curve      term.Curve
	sigma, a   float64
	horizon    float64
	steps      int
	paths      int
	seed       uint64
	antithetic bool
}

// simNode is the result at one grid node.
type simNode struct {
	t float64
	// curve quantities
	curveZero, curveFwd, curveDF float64
	// modelDF is the analytic model discount factor (the curve for the
	// arbitrage-free models)
	modelDF float64
	// MC estimate, its standard error and zero rate
	mcDF, mcSE, mcZero float64
	// theta and phi are the drift and E[r(t)] (Ho-Lee only)
	theta, phi float64
}

// simResult is the result of a simulation.
type simResult struct {
	grid  *mc.Grid
	nodes []simNode
	r0    float64
	seed  uint64
	// notes are diagnostics of the calibration
	notes []string
}

// simModels are the names of the supported models.
var simModels = []string{"holee", "hullwhite", "vasicek"}

// simulate prices the zero bonds of the short rate model on the grid by
// Monte Carlo.
func simulate(p simParams) (*simResult, error) {
	g, err := mc.NewUniformGrid(p.horizon, p.steps)
	if err != nil {
		return nil, err
	}
	res := &simResult{grid: g, seed: p.seed}
	var model mc.Model
	var modelDF func(t float64) float64
	var theta, phi func(t float64) float64
	switch p.model {
	case "holee":
		m, err := holee.New(p.curve, p.sigma)
		if err != nil {
			return nil, err
		}
		model, res.r0, theta, phi = m, m.R0(), m.Theta, m.Phi
	case "hullwhite":
		m, err := hullwhite.New(p.curve, p.a, p.sigma)
		if err != nil {
			return nil, err
		}
		model, res.r0, theta, phi = m, m.R0(), m.Theta, m.Phi
	case "vasicek":
		var mats []float64
		for i := 1; i <= 20; i++ {
			mats = append(mats, p.horizon*float64(i)/20)
		}
		m, cal, err := vasicek.Calibrate(p.curve, p.sigma, mats)
		if err != nil {
			return nil, err
		}
		model, res.r0 = m, m.R0
		modelDF = func(t float64) float64 { return math.Exp(-m.ZeroRate(t) * t) }
		res.notes = append(res.notes, fmt.Sprintf("Vasicek calibration: rbar=%.4f%%, gamma=%.4f, RMSE of the zero rates %.2f bp", toPct(m.Rbar), m.Gamma, toBP(cal.RMSE)))
		if cal.AtBound {
			res.notes = append(res.notes, "Vasicek calibration: the mean reversion is at its bound; the curve is better described by hullwhite or holee")
		}
	default:
		return nil, usagef("invalid --model %q (%s)", p.model, strings.Join(simModels, ", "))
	}
	mcres, err := mc.PriceMulti(model, mc.ZeroBonds{Grid: g}, g, mc.Options{Paths: p.paths, Seed: p.seed, Antithetic: p.antithetic})
	if err != nil {
		return nil, err
	}
	for i, t := range g.Times {
		n := simNode{t: t, curveZero: p.curve.ZeroRate(t), curveFwd: term.InstForward(p.curve, t), curveDF: p.curve.Discount(t),
			mcDF: mcres[i].Mean, mcSE: mcres[i].StdErr, theta: math.NaN(), phi: math.NaN()}
		n.modelDF = n.curveDF
		if modelDF != nil {
			n.modelDF = modelDF(t)
		}
		n.mcZero = res.r0
		if t > 0 {
			n.mcZero = -math.Log(n.mcDF) / t
		}
		if theta != nil {
			n.theta, n.phi = theta(t), phi(t)
		}
		res.nodes = append(res.nodes, n)
	}
	return res, nil
}

// zscore returns (mc - model)/se, NaN if the standard error is at rounding
// level or all paths underflow.
func (n simNode) zscore() float64 {
	if !(n.mcDF > 1e-8*n.modelDF) || !(n.mcSE > 1e-12*n.mcDF) {
		return math.NaN()
	}
	return (n.mcDF - n.modelDF) / n.mcSE
}

// writeSimSummary prints the summary and the warnings to w (stderr).
func writeSimSummary(w io.Writer, p simParams, r *simResult) {
	g := r.grid
	fmt.Fprintf(w, "%s model: T=%v, N=%d, dt=%v, sigma=%v, r0=%.6f, paths=%d, seed=%d\n",
		p.model, g.T(), g.Steps(), g.Dt(0), p.sigma, r.r0, p.paths, r.seed)
	for _, n := range r.notes {
		fmt.Fprintln(w, n)
	}
	fmt.Fprintf(w, "%8s %12s %12s %10s %8s\n", "t", "100*Z(0,t)", "100*E[Df]", "100*SE", "dev/SE")
	step := g.Steps() / 10
	if step < 1 {
		step = 1
	}
	underflow, tinySE, unreliable := false, false, false
	for i := step; i <= g.Steps(); i += step {
		n := r.nodes[i]
		dev := "n/a"
		switch {
		case !(n.mcDF > 1e-8*n.modelDF):
			underflow = true
		case !(n.mcSE > 1e-12*n.mcDF):
			tinySE = p.sigma > 0
		default:
			d := n.zscore()
			dev = fmt.Sprintf("%.2f", d)
			if math.Abs(d) > 1000 {
				dev = fmt.Sprintf("%.1e", d)
				unreliable = true
			}
		}
		fmt.Fprintf(w, "%8.4g %12.6f %12.6f %10.6f %8s\n", n.t, 100*n.modelDF, 100*n.mcDF, 100*n.mcSE, dev)
	}
	if underflow {
		fmt.Fprintln(w, "warning: simulated discount factors underflow (sigma too large for the horizon); the estimates are meaningless")
	}
	if unreliable {
		fmt.Fprintln(w, "warning: Monte Carlo estimates differ from the model by more than 1000 standard errors: the discount factors are too skewed (sigma too large) for this number of paths")
	}
	if tinySE {
		fmt.Fprintln(w, "warning: standard errors are at rounding level (horizon too short for sigma); dev/SE not shown")
	}
}

func buildSimZCB(e *env) (*flag.FlagSet, func() error) {
	fs := flag.NewFlagSet("sim zcb", flag.ContinueOnError)
	var cf curveFlags
	var ff formatFlags
	cf.register(fs, false)
	ff.register(fs)
	model := fs.String("model", "holee", "short rate model: "+strings.Join(simModels, "|"))
	sigma := fs.Float64("sigma", math.NaN(), "volatility of the short rate in percent per square root of a year (2 = 200bp)")
	a := fs.Float64("mean-reversion", 0, "mean reversion speed per year (hullwhite)")
	horizon := fs.String("horizon", "10y", "horizon of the simulation (tenor or years)")
	steps := fs.Int("steps", 120, "number of time steps")
	paths := fs.Int("paths", 10000, "number of Monte Carlo paths")
	seed := fs.Uint64("seed", 1, "random seed (0: time-based; printed on stderr)")
	anti := fs.Bool("antithetic", false, "use antithetic variates (needs an even number of paths)")
	return fs, func() error {
		set := setFlags(fs)
		if !set["sigma"] {
			return usagef("--sigma is required (percent, e.g. 2 for 200bp)")
		}
		name := strings.ToLower(*model)
		ok := false
		for _, m := range simModels {
			ok = ok || m == name
		}
		if !ok {
			return usagef("invalid --model %q (%s)", *model, strings.Join(simModels, ", "))
		}
		if !(*sigma >= 0) || math.IsInf(*sigma, 0) {
			return usagef("invalid --sigma %v: must be >= 0 (percent)", *sigma)
		}
		if set["mean-reversion"] && name != "hullwhite" {
			return usagef("--mean-reversion applies to --model hullwhite only (vasicek calibrates it, holee has none)")
		}
		if !(*a >= 0) || math.IsInf(*a, 0) {
			return usagef("invalid --mean-reversion %v: must be >= 0", *a)
		}
		h, err := parseYears(*horizon)
		if err != nil || !(h > 0) {
			return usagef("invalid --horizon %q (examples: 10y, 5)", *horizon)
		}
		if *steps < 1 || *steps > 10_000_000 {
			return usagef("invalid --steps %d", *steps)
		}
		if *paths < 2 || (*anti && *paths%2 != 0) {
			return usagef("invalid --paths %d (at least 2; even with --antithetic)", *paths)
		}
		fmtOut, err := resolveFormat(ff.format, true, e.stdout, ff.out != "" && ff.out != "-")
		if err != nil {
			return err
		}
		l, err := cf.load(e)
		if err != nil {
			return err
		}
		s := *seed
		if s == 0 {
			s = uint64(time.Now().UnixNano())
		}
		out, err := openOutput(ff.out, e.stdout, cf.path)
		if err != nil {
			return err
		}
		p := simParams{model: name, curve: l.curve, sigma: fromPct(*sigma), a: *a, horizon: h, steps: *steps, paths: *paths, seed: s, antithetic: *anti}
		r, err := simulate(p)
		if err != nil {
			return out.finish(err)
		}
		t := &table{cols: []string{"t_years", "curve_zero_cc_pct", "curve_fwd_cc_pct", "curve_discount", "model_discount",
			"mc_discount", "mc_stderr", "mc_zero_cc_pct", "z_score"}}
		for _, n := range r.nodes {
			t.add(n.t, toPct(n.curveZero), toPct(n.curveFwd), n.curveDF, n.modelDF, n.mcDF, n.mcSE, toPct(n.mcZero), n.zscore())
		}
		if err := out.finish(writeTable(out.w, t, fmtOut)); err != nil {
			return err
		}
		writeSimSummary(e.stderr, p, r)
		return nil
	}
}
