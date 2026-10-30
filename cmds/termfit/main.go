// Command termfit fits a Nelson-Siegel-Svensson and/or a cubic spline term
// structure to the quoted prices of fixed-coupon bonds (see package
// pkg/fit) and writes nss_opt.json, spline_opt.json and result.csv to the
// current directory.
package main

import (
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/konimarti/fixedincome/pkg/fit"
	"github.com/konimarti/fixedincome/pkg/term"
)

const dateFmt = "2006-01-02"

// config are the command line settings.
type config struct {
	file, date, start, objective, model string
	// onRate is the overnight rate in percent (nil: no constraint)
	onRate *float64
	// startExplicit is true if -f was given on the command line
	startExplicit         bool
	minTTM, lambda, huber float64
	settleLag             int
}

func main() {
	var c config
	var onRate float64
	flag.StringVar(&c.file, "file", "bonddata.csv", "CSV file with the bond data: maturity (YYYY-MM-DD or DD.MM.YYYY), coupon (%), clean price (% of face)[, id]; the header row is optional")
	flag.StringVar(&c.date, "date", time.Now().Format(dateFmt), "settlement date of the prices, i.e. the curve date t = 0 (YYYY-MM-DD or DD.MM.YYYY); no settlement lag is applied unless -settle-lag is given")
	flag.IntVar(&c.settleLag, "settle-lag", 0, "calendar days added to -date to obtain the settlement date (e.g. 2 for a trade date)")
	flag.Float64Var(&onRate, "onrate", 0, "overnight rate (e.g. SARON) in %, simple ACT/360; constrains the short end of the curve to 365*ln(1+r/360) (unset: no constraint; 0 imposes 0%)")
	flag.StringVar(&c.start, "f", "term.json", "optional curve file with an additional NSS starting point")
	flag.Float64Var(&c.minTTM, "min-ttm", fit.DefaultMinTTM, "minimum time to maturity in years of the bonds used (<= 0: all)")
	flag.Float64Var(&c.lambda, "lambda", fit.LambdaGCV, "spline smoothing parameter (0: none, < 0: generalized cross-validation)")
	flag.Float64Var(&c.huber, "huber", 0, "Huber threshold in bp for a robust fit (0: least squares)")
	flag.StringVar(&c.objective, "objective", "yield", "fit objective: yield (squared yield errors) or price (squared price errors with inverse-duration weights, for comparison)")
	flag.StringVar(&c.model, "model", "both", "curve model: nss, spline or both")
	flag.Parse()
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "onrate":
			c.onRate = &onRate
		case "f":
			c.startExplicit = true
		}
	})
	if err := run(c, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "termfit:", err)
		os.Exit(1)
	}
}

// run fits the curves and writes the output files to the current
// directory, a summary to out and diagnostics to logw.
func run(c config, out, logw io.Writer) error {
	date, err := fit.ParseDate(c.date)
	if err != nil {
		return fmt.Errorf("invalid -date: %w", err)
	}
	settle := date.AddDate(0, 0, c.settleLag)
	f, err := os.Open(c.file)
	if err != nil {
		return err
	}
	bonds, err := fit.ReadBondsCSV(f)
	f.Close()
	if err != nil {
		return fmt.Errorf("%s: %w", c.file, err)
	}
	o := fit.Options{Settle: settle, MinTTM: c.minTTM, Lambda: fit.Lambda(c.lambda), Huber: c.huber}
	if c.minTTM <= 0 {
		o.MinTTM = -1
	}
	if o.Objective, err = fit.ParseObjective(c.objective); err != nil {
		return err
	}
	if o.Objective == fit.ObjectivePrice {
		o.Weights = fit.WeightInvDuration
	}
	if c.onRate != nil {
		on := fit.OvernightToCC(*c.onRate / 100)
		o.OvernightRate = &on
		fmt.Fprintf(out, "O/N rate %.4f%% ACT/360 (cc ACT/365F %.4f%%) constrains the short end\n", *c.onRate, on*100)
	}
	doNSS, doSpline := c.model == "nss" || c.model == "both", c.model == "spline" || c.model == "both"
	if !doNSS && !doSpline {
		return fmt.Errorf("invalid -model %q (nss, spline, both)", c.model)
	}
	o.Start = readStart(c, out, logw)

	p, err := fit.PrepareBonds(bonds, o)
	if err != nil {
		return err
	}
	for _, w := range p.Warnings() {
		fmt.Fprintln(logw, w)
	}
	fmt.Fprintf(out, "%d of %d bonds, settlement %s\n", p.Len(), len(bonds), settle.Format(dateFmt))
	meta := term.Meta{AsOf: settle}
	var rows []row
	var errs []string
	for _, m := range []struct {
		name, file string
		do         bool
		fit        func(*fit.Prepared, fit.Options) (*fit.Result, error)
	}{{"nss", "nss_opt.json", doNSS, fit.FitNSS}, {"spline", "spline_opt.json", doSpline, fit.FitSpline}} {
		if !m.do {
			if _, err := os.Stat(m.file); err == nil {
				fmt.Fprintf(logw, "%s from an earlier run is not updated (-model %s)\n", m.file, c.model)
			}
			continue
		}
		start := time.Now()
		res, err := m.fit(p, o)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", m.name, err))
			fmt.Fprintf(logw, "%s fit failed: %v\n", m.name, err)
			continue
		}
		elapsed := time.Since(start)
		meta.Source = fmt.Sprintf("termfit %s fit (%s objective) to %s", m.name, c.objective, c.file)
		data, err := term.Marshal(res.Curve, meta)
		if err == nil {
			err = os.WriteFile(m.file, data, 0o644)
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", m.name, err))
			continue
		}
		fmt.Fprintf(out, "\n%s: RMSE %.3fbp, converged %v, %d iterations, %d evaluations, %v\n", m.name, res.RMSEbp, res.Converged, res.Iterations, res.Evaluations, elapsed.Round(time.Millisecond))
		if m.name == "spline" {
			fmt.Fprintf(out, "lambda %g, knots %.4g\n", res.Lambda, res.Knots)
		}
		for _, w := range res.Warnings[len(p.Warnings()):] {
			fmt.Fprintln(out, "warning:", w)
		}
		fmt.Fprintf(out, "%s\n", data)
		for _, r := range res.Residuals {
			rows = append(rows, row{model: m.name, Residual: r, curve: res.Curve})
		}
	}
	if len(rows) > 0 {
		fout, err := os.Create("result.csv")
		if err != nil {
			return err
		}
		if err := writeResults(fout, rows); err != nil {
			fout.Close()
			return err
		}
		if err := fout.Close(); err != nil {
			return err
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// readStart returns the NSS starting point of the curve file c.start (nil
// if there is none). Problems with the file are logged to logw if -f was
// given explicitly or the file exists but is unusable.
func readStart(c config, out, logw io.Writer) *term.NSS {
	data, err := os.ReadFile(c.start)
	if err != nil {
		if c.startExplicit || !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(logw, "-f %s: %v; no starting curve used\n", c.start, err)
		}
		return nil
	}
	curve, _, err := term.Parse(data)
	if err != nil {
		fmt.Fprintf(logw, "-f %s: %v; no starting curve used\n", c.start, err)
		return nil
	}
	base, spread := term.Spread(curve)
	n, ok := base.(term.NSS)
	if !ok {
		fmt.Fprintf(logw, "-f %s: not an NSS curve (%T); no starting curve used\n", c.start, base)
		return nil
	}
	n.B0 += spread // a z-spread shifts all NSS zero rates
	fmt.Fprintf(out, "additional NSS starting point from %s\n", c.start)
	return &n
}

// row is a line of result.csv.
type row struct {
	model string
	fit.Residual
	curve term.Curve
}

// resultHeader are the columns of result.csv (rates in percent, prices in
// percent of face).
var resultHeader = []string{"model", "id", "maturity", "ttm_years", "price_clean_market", "price_clean_model",
	"yield_cc_market_pct", "yield_cc_model_pct", "yield_annual_market_pct", "yield_annual_model_pct", "error_bp",
	"zero_cc_model_pct", "discount_factor_model"}

// writeResults writes the residuals as CSV with a header, sorted by time
// to maturity (then model).
func writeResults(w io.Writer, rows []row) error {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].TTM != rows[j].TTM {
			return rows[i].TTM < rows[j].TTM
		}
		return rows[i].model < rows[j].model
	})
	cw := csv.NewWriter(w)
	if err := cw.Write(resultHeader); err != nil {
		return err
	}
	g := func(v float64) string { return strconv.FormatFloat(v, 'g', 10, 64) }
	for _, r := range rows {
		rec := []string{r.model, r.ID, r.Maturity.Format(dateFmt), g(r.TTM), g(r.CleanMarket), g(r.CleanModel),
			g(r.YieldMarket * 100), g(r.YieldModel * 100), g(r.YieldMarketAnnual * 100), g(r.YieldModelAnnual * 100), g(r.ErrBp),
			g(r.curve.ZeroRate(r.TTM) * 100), g(r.curve.Discount(r.TTM))}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
