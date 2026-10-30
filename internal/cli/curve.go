package cli

import (
	"errors"
	"flag"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/konimarti/fixedincome/pkg/fit"
	"github.com/konimarti/fixedincome/pkg/rate"
	"github.com/konimarti/fixedincome/pkg/term"
)

// fitRequest is a curve fit of one or two models.
type fitRequest struct {
	bonds           []fit.Bond
	opts            fit.Options
	doNSS, doSpline bool
}

// fitOutcome is the result of one model.
type fitOutcome struct {
	model   string
	res     *fit.Result
	err     error
	elapsed time.Duration
	// nPrepared is the number of bonds used
	nPrepared int
}

// runFits prepares the bonds and fits the requested models independently;
// warn receives the diagnostics.
func runFits(req fitRequest, warn func(string)) (*fit.Prepared, []fitOutcome, error) {
	p, err := fit.PrepareBonds(req.bonds, req.opts)
	if err != nil {
		return nil, nil, err
	}
	for _, w := range p.Warnings() {
		warn(w)
	}
	var out []fitOutcome
	for _, m := range []struct {
		name string
		do   bool
		fit  func(*fit.Prepared, fit.Options) (*fit.Result, error)
	}{{term.ModelNSS, req.doNSS, fit.FitNSS}, {term.ModelSpline, req.doSpline, fit.FitSpline}} {
		if !m.do {
			continue
		}
		start := time.Now()
		res, err := m.fit(p, req.opts)
		o := fitOutcome{model: m.name, res: res, err: err, elapsed: time.Since(start), nPrepared: p.Len()}
		if err == nil {
			for _, w := range res.Warnings[min(len(p.Warnings()), len(res.Warnings)):] {
				warn(w)
			}
		}
		out = append(out, o)
	}
	return p, out, nil
}

// residualHeader are the columns of the residuals file (rates in percent,
// prices in percent of face).
var residualHeader = []string{"model", "id", "maturity", "ttm_years", "price_clean_market", "price_clean_model",
	"yield_cc_market_pct", "yield_cc_model_pct", "yield_annual_market_pct", "yield_annual_model_pct", "error_bp",
	"zero_cc_model_pct", "discount_factor_model"}

// residualRows converts the residuals of the outcomes to table rows sorted by
// time to maturity (then model).
func residualRows(outs []fitOutcome) *table {
	type row struct {
		model string
		r     fit.Residual
		c     term.Curve
	}
	var rows []row
	for _, o := range outs {
		if o.err != nil {
			continue
		}
		for _, r := range o.res.Residuals {
			rows = append(rows, row{o.model, r, o.res.Curve})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].r.TTM != rows[j].r.TTM {
			return rows[i].r.TTM < rows[j].r.TTM
		}
		return rows[i].model < rows[j].model
	})
	t := &table{cols: residualHeader}
	for _, x := range rows {
		r := x.r
		t.add(x.model, r.ID, formatDate(r.Maturity), r.TTM, r.CleanMarket, r.CleanModel,
			toPct(r.YieldMarket), toPct(r.YieldModel), toPct(r.YieldMarketAnnual), toPct(r.YieldModelAnnual), r.ErrBp,
			toPct(x.c.ZeroRate(r.TTM)), x.c.Discount(r.TTM))
	}
	return t
}

// loadStart reads the NSS starting point from a curve file (the spread is
// added to B0: a z-spread shifts all NSS zero rates).
func loadStart(e *env, path string) (*term.NSS, error) {
	c, _, err := readCurveFile(e, path)
	if err != nil {
		return nil, err
	}
	base, spread := term.Spread(c)
	n, ok := base.(term.NSS)
	if !ok {
		return nil, fmt.Errorf("%s: not an NSS curve (%T)", path, base)
	}
	n.B0 += spread
	return &n, nil
}

// parseLambda parses --lambda: "gcv" or a number >= 0.
func parseLambda(s string) (*float64, error) {
	if strings.EqualFold(strings.TrimSpace(s), "gcv") {
		return fit.Lambda(fit.LambdaGCV), nil
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return nil, usagef("invalid --lambda %q (gcv or a number >= 0)", s)
	}
	return fit.Lambda(v), nil
}

// fitBondsFromRows converts the valid rows of a bond file to fit bonds;
// rows with problems are skipped and reported to warn.
func fitBondsFromRows(bf *bondFile, warn func(string)) []fit.Bond {
	var bonds []fit.Bond
	skipped := 0
	for _, b := range bf.rows {
		err := b.err
		if err == nil && !b.hasPrice {
			err = errMissingPrice
		}
		if err != nil {
			skipped++
			if skipped <= 10 {
				warn(fmt.Sprintf("line %d (%s) skipped: %v", b.line, b.id, err))
			}
			continue
		}
		bonds = append(bonds, fit.Bond{ID: b.id, Maturity: b.maturity, Coupon: b.coupon, Frequency: b.freq,
			DayCount: b.dc, CleanPrice: b.price, Face: b.face, Weight: b.weight})
	}
	if skipped > 10 {
		warn(fmt.Sprintf("... and %d more rows skipped", skipped-10))
	}
	if skipped > 0 {
		warn(fmt.Sprintf("%d of %d rows skipped", skipped, len(bf.rows)))
	}
	return bonds
}

func buildCurveFit(e *env) (*flag.FlagSet, func() error) {
	fs := flag.NewFlagSet("curve fit", flag.ContinueOnError)
	bonds := fs.String("bonds", "", "bond CSV file (- for stdin): maturity,coupon_pct,price_clean[,id,freq,daycount,face]")
	date := fs.String("date", "", "settlement date of the prices = curve date, YYYY-MM-DD or DD.MM.YYYY (default: today)")
	model := fs.String("model", "nss", "curve model nss|spline|both (both needs -o and writes <stem>.nss.json and <stem>.spline.json)")
	onrate := fs.Float64("onrate", 0, "overnight rate in percent (simple ACT/360, e.g. SARON): constrains the short end; unset: no constraint")
	start := fs.String("start", "", "curve file with an additional NSS starting point (e.g. yesterday's curve)")
	minTTM := fs.Float64("min-ttm", fit.DefaultMinTTM, "minimum time to maturity in years of the bonds used (<= 0: all)")
	lambda := fs.String("lambda", "gcv", "spline smoothing: gcv (cross-validation) or a number >= 0")
	objective := fs.String("objective", "yield", "fit objective yield|price")
	huber := fs.Float64("huber", 0, "Huber threshold in bp for a robust fit (0: least squares)")
	lag := fs.Int("settle-lag", 0, "calendar days added to --date to obtain the settlement date")
	out := fs.String("o", "-", "output curve JSON file (- for stdout)")
	resid := fs.String("residuals", "", "write the per-bond residuals as CSV to this file")
	return fs, func() error {
		set := setFlags(fs)
		if *bonds == "" {
			return usagef("--bonds is required")
		}
		var doNSS, doSpline bool
		switch strings.ToLower(*model) {
		case "nss":
			doNSS = true
		case "spline":
			doSpline = true
		case "both":
			doNSS, doSpline = true, true
		default:
			return usagef("invalid --model %q (nss, spline, both)", *model)
		}
		both := doNSS && doSpline
		if both && (*out == "-" || *out == "") {
			return usagef("--model both needs -o FILE (writes <stem>.nss.json and <stem>.spline.json)")
		}
		if *resid == "-" && (*out == "-" || *out == "") {
			return usagef("--residuals - and the curve on stdout cannot be combined")
		}
		obj, err := fit.ParseObjective(*objective)
		if err != nil {
			return usagef("invalid --objective %q (yield, price)", *objective)
		}
		lam, err := parseLambda(*lambda)
		if err != nil {
			return err
		}
		if !(*huber >= 0) || math.IsInf(*huber, 0) {
			return usagef("invalid --huber %v: must be >= 0", *huber)
		}
		if *lag < 0 || *lag > 31 {
			return usagef("invalid --settle-lag %d", *lag)
		}
		d := today()
		if *date != "" {
			if d, err = parseDate(*date); err != nil {
				return usagef("invalid --date: %v", err)
			}
		}
		settle := d.AddDate(0, 0, *lag)
		o := fit.Options{Settle: settle, MinTTM: *minTTM, Lambda: lam, Huber: *huber, Objective: obj}
		if *minTTM <= 0 || math.IsNaN(*minTTM) {
			o.MinTTM = -1
		}
		if obj == fit.ObjectivePrice {
			o.Weights = fit.WeightInvDuration
		}
		if set["onrate"] {
			if math.IsNaN(*onrate) || math.IsInf(*onrate, 0) {
				return usagef("invalid --onrate")
			}
			on := fit.OvernightToCC(fromPct(*onrate))
			o.OvernightRate = &on
		}
		if *start != "" {
			if o.Start, err = loadStart(e, *start); err != nil {
				return fmt.Errorf("--start: %w", err)
			}
		}

		// open all outputs before the heavy work
		var outs []*output
		cleanup := func(err error) error {
			for _, f := range outs {
				if err != nil {
					f.Discard()
				}
			}
			return err
		}
		files := map[string]*output{}
		if both {
			stem := strings.TrimSuffix(*out, ".json")
			for _, m := range []string{term.ModelNSS, term.ModelSpline} {
				f, err := openOutput(stem+"."+m+".json", e.stdout, *bonds, *start)
				if err != nil {
					return cleanup(err)
				}
				outs = append(outs, f)
				files[m] = f
			}
		} else {
			f, err := openOutput(*out, e.stdout, *bonds, *start)
			if err != nil {
				return err
			}
			outs = append(outs, f)
			files[strings.ToLower(*model)] = f
		}
		var rout *output
		if *resid != "" {
			if rout, err = openOutput(*resid, e.stdout, *bonds, *start); err != nil {
				return cleanup(err)
			}
			outs = append(outs, rout)
		}

		r, closeIn, err := openInput(e, *bonds)
		if err != nil {
			return cleanup(err)
		}
		bf, err := readBondRows(r)
		closeIn()
		if err != nil {
			return cleanup(fmt.Errorf("%s: %w", *bonds, err))
		}
		warn := func(s string) { e.warnf("%s", s) }
		bs := fitBondsFromRows(bf, warn)
		if len(bs) == 0 {
			return cleanup(errors.New("no valid bonds with a price"))
		}
		o.Settle = settle
		_, results, err := runFits(fitRequest{bonds: bs, opts: o, doNSS: doNSS, doSpline: doSpline}, warn)
		if err != nil {
			return cleanup(err)
		}
		source := filepath.Base(*bonds)
		if *bonds == "-" {
			source = "stdin"
		}
		meta := term.Meta{AsOf: settle, Source: "fic curve fit: " + source}
		var errs []string
		for _, res := range results {
			f := files[res.model]
			if res.err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", res.model, res.err))
				f.Discard()
				continue
			}
			data, err := term.Marshal(res.res.Curve, meta)
			if err == nil {
				_, err = f.w.Write(append(data, '\n'))
			}
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", res.model, err))
				f.Discard()
				continue
			}
			if err := f.Close(); err != nil {
				errs = append(errs, err.Error())
			}
			e.infof("%s: %d of %d bonds, RMSE %.3f bp, converged %v, %v", res.model, res.nPrepared, len(bs),
				res.res.RMSEbp, res.res.Converged, res.elapsed.Round(time.Millisecond))
		}
		if rout != nil {
			if err := writeTable(rout.w, residualRows(results), formatCSV); err != nil {
				errs = append(errs, err.Error())
				rout.Discard()
			} else if err := rout.Close(); err != nil {
				errs = append(errs, err.Error())
			}
		}
		if len(errs) > 0 {
			return errors.New(strings.Join(errs, "; "))
		}
		return nil
	}
}

// curveGrid returns the times of --grid or --t (default grid if neither).
func curveGrid(grid, ts, defGrid string) ([]float64, error) {
	if grid != "" && ts != "" {
		return nil, usagef("--grid and --t cannot be combined")
	}
	if ts != "" {
		v, err := parseTimes(ts)
		if err != nil {
			return nil, usagef("invalid --t: %v", err)
		}
		return v, nil
	}
	if grid == "" {
		grid = defGrid
	}
	v, err := parseGrid(grid)
	if err != nil {
		return nil, usagef("%v", err)
	}
	return v, nil
}

func buildCurveShow(e *env) (*flag.FlagSet, func() error) {
	fs := flag.NewFlagSet("curve show", flag.ContinueOnError)
	var cf curveFlags
	var ff formatFlags
	cf.register(fs, false)
	ff.register(fs)
	grid := fs.String("grid", "", "time grid start:end:step (default 1m:10y:1m)")
	ts := fs.String("t", "", "comma-separated maturities instead of a grid, e.g. 1y,2y,5y")
	return fs, func() error {
		times, err := curveGrid(*grid, *ts, "1m:10y:1m")
		if err != nil {
			return err
		}
		fmtOut, err := resolveFormat(ff.format, true, e.stdout, ff.out != "" && ff.out != "-")
		if err != nil {
			return err
		}
		l, err := cf.load(e)
		if err != nil {
			return err
		}
		out, err := openOutput(ff.out, e.stdout, cf.path)
		if err != nil {
			return err
		}
		t := &table{cols: []string{"t_years", "zero_cc_pct", "zero_annual_pct", "discount_factor", "inst_fwd_cc_pct"}}
		for _, x := range times {
			z := l.curve.ZeroRate(x)
			ann, _ := rate.FromContinuous(z, 1)
			t.add(x, toPct(z), toPct(ann), l.curve.Discount(x), toPct(term.InstForward(l.curve, x)))
		}
		return out.finish(writeTable(out.w, t, fmtOut))
	}
}

func buildCurveForward(e *env) (*flag.FlagSet, func() error) {
	fs := flag.NewFlagSet("curve forward", flag.ContinueOnError)
	var cf curveFlags
	var ff formatFlags
	cf.register(fs, false)
	ff.register(fs)
	tn := fs.String("tenor", "1y", "tenor of the forward rate, e.g. 3m, 1y")
	grid := fs.String("grid", "", "start times start:end:step (default 1m:10y:1m)")
	ts := fs.String("t", "", "comma-separated start times instead of a grid")
	return fs, func() error {
		m, err := parseYears(*tn)
		if err != nil || !(m > 0) {
			return usagef("invalid --tenor %q (examples: 3m, 1y)", *tn)
		}
		times, err := curveGrid(*grid, *ts, "1m:10y:1m")
		if err != nil {
			return err
		}
		fmtOut, err := resolveFormat(ff.format, true, e.stdout, ff.out != "" && ff.out != "-")
		if err != nil {
			return err
		}
		l, err := cf.load(e)
		if err != nil {
			return err
		}
		out, err := openOutput(ff.out, e.stdout, cf.path)
		if err != nil {
			return err
		}
		t := &table{cols: []string{"t_years", "tenor_years", "zero_cc_pct", "discount_factor", "fwd_cc_pct", "fwd_annual_pct", "fwd_discount_factor"}}
		for _, x := range times {
			z0, z1 := l.curve.Discount(x), l.curve.Discount(x+m)
			f := z0 / z1
			fcc := math.Log(f) / m
			fann, _ := rate.FromContinuous(fcc, 1)
			t.add(x, m, toPct(l.curve.ZeroRate(x)), z0, toPct(fcc), toPct(fann), z1/z0)
		}
		return out.finish(writeTable(out.w, t, fmtOut))
	}
}

func buildCurveTemplate(e *env) (*flag.FlagSet, func() error) {
	fs := flag.NewFlagSet("curve template", flag.ContinueOnError)
	model := fs.String("model", "nss", "curve model nss|spline|flat")
	outFlag := fs.String("o", "-", "output file (- for stdout)")
	return fs, func() error {
		meta := term.Meta{AsOf: time.Date(2021, 3, 31, 0, 0, 0, 0, time.UTC), Currency: "CHF", Source: "SNB"}
		nss, err := term.NewNSS(-0.00266372, -0.00471343, 0.0568789, -0.0512324, 5.74881, 4.14426)
		if err != nil {
			return err
		}
		var c term.Curve
		switch strings.ToLower(*model) {
		case "nss":
			c = nss
		case "flat":
			c = term.Flat{Rate: 0.01}
			meta = term.Meta{Currency: "CHF"}
		case "spline":
			knots := []float64{0.5, 1, 2, 5, 10, 30}
			dfs := make([]float64, len(knots))
			for i, k := range knots {
				dfs[i] = nss.Discount(k)
			}
			if c, err = term.NewSpline(knots, dfs); err != nil {
				return err
			}
		default:
			return usagef("invalid --model %q (nss, spline, flat)", *model)
		}
		data, err := term.Marshal(c, meta)
		if err != nil {
			return err
		}
		out, err := openOutput(*outFlag, e.stdout)
		if err != nil {
			return err
		}
		_, err = out.w.Write(append(data, '\n'))
		return out.finish(err)
	}
}
