package cli

import (
	"errors"
	"flag"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/konimarti/fixedincome/pkg/cashflow"
	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/rate"
	"github.com/konimarti/fixedincome/pkg/term"
)

// bondInput is a fixed-coupon bond with the optional market quote.
type bondInput struct {
	maturity time.Time
	// coupon is a decimal
	coupon float64
	freq   maturity.Frequency
	dc     maturity.DayCount
	// face is the face value (> 0), redemption defaults to face
	face       float64
	redemption *float64
	// price is the clean market price in percent of face; yield the
	// continuously compounded market yield (decimal); at most one is set
	price, yield *float64
}

// bondValue is the valuation of a bond. Prices are in percent of face (per
// 100 of face value), z-spread and yields are decimals.
type bondValue struct {
	ttm        float64
	accrued    float64
	dirty      float64
	clean      float64
	cleanModel float64
	yieldCC    float64
	yieldAnn   float64
	zspread    float64
	// spreadTotal is the spread over the curve without any spread: the
	// spread_bp of the curve file plus --spread plus the z-spread
	spreadTotal float64
	// market is true if the yields and spreads refer to a market price
	// (quote or yield), false if the bond is valued on the curve
	market    bool
	duration  float64
	convexity float64
	dv01      float64
}

// valueBond values the bond at the settlement date. The market dirty price
// is the quoted clean price plus accrued interest, or the price of the
// yield, or (without a quote) the price from the curve. The yields are
// those of the market dirty price, the z-spread is relative to the curve
// (including the spread of the file and of --spread), the total spread is
// the spread over the curve without spread (file spread + --spread +
// z-spread), and duration, convexity and DV01 are evaluated on the curve
// shifted by the z-spread, i.e. at the market price (on the curve itself
// without a quote).
func valueBond(in bondInput, settle time.Time, curve term.Curve) (bondValue, error) {
	var v bondValue
	if !settle.Before(in.maturity) {
		return v, fmt.Errorf("maturity %s is not after the settlement date %s", formatDate(in.maturity), formatDate(settle))
	}
	opts := []bond.Option{bond.WithFace(in.face)}
	if in.redemption != nil {
		opts = append(opts, bond.WithRedemption(*in.redemption))
	}
	b, err := bond.NewStraight(maturity.Schedule{Maturity: in.maturity, Frequency: in.freq, DayCount: in.dc}, in.coupon, opts...)
	if err != nil {
		return v, err
	}
	cfs, err := b.CashFlows(settle)
	if err != nil {
		return v, err
	}
	accrued, err := b.Accrued(settle)
	if err != nil {
		return v, err
	}
	scale := 100 / in.face
	v.market = in.price != nil || in.yield != nil
	v.ttm = maturity.YearFraction(settle, in.maturity)
	v.accrued = accrued * scale
	modelDirty := cashflow.PV(cfs, curve)
	v.cleanModel = (modelDirty - accrued) * scale
	dirty := modelDirty
	switch {
	case in.price != nil:
		dirty = *in.price*in.face/100 + accrued
	case in.yield != nil:
		dirty = cashflow.PV(cfs, term.Flat{Rate: *in.yield})
	}
	v.dirty = dirty * scale
	v.clean = v.dirty - v.accrued
	if v.yieldCC, err = cashflow.YieldCC(dirty, cfs); err != nil {
		return v, fmt.Errorf("yield: %w", err)
	}
	if v.yieldAnn, err = rate.FromContinuous(v.yieldCC, 1); err != nil {
		return v, err
	}
	risk := curve
	_, curveSpread := term.Spread(curve)
	if v.market {
		z, err := cashflow.ZSpread(dirty, cfs, curve)
		if err != nil {
			return v, fmt.Errorf("z-spread: %w", err)
		}
		v.zspread = z
		risk = term.WithSpread(curve, z)
	}
	v.spreadTotal = curveSpread + v.zspread
	v.duration = cashflow.Duration(cfs, risk)
	v.convexity = cashflow.Convexity(cfs, risk)
	v.dv01 = cashflow.DV01(cfs, risk) * scale
	return v, nil
}

// parseFreq parses --freq (usage error listing the valid values).
func parseFreq(s string) (maturity.Frequency, error) {
	f, err := maturity.ParseFrequency(s)
	if err != nil {
		return 0, usagef("invalid --freq %q (valid: 1, 2, 3, 4, 6, 12)", s)
	}
	return f, nil
}

// parseDayCountFlag parses --daycount (usage error listing the valid names).
func parseDayCountFlag(s string) (maturity.DayCount, error) {
	d, err := maturity.ParseDayCount(s)
	if err != nil {
		return 0, usagef("invalid --daycount %q (valid: %s)", s, strings.Join(maturity.DayCountNames(), ", "))
	}
	return d, nil
}

// bondCols are the columns of the single bond record; bondTableCols those of
// the batch table.
var bondRecordCols = []string{"settlement", "maturity", "ttm_years", "accrued", "price_dirty", "price_clean",
	"price_clean_model", "yield_cc_pct", "yield_annual_pct", "zspread_bp", "spread_total_bp", "mod_duration", "convexity", "dv01"}

func buildBondPrice(e *env) (*flag.FlagSet, func() error) {
	fs := flag.NewFlagSet("bond price", flag.ContinueOnError)
	var cf curveFlags
	var ff formatFlags
	cf.register(fs, true)
	ff.register(fs)
	matFlag := fs.String("maturity", "", "maturity date YYYY-MM-DD or DD.MM.YYYY (or a tenor such as 5y from the valuation date)")
	coupon := fs.Float64("coupon", math.NaN(), "annual coupon in percent of face")
	freq := fs.String("freq", "1", "coupons per year (1, 2, 3, 4, 6, 12)")
	dayCount := fs.String("daycount", "30E360", "accrual day count: "+strings.Join(maturity.DayCountNames(), ", "))
	redemption := fs.Float64("redemption", 0, "redemption value (default: the face value)")
	face := fs.Float64("face", 100, "face value")
	price := fs.Float64("price", math.NaN(), "quoted clean price in percent of face (yields and z-spread are implied from it)")
	yield := fs.Float64("yield", math.NaN(), "continuously compounded yield in percent (the price is computed from it)")
	in := fs.String("in", "", "batch mode: CSV file with the bonds (- for stdin); columns maturity,coupon_pct,price_clean[,id,freq,daycount,face,redemption,issuer,rating]")
	groupBy := fs.String("group-by", "", "with --in: one row per group of the column issuer|rating|freq|daycount instead of one per bond (n, duration-weighted z-spread, mean duration, DV01 sum)")
	return fs, func() error {
		set := setFlags(fs)
		if *in != "" {
			for _, n := range []string{"maturity", "coupon", "price", "yield"} {
				if set[n] {
					return usagef("--%s cannot be combined with --in (put it into the CSV file)", n)
				}
			}
		}
		if set["group-by"] {
			if *in == "" {
				return usagef("--group-by needs --in FILE")
			}
			*groupBy = strings.ToLower(strings.TrimSpace(*groupBy))
			if !groupColumns[*groupBy] {
				return usagef("invalid --group-by %q (issuer, rating, freq, daycount)", *groupBy)
			}
		}
		if set["price"] && set["yield"] {
			return usagef("--price and --yield cannot be combined")
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
		if set["redemption"] && !(*redemption >= 0) {
			return usagef("invalid --redemption %v", *redemption)
		}
		var red *float64
		if set["redemption"] {
			red = redemption
		}
		if *in == "" {
			if *matFlag == "" || !set["coupon"] {
				return usagef("--maturity and --coupon are required (or --in FILE for several bonds)")
			}
			if math.IsInf(*coupon, 0) {
				return usagef("invalid --coupon")
			}
		}
		fmtOut, err := resolveFormat(ff.format, *in != "", e.stdout, ff.out != "" && ff.out != "-")
		if err != nil {
			return err
		}
		d := bondDefaults{freq: fr, dc: dc, face: *face, redemption: red}
		if *in != "" {
			return bondBatch(e, &cf, &ff, fmtOut, *in, *groupBy, d)
		}
		// single bond
		if set["price"] && !(*price > 0) {
			return usagef("invalid --price %v: must be positive", *price)
		}
		if set["yield"] && (math.IsNaN(*yield) || math.IsInf(*yield, 0)) {
			return usagef("invalid --yield")
		}
		l, err := cf.load(e)
		if err != nil {
			return err
		}
		mat, err := parseMaturityDate(*matFlag, l.settle)
		if err != nil {
			return usagef("%v", err)
		}
		bi := bondInput{maturity: mat, coupon: fromPct(*coupon), freq: fr, dc: dc, face: *face, redemption: red}
		if set["price"] {
			bi.price = price
		}
		if set["yield"] {
			y := fromPct(*yield)
			bi.yield = &y
		}
		out, err := openOutput(ff.out, e.stdout, cf.path)
		if err != nil {
			return err
		}
		v, err := valueBond(bi, l.settle, l.curve)
		if err != nil {
			return out.finish(err)
		}
		cols := bondRecordCols
		vals := []any{formatDate(l.settle), formatDate(mat), v.ttm, v.accrued, v.dirty, v.clean, v.cleanModel,
			toPct(v.yieldCC), toPct(v.yieldAnn), toBP(v.zspread), toBP(v.spreadTotal), v.duration, v.convexity, v.dv01}
		if fmtOut == formatText && !v.market {
			// no market quote: the z-spread is trivially 0 (the total spread is the
			// spread of the curve) and not shown
			var c2 []string
			var v2 []any
			for i, c := range cols {
				if c != "zspread_bp" {
					c2, v2 = append(c2, c), append(v2, vals[i])
				}
			}
			cols, vals = c2, v2
		}
		return out.finish(writeRecord(out.w, cols, vals, fmtOut))
	}
}

// bondDefaults are the defaults of the bond attributes missing in a CSV row.
type bondDefaults struct {
	freq       maturity.Frequency
	dc         maturity.DayCount
	face       float64
	redemption *float64
}

// bondTableCols are the columns of the batch output.
func bondTableCols(withIssuer, withRating bool) []string {
	cols := []string{"id"}
	if withIssuer {
		cols = append(cols, "issuer")
	}
	if withRating {
		cols = append(cols, "rating")
	}
	return append(cols, "maturity", "coupon_pct", "freq", "daycount", "ttm_years", "accrued", "price_source",
		"price_clean", "price_dirty", "price_clean_model", "yield_cc_pct", "yield_annual_pct", "zspread_bp",
		"spread_total_bp", "mod_duration", "convexity", "dv01", "status")
}

// groupColumns are the columns of the batch output that --group-by accepts.
var groupColumns = map[string]bool{"issuer": true, "rating": true, "freq": true, "daycount": true}

// groupStats aggregates the bonds with a market price per group: the
// z-spread is the duration-weighted average (sum(z*d) / sum(d)), the
// duration the mean.
type groupStats struct {
	rows  int
	order []string
	g     map[string]*group
}

type group struct {
	n           int
	zd, d, dv01 float64
}

func (s *groupStats) add(key string, zspreadBP, duration, dv01 float64) {
	if s.g == nil {
		s.g = map[string]*group{}
	}
	key = strings.TrimSpace(key)
	if key == "" {
		key = "unknown"
	}
	x := s.g[key]
	if x == nil {
		x = &group{}
		s.g[key] = x
		s.order = append(s.order, key)
	}
	s.rows++
	x.n++
	x.zd += zspreadBP * duration
	x.d += duration
	x.dv01 += dv01
}

// table returns one row per group sorted by the average z-spread (then by
// name); name is the title of the first column.
func (s *groupStats) table(name string) *table {
	t := &table{cols: []string{name, "n", "avg_zspread_bp", "avg_mod_duration", "dv01_sum"}}
	avg := func(x *group) float64 {
		if x.d > 0 {
			return x.zd / x.d
		}
		return 0
	}
	keys := append([]string(nil), s.order...)
	sort.SliceStable(keys, func(i, j int) bool {
		ai, aj := avg(s.g[keys[i]]), avg(s.g[keys[j]])
		if ai != aj {
			return ai < aj
		}
		return keys[i] < keys[j]
	})
	for _, k := range keys {
		x := s.g[k]
		var key any = k
		if f, err := strconv.Atoi(k); err == nil && name == "freq" {
			key = f
		}
		t.add(key, x.n, avg(x), x.d/float64(x.n), x.dv01)
	}
	return t
}

// bondBatch values the bonds of a CSV file. Failing rows get an error text
// in the status column instead of aborting; the exit code is 1 only if all
// rows failed.
func bondBatch(e *env, cf *curveFlags, ff *formatFlags, fmtOut format, path, groupBy string, d bondDefaults) error {
	r, closeIn, err := openInput(e, path)
	if err != nil {
		return err
	}
	bf, err := readBondRows(r)
	closeIn()
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if groupBy == "issuer" && !bf.hasIssuer {
		return fmt.Errorf("%s: --group-by issuer needs an issuer column in the input", path)
	}
	if groupBy == "rating" && !bf.hasRating {
		return fmt.Errorf("%s: --group-by rating needs a rating column in the input", path)
	}
	l, err := cf.load(e)
	if err != nil {
		return err
	}
	out, err := openOutput(ff.out, e.stdout, cf.path, path)
	if err != nil {
		return err
	}
	t := &table{cols: bondTableCols(bf.hasIssuer, bf.hasRating)}
	var failedIDs []string
	failed := 0
	groups := &groupStats{}
	for _, b := range bf.rows {
		var v bondValue
		fr, dc, face, red := d.freq, d.dc, d.face, d.redemption
		if b.freq != 0 {
			fr = b.freq
		}
		if b.dcSet {
			dc = b.dc
		}
		if b.face > 0 {
			face = b.face
		}
		if b.hasRed {
			x := b.redemption
			red = &x
		}
		err := b.err
		if err == nil {
			bi := bondInput{maturity: b.maturity, coupon: b.coupon, freq: fr, dc: dc, face: face, redemption: red}
			if b.hasPrice {
				p := b.price
				bi.price = &p
			}
			v, err = valueBond(bi, l.settle, l.curve)
		}
		row := []any{b.id}
		if bf.hasIssuer {
			row = append(row, b.issuer)
		}
		if bf.hasRating {
			row = append(row, b.rating)
		}
		if err != nil {
			// failed rows echo the raw input fields
			failed++
			if len(failedIDs) < 3 {
				failedIDs = append(failedIDs, b.id)
			}
			var cpn any = b.couponRaw
			if x, perr := parseFinite(b.couponRaw); perr == nil {
				cpn = x
			}
			row = append(row, b.matRaw, cpn, b.freqRaw, b.dcRaw, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err.Error())
			t.add(row...)
			continue
		}
		source := "market"
		yc, ya, zs, st := toPct(v.yieldCC), toPct(v.yieldAnn), any(toBP(v.zspread)), any(toBP(v.spreadTotal))
		var ycv, yav any = yc, ya
		if !v.market {
			source = "model"
			ycv, yav, zs, st = nil, nil, nil, nil
		}
		row = append(row, formatDate(b.maturity), toPct(b.coupon), int(fr), dc.String(), v.ttm, v.accrued, source,
			v.clean, v.dirty, v.cleanModel, ycv, yav, zs, st, v.duration, v.convexity, v.dv01, "ok")
		t.add(row...)
		if v.market {
			key := ""
			switch groupBy {
			case "issuer":
				key = b.issuer
			case "rating":
				key = b.rating
			case "freq":
				key = strconv.Itoa(int(fr))
			case "daycount":
				key = dc.String()
			}
			groups.add(key, toBP(v.zspread), v.duration, v.dv01)
		}
	}
	if groupBy != "" {
		t = groups.table(groupBy)
		if skipped := len(bf.rows) - groups.rows; skipped > 0 {
			e.warnf("%d of %d rows not in the groups (status not ok or no market price)", skipped, len(bf.rows))
		}
	}
	if err := out.finish(writeTable(out.w, t, fmtOut)); err != nil {
		return err
	}
	if failed > 0 {
		more := ""
		if failed > len(failedIDs) {
			more = ", ..."
		}
		e.warnf("%d of %d rows failed (see the status column; first: %s%s)", failed, len(bf.rows), strings.Join(failedIDs, ", "), more)
	}
	if failed == len(bf.rows) {
		return errors.New("all rows failed")
	}
	return nil
}
