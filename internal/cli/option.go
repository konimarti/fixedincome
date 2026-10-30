package cli

import (
	"errors"
	"flag"
	"math"

	"github.com/konimarti/fixedincome/pkg/instrument/option"
	"github.com/konimarti/fixedincome/pkg/term"
)

func buildOptionPrice(e *env) (*flag.FlagSet, func() error) {
	fs := flag.NewFlagSet("option price", flag.ContinueOnError)
	var ff formatFlags
	ff.register(fs)
	typ := fs.String("type", "", "call or put")
	spot := fs.Float64("spot", math.NaN(), "price of the underlying")
	strike := fs.Float64("strike", math.NaN(), "strike price")
	expiry := fs.String("expiry", "", "expiry: years (2), tenor (6m, 2y) or date YYYY-MM-DD / DD.MM.YYYY (relative to --date)")
	vol := fs.Float64("vol", math.NaN(), "volatility in percent (30 = 30%)")
	rateFlag := fs.Float64("rate", math.NaN(), "flat continuously compounded interest rate in percent")
	div := fs.Float64("div", 0, "continuous dividend yield in percent")
	curve := fs.String("curve", "", "curve file: the rate is its zero rate at the expiry (instead of --rate)")
	date := fs.String("date", "", "valuation date YYYY-MM-DD or DD.MM.YYYY for a date --expiry (default: asof date of --curve, else today)")
	implied := fs.String("implied-from", "", "option price: solve for the implied volatility (replaces --vol)")
	return fs, func() error {
		set := setFlags(fs)
		if *typ == "" {
			return usagef("--type is required (call or put)")
		}
		ot, err := option.ParseType(*typ)
		if err != nil {
			return usagef("invalid --type %q (call or put)", *typ)
		}
		if math.IsNaN(*spot) || math.IsNaN(*strike) || *expiry == "" {
			return usagef("--spot, --strike and --expiry are required")
		}
		if set["curve"] == set["rate"] {
			return usagef("exactly one of --rate and --curve is required")
		}
		var impliedPrice float64
		if *implied != "" {
			if set["vol"] {
				return usagef("--vol and --implied-from cannot be combined")
			}
			if impliedPrice, err = parseFinite(*implied); err != nil || !(impliedPrice >= 0) {
				return usagef("invalid --implied-from %q", *implied)
			}
		} else if !(*vol > 0) || math.IsInf(*vol, 0) {
			return usagef("--vol is required and must be positive (percent, e.g. 30), or use --implied-from")
		}
		if !(*spot > 0) || !(*strike > 0) || math.IsInf(*spot, 0) || math.IsInf(*strike, 0) {
			return usagef("--spot and --strike must be positive")
		}
		for name, v := range map[string]float64{"rate": *rateFlag, "div": *div} {
			if set[name] && (math.IsNaN(v) || math.IsInf(v, 0)) {
				return usagef("invalid --%s", name)
			}
		}
		fmtOut, err := resolveFormat(ff.format, false, e.stdout, ff.out != "" && ff.out != "-")
		if err != nil {
			return err
		}
		var c term.Curve
		settle := today()
		if set["curve"] {
			cc, meta, err := readCurveFile(e, *curve)
			if err != nil {
				return err
			}
			c = cc
			if !meta.AsOf.IsZero() {
				settle = meta.AsOf
			}
		} else {
			c = term.Flat{Rate: fromPct(*rateFlag)}
		}
		if *date != "" {
			if settle, err = parseDate(*date); err != nil {
				return usagef("invalid --date: %v", err)
			}
		}
		T, err := parseExpiry(*expiry, settle)
		if err != nil {
			return usagef("invalid --expiry: %v", err)
		}
		if !(T > 0) {
			return usagef("--expiry %q is not after the valuation date %s", *expiry, formatDate(settle))
		}
		o := option.European{Type: ot, S: *spot, K: *strike, T: T, Q: fromPct(*div), Vol: fromPct(*vol)}
		out, err := openOutput(ff.out, e.stdout, *curve)
		if err != nil {
			return err
		}
		if *implied != "" {
			iv, err := option.ImpliedVol(impliedPrice, o, c)
			if err != nil {
				if errors.Is(err, option.ErrNoImpliedVol) || errors.Is(err, option.ErrNoVega) {
					err = errors.New("implied volatility: " + err.Error())
				}
				return out.finish(err)
			}
			o.Vol = iv
		}
		price, err := o.Price(c)
		if err != nil {
			return out.finish(err)
		}
		g, err := o.Greeks(c)
		if err != nil {
			return out.finish(err)
		}
		typeName := "call"
		if ot == option.Put {
			typeName = "put"
		}
		// Greeks are scaled to practical units at the boundary
		cols := []string{"type", "spot", "strike", "expiry_years", "vol_pct", "rate_cc_pct", "div_pct", "price", "delta", "gamma",
			"vega_per_volpt", "theta_per_day", "rho_per_pct", "phi_per_pct"}
		vals := []any{typeName, o.S, o.K, o.T, toPct(o.Vol), toPct(c.ZeroRate(o.T)), toPct(o.Q), price, g.Delta, g.Gamma,
			g.Vega / 100, g.Theta / 365, g.Rho / 100, g.Phi / 100}
		return out.finish(writeRecord(out.w, cols, vals, fmtOut))
	}
}
