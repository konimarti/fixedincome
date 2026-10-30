package cli

// Shims of the legacy command line tools (bonds-cli, swaprate-cli,
// option-cli, termfit, fwdcrv, holee). They keep their old flags, units
// (including holee's decimal -s and option-cli's decimal -vola) and, for
// termfit, fwdcrv and holee, their old output files and formats. The
// calculation is done by the implementation of the corresponding fic
// subcommand. Every shim prints a deprecation note to stderr.

import (
	"bufio"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/konimarti/fixedincome/pkg/fit"
	"github.com/konimarti/fixedincome/pkg/instrument/option"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/term"
)

// legacyFlags creates a flag set that reports errors to stderr.
func legacyFlags(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

func deprecated(stderr io.Writer, use string) {
	fmt.Fprintf(stderr, "deprecated: use \"fic %s\"\n", use)
}

// ftoa formats a number for a flag value (12 significant digits, so that
// 0.07*100 is "7").
func ftoa(v float64) string { return strconv.FormatFloat(v, 'g', 12, 64) }

// parseLegacy parses the flags; it returns the exit code and false on error.
func parseLegacy(fs *flag.FlagSet, args []string) (int, bool) {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ExitOK, false
		}
		return ExitUsage, false
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(fs.Output(), "%s: unexpected arguments: %v\n", fs.Name(), fs.Args())
		return ExitUsage, false
	}
	return 0, true
}

// LegacyBonds is the bonds-cli shim: "fic bond price --format text". Differences
// to the old tool: the output has the format of fic (spread_total_bp is the
// old "Implied spread"; duration and convexity are evaluated at the market
// price), and -spread is ADDED to the spread_bp of the curve file (it
// replaced it before).
func LegacyBonds(args []string, stdout, stderr io.Writer) int {
	fs := legacyFlags("bonds-cli", stderr)
	settlement := fs.String("settlement", today().Format(dateLayout), "valuation date / settlement date")
	mat := fs.String("maturity", today().AddDate(1, 0, 0).Format(dateLayout), "maturity date of bond")
	coupon := fs.Float64("coupon", 0, "coupon in percent of par value")
	n := fs.Int("n", 1, "compounding frequency per year")
	quote := fs.Float64("quote", 0, "quoted bond price at settlement date")
	redemption := fs.Float64("redemption", 100, "redemption value of bond at maturity")
	spread := fs.Float64("spread", 0, "static spread in basis points")
	file := fs.String("f", "term.json", "json file containing the term structure")
	daycount := fs.String("daycount", "30E360", "day count convention for accrued interest: "+strings.Join(maturity.DayCountNames(), ", "))
	if code, ok := parseLegacy(fs, args); !ok {
		return code
	}
	deprecated(stderr, "bond price")
	if *n <= 0 {
		*n = 1 // legacy: frequency <= 0 means annual
	}
	a := []string{"bond", "price", "--curve", *file, "--date", *settlement, "--maturity", *mat, "--coupon", ftoa(*coupon),
		"--freq", strconv.Itoa(*n), "--daycount", *daycount, "--redemption", ftoa(*redemption), "--spread", ftoa(*spread), "--format", "text"}
	if *quote > 0 {
		a = append(a, "--price", ftoa(*quote))
	}
	return Run(a, stdout, stderr)
}

// LegacySwapRate is the swaprate-cli shim: "fic swap rate --format text".
func LegacySwapRate(args []string, stdout, stderr io.Writer) int {
	fs := legacyFlags("swaprate-cli", stderr)
	spread := fs.Float64("s", 0, "spread in bps for yield curve")
	file := fs.String("f", "term.json", "json file containing the term structure")
	if code, ok := parseLegacy(fs, args); !ok {
		return code
	}
	deprecated(stderr, "swap rate")
	return Run([]string{"swap", "rate", "--curve", *file, "--spread", ftoa(*spread), "--format", "text"}, stdout, stderr)
}

// LegacyOption is the option-cli shim. It keeps the old text output and the
// raw units of the Greeks (per 1.00 change of the volatility, the rate and
// the dividend yield, per year for theta, which is not printed) and the old
// decimal -vola (fic option price takes percent and scales the Greeks).
func LegacyOption(args []string, stdout, stderr io.Writer) int {
	fs := legacyFlags("option-cli", stderr)
	typ := fs.String("type", "call", "type is either 'call' or 'put'")
	spot := fs.Float64("s", 110, "current stock price")
	strike := fs.Float64("k", 100, "strike price")
	t := fs.Float64("t", 2, "time to expiration date in years")
	q := fs.Float64("q", 0, "dividend yield in percent")
	vola := fs.Float64("vola", 0.3, "volatility as a decimal (0.3 = 30%)")
	r := fs.Float64("r", 2, "continuously compounded discount rate in percent")
	if code, ok := parseLegacy(fs, args); !ok {
		return code
	}
	deprecated(stderr, "option price")
	if *vola > 5 {
		fmt.Fprintf(stderr, "warning: -vola %g looks like percent; -vola is a decimal here (0.3 = 30%%)\n", *vola)
	}
	ot, err := option.ParseType(*typ)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return ExitUsage
	}
	o := option.European{Type: ot, S: *spot, K: *strike, T: *t, Q: *q / 100.0, Vol: *vola}
	c := term.Flat{Rate: *r / 100.0}
	price, err := o.Price(c)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return ExitUsage
	}
	g, err := o.Greeks(c)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return ExitUsage
	}
	if ot == option.Call {
		fmt.Fprintln(stdout, "Call Option")
	} else {
		fmt.Fprintln(stdout, "Put Option")
	}
	fmt.Fprintf(stdout, "Price: %9.4f\n", price)
	fmt.Fprintln(stdout, "\nThe 'Greeks'")
	fmt.Fprintf(stdout, "Delta: %9.4f\n", g.Delta)
	fmt.Fprintf(stdout, "Gamma: %9.4f\n", g.Gamma)
	fmt.Fprintf(stdout, "Rho  : %9.4f\n", g.Rho)
	fmt.Fprintf(stdout, "Vega : %9.4f\n", g.Vega)
	return ExitOK
}

// LegacyFwdcrv is the fwdcrv shim. It keeps the old output: a block with the
// discount factors and forward rate for -t to stdout and the file
// forward.csv (columns x,SpotRate,FwdRate,Z,F; rates in percent, monthly
// for 10 years) in the current directory.
func LegacyFwdcrv(args []string, stdout, stderr io.Writer) int {
	fs := legacyFlags("fwdcrv", stderr)
	file := fs.String("f", "term.json", "json file containing the term structure")
	m := fs.Float64("m", 1, "term maturity of forward rate in decimal years")
	t := fs.Float64("t", 0, "start time for forward rate in decimal years")
	if code, ok := parseLegacy(fs, args); !ok {
		return code
	}
	deprecated(stderr, "curve forward")
	if err := fwdcrv(*file, *m, *t, stdout, stderr); err != nil {
		fmt.Fprintln(stderr, "fwdcrv:", err)
		return ExitError
	}
	return ExitOK
}

func fwdcrv(file string, m, t1 float64, stdout, stderr io.Writer) (err error) {
	e := &env{stdin: os.Stdin, stdout: stdout, stderr: stderr}
	ts, _, err := readCurveFile(e, file)
	if err != nil {
		return err
	}
	fmt.Fprintln(stderr, "Term model read from", file)
	if math.Abs(m) < 1e-16 {
		return errors.New("term maturity too small")
	}
	if math.Abs(t1) > 1e-16 {
		fmt.Fprintln(stdout, "Discount Factors and Forward Rate")
		fmt.Fprintln(stdout, "")
		fmt.Fprintf(stdout, "Z(0,%4.2f)\t\t%3.4f\n", t1, ts.Discount(t1))
		ffac := ts.Discount(t1+m) / ts.Discount(t1)
		fmt.Fprintf(stdout, "F(0,%4.2f,%4.2f)\t\t%3.4f\n", t1, t1+m, ffac)
		fmt.Fprintf(stdout, "Z(0,%4.2f)\t\t%3.4f\n", t1+m, ts.Discount(t1+m))
		fmt.Fprintln(stdout, "------------------------------")
		fmt.Fprintf(stdout, "r(0,%4.2f)\t\t%3.4f%%\n", t1, ts.ZeroRate(t1)*100.0)
		fmt.Fprintf(stdout, "f(0,%4.2f,%4.2f)\t\t%3.4f%%\n", t1, t1+m, -math.Log(ffac)/m*100.0)
		fmt.Fprintf(stdout, "r(0,%4.2f)\t\t%3.4f%%\n", t1+m, ts.ZeroRate(t1+m)*100.0)
		fmt.Fprintln(stdout, "------------------------------")
	}
	output := [][]string{{"x", "SpotRate", "FwdRate", "Z", "F"}}
	for i := 1; i <= 10*12; i++ {
		t := float64(i) / 12.0
		z := ts.Discount(t)
		f := ts.Discount(t+m) / z
		spot := -math.Log(z) / t * 100.0
		fwd := -math.Log(f) / m * 100.0
		output = append(output, []string{fmt.Sprintf("%v", t), fmt.Sprintf("%v", spot), fmt.Sprintf("%v", fwd),
			fmt.Sprintf("%v", z), fmt.Sprintf("%v", f)})
	}
	out, err := openOutput("forward.csv", stdout)
	if err != nil {
		return err
	}
	w := csv.NewWriter(out.w)
	err = w.WriteAll(output)
	return out.finish(err)
}

// termfitConfig are the settings of the termfit shim.
type termfitConfig struct {
	file, date, start, objective, model string
	// onRate is the overnight rate in percent (nil: no constraint)
	onRate *float64
	// startExplicit is true if -f was given on the command line
	startExplicit         bool
	minTTM, lambda, huber float64
	settleLag             int
}

// LegacyTermfit is the termfit shim. It keeps the old behaviour: reads
// bonddata.csv (any bond CSV, see fic curve fit) and writes nss_opt.json,
// spline_opt.json and result.csv to the current directory and a summary to
// stdout; -f term.json is used as an NSS starting point if it exists.
func LegacyTermfit(args []string, stdout, stderr io.Writer) int {
	var c termfitConfig
	var onRate float64
	fs := legacyFlags("termfit", stderr)
	fs.StringVar(&c.file, "file", "bonddata.csv", "CSV file with the bond data")
	fs.StringVar(&c.date, "date", today().Format(dateLayout), "settlement date of the prices, i.e. the curve date")
	fs.IntVar(&c.settleLag, "settle-lag", 0, "calendar days added to -date to obtain the settlement date")
	fs.Float64Var(&onRate, "onrate", 0, "overnight rate in %, simple ACT/360 (unset: no constraint)")
	fs.StringVar(&c.start, "f", "term.json", "optional curve file with an additional NSS starting point")
	fs.Float64Var(&c.minTTM, "min-ttm", fit.DefaultMinTTM, "minimum time to maturity in years of the bonds used (<= 0: all)")
	fs.Float64Var(&c.lambda, "lambda", fit.LambdaGCV, "spline smoothing parameter (0: none, < 0: generalized cross-validation)")
	fs.Float64Var(&c.huber, "huber", 0, "Huber threshold in bp for a robust fit (0: least squares)")
	fs.StringVar(&c.objective, "objective", "yield", "fit objective: yield or price")
	fs.StringVar(&c.model, "model", "both", "curve model: nss, spline or both")
	if code, ok := parseLegacy(fs, args); !ok {
		return code
	}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "onrate":
			c.onRate = &onRate
		case "f":
			c.startExplicit = true
		}
	})
	deprecated(stderr, "curve fit")
	if err := termfitRun(c, stdout, stderr); err != nil {
		fmt.Fprintln(stderr, "termfit:", err)
		return ExitError
	}
	return ExitOK
}

// termfitRun fits the curves and writes the output files to the current
// directory, a summary to out and diagnostics to logw.
func termfitRun(c termfitConfig, out, logw io.Writer) error {
	e := &env{stdin: os.Stdin, stdout: out, stderr: logw, name: "termfit"}
	date, err := fit.ParseDate(c.date)
	if err != nil {
		return fmt.Errorf("invalid -date: %w", err)
	}
	settle := date.AddDate(0, 0, c.settleLag)
	f, err := os.Open(c.file)
	if err != nil {
		return err
	}
	bf, err := readBondRows(f)
	f.Close()
	if err != nil {
		return fmt.Errorf("%s: %w", c.file, err)
	}
	bonds := fitBondsFromRows(bf, func(s string) { fmt.Fprintln(logw, s) })
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
		on := fit.OvernightToCC(fromPct(*c.onRate))
		o.OvernightRate = &on
		fmt.Fprintf(out, "O/N rate %.4f%% ACT/360 (cc ACT/365F %.4f%%) constrains the short end\n", *c.onRate, on*100)
	}
	doNSS, doSpline := c.model == "nss" || c.model == "both", c.model == "spline" || c.model == "both"
	if !doNSS && !doSpline {
		return fmt.Errorf("invalid -model %q (nss, spline, both)", c.model)
	}
	o.Start = termfitStart(e, c, out, logw)

	p, results, err := runFits(fitRequest{bonds: bonds, opts: o, doNSS: doNSS, doSpline: doSpline},
		func(s string) { fmt.Fprintln(logw, s) })
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%d of %d bonds, settlement %s\n", p.Len(), len(bf.rows), formatDate(settle))
	meta := term.Meta{AsOf: settle}
	var errs []string
	files := map[string]string{term.ModelNSS: "nss_opt.json", term.ModelSpline: "spline_opt.json"}
	for model, name := range files {
		if (model == term.ModelNSS && !doNSS) || (model == term.ModelSpline && !doSpline) {
			if _, err := os.Stat(name); err == nil {
				fmt.Fprintf(logw, "%s from an earlier run is not updated (-model %s)\n", name, c.model)
			}
		}
	}
	for _, r := range results {
		if r.err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", r.model, r.err))
			fmt.Fprintf(logw, "%s fit failed: %v\n", r.model, r.err)
			continue
		}
		meta.Source = fmt.Sprintf("termfit %s fit (%s objective) to %s", r.model, c.objective, c.file)
		data, err := term.Marshal(r.res.Curve, meta)
		if err == nil {
			err = os.WriteFile(files[r.model], data, 0o644)
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", r.model, err))
			continue
		}
		fmt.Fprintf(out, "\n%s: RMSE %.3fbp, converged %v, %d iterations, %d evaluations, %v\n", r.model, r.res.RMSEbp, r.res.Converged,
			r.res.Iterations, r.res.Evaluations, r.elapsed.Round(time.Millisecond))
		if r.model == term.ModelSpline {
			fmt.Fprintf(out, "lambda %g, knots %.4g\n", r.res.Lambda, r.res.Knots)
		}
		for _, w := range r.res.Warnings[len(p.Warnings()):] {
			fmt.Fprintln(out, "warning:", w)
		}
		fmt.Fprintf(out, "%s\n", data)
	}
	if tbl := residualRows(results); len(tbl.rows) > 0 {
		fout, err := os.Create("result.csv")
		if err != nil {
			return err
		}
		if err := writeTable(fout, tbl, formatCSV); err != nil {
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

// termfitStart returns the NSS starting point of the curve file c.start (nil
// if there is none). Problems are logged to logw if -f was given explicitly
// or the file exists but is unusable.
func termfitStart(e *env, c termfitConfig, out, logw io.Writer) *term.NSS {
	if _, err := os.Stat(c.start); err != nil && !c.startExplicit && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	n, err := loadStart(e, c.start)
	if err != nil {
		fmt.Fprintf(logw, "-f %s: %v; no starting curve used\n", c.start, err)
		return nil
	}
	fmt.Fprintf(out, "additional NSS starting point from %s\n", c.start)
	return n
}

// LegacyHolee is the holee shim. It keeps the old flags (-s is a DECIMAL
// volatility here, --sigma of fic is in percent) and the old CSV format
// (header t,zero_rate,fwd_rate,theta,exp_short_rate,discount,mc_discount,
// mc_stderr,mc_zero_rate; rates and discount factors as decimals) and
// summary on stderr.
func LegacyHolee(args []string, stdout, stderr io.Writer) int {
	deprecated(stderr, "sim zcb --model holee")
	if err := legacyHolee(args, stdout, stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ExitOK
		}
		fmt.Fprintln(stderr, "holee:", err)
		var ue *usageError
		if errors.As(err, &ue) {
			return ExitUsage
		}
		return ExitError
	}
	return ExitOK
}

func legacyHolee(args []string, stdout, stderr io.Writer) (err error) {
	fs := legacyFlags("holee", stderr)
	file := fs.String("f", "term.json", "JSON file with the term structure")
	n := fs.Int("n", 1000, "number of time steps (>= 1)")
	horizon := fs.Float64("m", 10.0, "horizon (maturity) of the simulation in years")
	sigma := fs.Float64("s", 0.02, "volatility of the short rate as a decimal per square root of a year (0.01 = 100bp)")
	paths := fs.Int("paths", 10000, "number of Monte Carlo paths")
	seed := fs.Uint64("seed", 0, "seed of the random number generator (0: time-based seed, printed)")
	outFlag := fs.String("o", "-", "output CSV file ('-' for stdout)")
	if err := fs.Parse(args); err != nil {
		return &usageError{msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return usagef("unexpected arguments: %v", fs.Args())
	}
	e := &env{stdin: os.Stdin, stdout: stdout, stderr: stderr}
	curve, _, err := readCurveFile(e, *file)
	if err != nil {
		return err
	}
	s := *seed
	if s == 0 {
		s = uint64(time.Now().UnixNano())
	}
	// open the output before the (possibly long) simulation
	out, err := openOutput(*outFlag, stdout)
	if err != nil {
		return err
	}
	p := simParams{model: "holee", curve: curve, sigma: *sigma, horizon: *horizon, steps: *n, paths: *paths, seed: s}
	r, err := simulate(p)
	if err != nil {
		return out.finish(err)
	}
	bw := bufio.NewWriter(out.w)
	cw := csv.NewWriter(bw)
	if err := cw.Write([]string{"t", "zero_rate", "fwd_rate", "theta", "exp_short_rate", "discount", "mc_discount", "mc_stderr", "mc_zero_rate"}); err != nil {
		return out.finish(err)
	}
	ff := func(x float64) string { return strconv.FormatFloat(x, 'g', 12, 64) }
	for _, nd := range r.nodes {
		if err := cw.Write([]string{ff(nd.t), ff(nd.curveZero), ff(nd.curveFwd), ff(nd.theta), ff(nd.phi),
			ff(nd.curveDF), ff(nd.mcDF), ff(nd.mcSE), ff(nd.mcZero)}); err != nil {
			return out.finish(err)
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return out.finish(err)
	}
	if err := bw.Flush(); err != nil {
		return out.finish(err)
	}
	if err := out.finish(nil); err != nil {
		return err
	}
	writeSimSummary(stderr, p, r)
	return nil
}
