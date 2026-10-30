package cli

import (
	"flag"
	"math"

	"github.com/konimarti/fixedincome/pkg/term"
)

func buildCurveCompare(e *env) (*flag.FlagSet, func() error) {
	fs := flag.NewFlagSet("curve compare", flag.ContinueOnError)
	var ff formatFlags
	ff.register(fs)
	grid := fs.String("grid", "", "time grid start:end:step (default 1m:10y:1m)")
	ts := fs.String("t", "", "comma-separated maturities instead of a grid, e.g. 1y,2y,5y")
	return fs, func() error {
		if len(e.args) != 2 {
			return usagef("two curve files are required: fic curve compare A.json B.json")
		}
		pa, pb := e.args[0], e.args[1]
		if pa == "-" && pb == "-" {
			return usagef("only one of the curve files can be read from stdin")
		}
		times, err := curveGrid(*grid, *ts, "1m:10y:1m")
		if err != nil {
			return err
		}
		fmtOut, err := resolveFormat(ff.format, true, e.stdout, ff.out != "" && ff.out != "-")
		if err != nil {
			return err
		}
		a, ma, err := readCurveFile(e, pa)
		if err != nil {
			return err
		}
		b, mb, err := readCurveFile(e, pb)
		if err != nil {
			return err
		}
		if !ma.AsOf.IsZero() && !mb.AsOf.IsZero() && !ma.AsOf.Equal(mb.AsOf) {
			e.warnf("the asof dates differ: %s (A) and %s (B)", formatDate(ma.AsOf), formatDate(mb.AsOf))
		}
		out, err := openOutput(ff.out, e.stdout, pa, pb)
		if err != nil {
			return err
		}
		t := &table{cols: []string{"t_years", "a_zero_cc_pct", "b_zero_cc_pct", "diff_zero_bp",
			"a_fwd_cc_pct", "b_fwd_cc_pct", "diff_fwd_bp"}}
		var sum, sumSq, maxAbs, maxAt float64
		n := 0
		for _, x := range times {
			za, zb := a.ZeroRate(x), b.ZeroRate(x)
			fa, fb := term.InstForward(a, x), term.InstForward(b, x)
			d := toBP(zb - za)
			t.add(x, toPct(za), toPct(zb), d, toPct(fa), toPct(fb), toBP(fb-fa))
			if isNaNInf(d) {
				continue
			}
			n++
			sum += d
			sumSq += d * d
			if math.Abs(d) > maxAbs {
				maxAbs, maxAt = math.Abs(d), x
			}
		}
		if err := out.finish(writeTable(out.w, t, fmtOut)); err != nil {
			return err
		}
		if n > 0 {
			e.infof("diff_zero_bp (B minus A) at %d maturities: mean %.4g, max abs %.4g (at %.4g y), rms %.4g",
				n, sum/float64(n), maxAbs, maxAt, math.Sqrt(sumSq/float64(n)))
		}
		return nil
	}
}
