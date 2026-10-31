package cli

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/konimarti/fixedincome/pkg/lattice"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/portfolio"
)

// defaultKRDTenors are the default --krd-tenors.
const defaultKRDTenors = "3m,6m,1y,2y,3y,5y,7y,10y,15y,20y,30y"

// parseKRDTenors parses --krd-tenors into labels and years.
func parseKRDTenors(s string) ([]string, []float64, error) {
	var labels []string
	var keys []float64
	for _, p := range strings.Split(s, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		y, err := parseYears(p)
		if err != nil {
			return nil, nil, usagef("--krd-tenors: %v", err)
		}
		if !(y > 0) || (len(keys) > 0 && !(y > keys[len(keys)-1])) {
			return nil, nil, usagef("invalid --krd-tenors %q: tenors must be positive and strictly increasing", s)
		}
		labels, keys = append(labels, p), append(keys, y)
	}
	return labels, keys, nil
}

// bpLabel formats a shift in bp for a scenario name.
func bpLabel(bp float64) string { return strconv.FormatFloat(bp, 'g', 10, 64) + "bp" }

// buildScenarios builds the scenarios of --scenarios (a comma-separated
// list of parallel, irrbb, none).
func buildScenarios(kinds string, shocks []float64, irrbb portfolio.IRRBBParams) ([]portfolio.Scenario, error) {
	var out []portfolio.Scenario
	for _, k := range strings.Split(kinds, ",") {
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "none":
		case "parallel":
			for _, s := range shocks {
				out = append(out,
					portfolio.ParallelScenario("parallel +"+bpLabel(s), fromBP(s)),
					portfolio.ParallelScenario("parallel -"+bpLabel(s), -fromBP(s)))
			}
		case "irrbb":
			out = append(out, portfolio.IRRBB(irrbb)...)
		default:
			return nil, usagef("invalid --scenarios %q (parallel, irrbb, none; a comma-separated list is allowed)", kinds)
		}
	}
	return out, nil
}

// posRow is a row of the position file with its result.
type posRow struct {
	id, issuer, rating string
	nominal            any
	source             string
	spreadBP           float64
	res                portfolio.Result
	status             string
	ok                 bool
}

func buildPortfolioRisk(e *env) (*flag.FlagSet, func() error) {
	fs := flag.NewFlagSet("portfolio risk", flag.ContinueOnError)
	var cf curveFlags
	var ff formatFlags
	cf.register(fs, true)
	ff.register(fs)
	in := fs.String("in", "", "CSV file with the positions (- for stdin): the bond columns maturity,coupon_pct,price_clean[,id,freq,daycount,face,redemption,issuer,rating,call,call_from,put] and the REQUIRED column nominal (currency units of face; negative = short or liability)")
	freq := fs.String("freq", "1", "default coupons per year (1, 2, 3, 4, 6, 12)")
	dayCount := fs.String("daycount", "30E360", "default accrual day count: "+strings.Join(maturity.DayCountNames(), ", "))
	face := fs.Float64("face", 100, "default face value")
	vol := fs.Float64("vol", math.NaN(), "Hull-White volatility of the short rate in percent per year (absolute, 1.0 = 100bp); required with call or put terms")
	meanRev := fs.Float64("mean-reversion", 0.03, "Hull-White mean reversion per year (decimal, not percent) for call or put terms")
	steps := fs.Int("steps", lattice.DefaultStepsPerYear, "lattice steps per year for call or put terms")
	krd := fs.String("krd-tenors", defaultKRDTenors, "key rate tenors (triangular bumps of the zero rate, flat beyond the first and last)")
	shocks := fs.String("shocks", "50,100,200", "sizes in bp of the parallel scenarios (each as + and -)")
	scen := fs.String("scenarios", "parallel", "scenarios, full revaluation with the spreads held: parallel (--shocks), irrbb (the six standardised IRRBB scenarios; no floor on the shocked rates is applied; the irrbb sizes are illustrative, check BCBS d368 / FINMA), none; comma-separated list allowed")
	irrP := fs.Float64("irrbb-parallel", 100, "irrbb: parallel shock in bp; defaults are illustrative, check the current BCBS d368 / FINMA values; no floor on shocked rates is applied")
	irrS := fs.Float64("irrbb-short", 150, "irrbb: short rate shock in bp (illustrative default, check BCBS d368 / FINMA; no floor on shocked rates)")
	irrL := fs.Float64("irrbb-long", 100, "irrbb: long rate shock in bp (illustrative default, check BCBS d368 / FINMA; no floor on shocked rates)")
	groupBy := fs.String("group-by", "", "group the summary by the column issuer|rating|freq|daycount (market value, weight, duration, DV01 and KRD per group)")
	section := fs.String("section", "", "table to print: summary, krd, scenarios or positions (default: all in text and json, positions in csv); spread_bp of the positions is the z-spread of a straight bond and the OAS of a callable one")
	strict := fs.Bool("strict", false, "exit with code 1 and print no output if a position is excluded or valued on the curve without a market price (default: warn and continue)")
	return fs, func() error {
		set := setFlags(fs)
		if *in == "" {
			return usagef("--in FILE is required (CSV with the bond columns and nominal)")
		}
		hw, err := parseHW(set, *vol, *meanRev, *steps)
		if err != nil {
			return err
		}
		fr, err := parseFreq(*freq)
		if err != nil {
			return err
		}
		dc, err := parseDayCountFlag(*dayCount)
		if err != nil {
			return err
		}
		if !(*face > 0) || math.IsInf(*face, 0) {
			return usagef("invalid --face %v: must be positive", *face)
		}
		labels, keys, err := parseKRDTenors(*krd)
		if err != nil {
			return err
		}
		var shockList []float64
		for _, p := range strings.Split(*shocks, ",") {
			x, err := parseFinite(p)
			if err != nil || !(x > 0) {
				return usagef("invalid --shocks %q: positive sizes in bp, comma-separated", *shocks)
			}
			shockList = append(shockList, x)
		}
		for _, c := range []struct {
			n string
			x float64
		}{{"irrbb-parallel", *irrP}, {"irrbb-short", *irrS}, {"irrbb-long", *irrL}} {
			if !(c.x >= 0) || math.IsInf(c.x, 0) {
				return usagef("invalid --%s %v: must be non-negative (bp)", c.n, c.x)
			}
		}
		scenarios, err := buildScenarios(*scen, shockList, portfolio.IRRBBParams{Parallel: fromBP(*irrP), Short: fromBP(*irrS), Long: fromBP(*irrL)})
		if err != nil {
			return err
		}
		*groupBy = strings.ToLower(strings.TrimSpace(*groupBy))
		if set["group-by"] && !groupColumns[*groupBy] {
			return usagef("invalid --group-by %q (issuer, rating, freq, daycount)", *groupBy)
		}
		*section = strings.ToLower(strings.TrimSpace(*section))
		switch *section {
		case "", "summary", "krd", "scenarios", "positions":
		default:
			return usagef("invalid --section %q (summary, krd, scenarios, positions)", *section)
		}
		fmtOut, err := resolveFormat(ff.format, true, e.stdout, ff.out != "" && ff.out != "-")
		if err != nil {
			return err
		}
		if *section == "" && fmtOut == formatCSV {
			*section = "positions"
		}

		if *in == "-" && cf.path == "-" {
			return usagef("stdin can be used only once (--curve - and --in -)")
		}
		r, closeIn, err := openInput(e, *in)
		if err != nil {
			return err
		}
		bf, err := readBondRows(r)
		closeIn()
		if err != nil {
			return fmt.Errorf("%s: %w", *in, err)
		}
		if !bf.hasNominal {
			return usagef("%s: the column nominal is required (currency units of face; negative for a short position or liability)", *in)
		}
		if *groupBy == "issuer" && !bf.hasIssuer {
			return fmt.Errorf("%s: --group-by issuer needs an issuer column in the input", *in)
		}
		if *groupBy == "rating" && !bf.hasRating {
			return fmt.Errorf("%s: --group-by rating needs a rating column in the input", *in)
		}
		if bf.callable && !(hw.sigma > 0) {
			return usagef("--vol is required: %s has rows with call or put terms and the volatility cannot be observed from the bonds (percent per year, e.g. --vol 1.0)", *in)
		}
		l, err := cf.load(e)
		if err != nil {
			return err
		}
		out, err := openOutput(ff.out, e.stdout, cf.path, *in)
		if err != nil {
			return err
		}
		cfg := portfolio.Config{Keys: keys, Scenarios: scenarios}
		d := bondDefaults{freq: fr, dc: dc, face: *face}
		var rows []*posRow
		var results []portfolio.Result
		var failedIDs, modelIDs, dupIDs []string
		seen := map[string]bool{}
		for _, b := range bf.rows {
			pr := &posRow{id: b.id, issuer: b.issuer, rating: b.rating}
			rows = append(rows, pr)
			if seen[b.id] {
				dupIDs = append(dupIDs, b.id)
			}
			seen[b.id] = true
			if b.nominalErr == nil && b.err == nil {
				pr.nominal = b.nominal
			}
			res, err := evalRow(b, d, hw, l, cfg, pr)
			if err != nil {
				pr.status = err.Error()
				failedIDs = append(failedIDs, b.id+": "+pr.status)
				continue
			}
			pr.ok, pr.status, pr.res = true, "ok", res
			if pr.source == "model" {
				modelIDs = append(modelIDs, b.id)
			}
			results = append(results, res)
		}
		failed := len(rows) - len(results)
		if len(results) == 0 {
			return out.finish(fmt.Errorf("all rows failed (%s)", firstN(failedIDs, 3)))
		}
		if len(dupIDs) > 0 {
			e.warnf("duplicate ids: %s", firstN(dupIDs, 5))
		}
		// with --strict nothing is written if the run will fail
		var problems []string
		if failed > 0 {
			problems = append(problems, fmt.Sprintf("%d of %d positions excluded (%s; see the status column of --section positions)", failed, len(rows), firstN(failedIDs, 3)))
		}
		if len(modelIDs) > 0 {
			problems = append(problems, fmt.Sprintf("%d positions valued on the curve, not from a market price (%s)", len(modelIDs), firstN(modelIDs, 5)))
		}
		if *strict && len(problems) > 0 {
			return out.finish(errors.New(strings.Join(problems, "; ")))
		}
		rep, err := newRiskReport(cfg, labels, rows, results, *groupBy, bf, l.settle)
		if err != nil {
			return out.finish(err)
		}
		rep.failed = failed
		if err := out.finish(rep.write(out.w, fmtOut, *section)); err != nil {
			return err
		}
		for _, p := range problems {
			e.warnf("%s", p)
		}
		if rep.total.Degenerate {
			e.warnf("net value ~0; use dv01 / key_dv01 (duration, convexity, KRD and weights are not shown)")
		}
		return nil
	}
}

// firstN joins the first n items, with "..." if there are more.
func firstN(x []string, n int) string {
	if len(x) <= n {
		return strings.Join(x, ", ")
	}
	return strings.Join(x[:n], ", ") + ", ..."
}

// evalRow values one position row (the spread is solved from the market
// price as in "bond price") and computes its risk.
func evalRow(b bondRow, d bondDefaults, hw hwParams, l *loaded, cfg portfolio.Config, pr *posRow) (portfolio.Result, error) {
	if b.err != nil {
		return portfolio.Result{}, b.err
	}
	if b.nominalErr != nil {
		return portfolio.Result{}, b.nominalErr
	}
	bi := b.input(d, hw)
	v, err := valueBond(bi, l.settle, l.curve)
	if err != nil {
		return portfolio.Result{}, err
	}
	bnd, err := newStraight(bi)
	if err != nil {
		return portfolio.Result{}, err
	}
	cfs, err := bnd.CashFlows(l.settle)
	if err != nil {
		return portfolio.Result{}, err
	}
	accrued, err := bnd.Accrued(l.settle)
	if err != nil {
		return portfolio.Result{}, err
	}
	pos := portfolio.Position{ID: b.id, CashFlows: cfs, Accrued: accrued, Factor: b.nominal / bi.face, Spread: v.zspread,
		Groups: map[string]string{"issuer": b.issuer, "rating": b.rating, "freq": strconv.Itoa(int(bi.freq)), "daycount": bi.dc.String()}}
	if v.call != nil {
		c, err := makeCallable(bnd, bi.opt)
		if err != nil {
			return portfolio.Result{}, err
		}
		pos.Spread = v.call.oas
		pos.Callable = &portfolio.Callable{Bond: c, Settle: l.settle, MeanReversion: hw.a, Vol: hw.sigma,
			Options: []lattice.Option{lattice.WithStepsPerYear(hw.steps)}}
	}
	pr.source = "model"
	if v.market {
		pr.source = "market"
	}
	pr.spreadBP = toBP(pos.Spread)
	return portfolio.Evaluate(l.curve, pos, cfg)
}

// riskReport is the result of "portfolio risk" with its tables.
type riskReport struct {
	settle    string
	cfg       portfolio.Config
	labels    []string
	rows      []*posRow
	total     portfolio.Total
	groupBy   string
	groups    []portfolio.Group
	failed    int
	hasIssuer bool
	hasRating bool
}

func newRiskReport(cfg portfolio.Config, labels []string, rows []*posRow, results []portfolio.Result, groupBy string, bf *bondFile, settle time.Time) (*riskReport, error) {
	r := &riskReport{settle: formatDate(settle), cfg: cfg, labels: labels, rows: rows, groupBy: groupBy, hasIssuer: bf.hasIssuer, hasRating: bf.hasRating}
	var err error
	if r.total, err = portfolio.Aggregate(cfg, results); err != nil {
		return nil, err
	}
	if groupBy != "" {
		if r.groups, err = portfolio.GroupBy(cfg, results, groupBy); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// snap sets numerical noise (|x| < 1e-10, e.g. of the lattice differences
// of a key rate without any exposure) to 0.
func snap(x float64) float64 {
	if math.Abs(x) < 1e-10 {
		return 0
	}
	return x
}

// ratio returns x, or NaN (empty, null, n/a) if the value is degenerate.
func ratio(degenerate bool, x float64) float64 {
	if degenerate {
		return math.NaN()
	}
	return x
}

func (r *riskReport) krdCols() []string {
	var c []string
	for _, l := range r.labels {
		c = append(c, "krd_"+l)
	}
	return c
}

// summary is the one-row table of the portfolio.
func (r *riskReport) summary() *table {
	t := &table{cols: []string{"n_positions", "n_excluded", "mv_dirty", "mv_clean", "accrued", "gross_mv", "mod_duration", "convexity", "dv01"}}
	x := r.total
	t.add(x.N, r.failed, x.Dirty, x.Clean, x.Accrued, x.Gross, ratio(x.Degenerate, x.Duration), ratio(x.Degenerate, x.Convexity), x.DV01)
	return t
}

// groupTable has one row per group and a final TOTAL row.
func (r *riskReport) groupTable() *table {
	cols := append([]string{r.groupBy, "n", "mv_dirty", "mv_clean", "accrued", "weight_pct", "weight_gross_pct", "mod_duration", "convexity", "dv01"}, r.krdCols()...)
	t := &table{cols: cols}
	add := func(name string, x portfolio.Total) {
		w, wg := math.NaN(), 0.0
		if !r.total.Degenerate {
			w = 100 * x.Dirty / r.total.Dirty
		}
		if r.total.Gross != 0 {
			wg = 100 * x.Dirty / r.total.Gross
		}
		row := []any{name, x.N, x.Dirty, x.Clean, x.Accrued, w, wg, ratio(x.Degenerate, x.Duration), ratio(x.Degenerate, x.Convexity), x.DV01}
		for _, k := range x.KRD {
			row = append(row, ratio(x.Degenerate, snap(k)))
		}
		t.add(row...)
	}
	for _, g := range r.groups {
		add(g.Key, g.Total)
	}
	add("TOTAL", r.total)
	return t
}

// krdTable has one row per key tenor of the portfolio.
func (r *riskReport) krdTable() *table {
	t := &table{cols: []string{"tenor", "years", "krd", "key_dv01"}}
	for k, l := range r.labels {
		t.add(l, r.total.Keys[k], ratio(r.total.Degenerate, snap(r.total.KRD[k])), snap(r.total.KeyDV01[k]))
	}
	return t
}

// scenarioTable has one row per scenario: the full revaluation and the
// approximation from the risk measures.
func (r *riskReport) scenarioTable() *table {
	t := &table{cols: []string{"scenario", "pv_change", "pv_change_pct", "approx_change", "approx_error", "approx_basis"}}
	for i, s := range r.cfg.Scenarios {
		chg := r.total.Change[i]
		var pct any
		if !r.total.Degenerate {
			pct = 100 * chg / math.Abs(r.total.Dirty)
		}
		ap := r.total.Approx(s)
		basis := "krd"
		if s.Parallel {
			basis = "duration_convexity"
		}
		t.add(s.Name, chg, pct, ap, ap-chg, basis)
	}
	return t
}

// positionTable has one row per position (failed rows with the status).
func (r *riskReport) positionTable() *table {
	cols := []string{"id"}
	if r.hasIssuer {
		cols = append(cols, "issuer")
	}
	if r.hasRating {
		cols = append(cols, "rating")
	}
	cols = append(cols, "nominal", "price_source", "mv_dirty", "mv_clean", "accrued", "spread_bp", "mod_duration", "convexity", "dv01")
	cols = append(cols, r.krdCols()...)
	t := &table{cols: append(cols, "status")}
	for _, p := range r.rows {
		row := []any{p.id}
		if r.hasIssuer {
			row = append(row, p.issuer)
		}
		if r.hasRating {
			row = append(row, p.rating)
		}
		if !p.ok {
			row = append(row, p.nominal)
			row = append(row, make([]any, len(t.cols)-len(row)-1)...)
			t.add(append(row, p.status)...)
			continue
		}
		x := p.res
		row = append(row, p.nominal, p.source, x.Dirty, x.Clean, x.Accrued, p.spreadBP, x.Duration, x.Convexity, x.DV01)
		for _, k := range x.KRD {
			row = append(row, snap(k))
		}
		t.add(append(row, p.status)...)
	}
	return t
}

// section returns the table of the named section; the summary of a
// grouped report is the group table.
func (r *riskReport) section(name string) *table {
	switch name {
	case "summary":
		if r.groupBy != "" {
			return r.groupTable()
		}
		return r.summary()
	case "krd":
		return r.krdTable()
	case "scenarios":
		return r.scenarioTable()
	}
	return r.positionTable()
}

// write writes the report: one section, or (section "") all of them in text
// and json.
func (r *riskReport) write(w io.Writer, f format, section string) error {
	if section != "" {
		t := r.section(section)
		if section == "summary" && r.groupBy == "" && f != formatCSV {
			return writeRecord(w, t.cols, t.rows[0], f)
		}
		return writeTable(w, t, f)
	}
	bw := bufio.NewWriter(w)
	switch f {
	case formatJSON:
		s := r.summary()
		fmt.Fprintf(bw, "{\n\"settlement\": %q,\n\"summary\": %s", r.settle, jsonObject(s.cols, s.rows[0]))
		parts := []struct {
			key string
			t   *table
			on  bool
		}{{"groups", r.groupTable(), r.groupBy != ""}, {"krd", r.krdTable(), true}, {"scenarios", r.scenarioTable(), len(r.cfg.Scenarios) > 0}, {"positions", r.positionTable(), true}}
		for _, p := range parts {
			if !p.on {
				continue
			}
			var buf bytes.Buffer
			if err := writeTable(&buf, p.t, formatJSON); err != nil {
				return err
			}
			fmt.Fprintf(bw, ",\n%q: %s", p.key, strings.TrimSpace(buf.String()))
		}
		bw.WriteString("\n}\n")
	default:
		s := r.summary()
		fmt.Fprintf(bw, "Portfolio risk as of %s: %d positions, %d excluded\n", r.settle, r.total.N, r.failed)
		if err := writeRecord(bw, s.cols[2:], s.rows[0][2:], formatText); err != nil {
			return err
		}
		if r.groupBy != "" {
			fmt.Fprintf(bw, "\nBy %s\n", r.groupBy)
			if err := writeTable(bw, r.groupTable(), formatText); err != nil {
				return err
			}
		}
		fmt.Fprintf(bw, "\nKey rate durations (years; key_dv01 in currency per 1bp)\n")
		if err := writeTable(bw, r.krdTable(), formatText); err != nil {
			return err
		}
		if len(r.cfg.Scenarios) > 0 {
			fmt.Fprintf(bw, "\nScenarios (full revaluation, spreads held; pv_change_pct of |PV|)\n")
			if err := writeTable(bw, r.scenarioTable(), formatText); err != nil {
				return err
			}
		}
	}
	return bw.Flush()
}
