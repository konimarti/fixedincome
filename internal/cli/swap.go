package cli

import (
	"flag"
	"fmt"
	"math"
	"strings"

	"github.com/konimarti/fixedincome/pkg/term"
)

func buildSwapRate(e *env) (*flag.FlagSet, func() error) {
	fs := flag.NewFlagSet("swap rate", flag.ContinueOnError)
	var cf curveFlags
	var ff formatFlags
	cf.register(fs, false)
	ff.register(fs)
	fwd := fs.String("fwd-curve", "", "forecast curve of the floating leg (default: same as --curve, single curve swap)")
	tenors := fs.String("tenors", "2y,3y,4y,5y,6y,7y,8y,9y,10y", "comma-separated swap tenors (multiples of the fixed leg period)")
	freq := fs.Int("freq", 2, "fixed leg payments per year (swaps start at the curve date; there is no --date)")
	return fs, func() error {
		if *freq <= 0 || *freq > 12 {
			return usagef("invalid --freq %d (1..12)", *freq)
		}
		var ys []float64
		var labels []string
		for _, s := range strings.Split(*tenors, ",") {
			s = strings.TrimSpace(s)
			y, err := parseYears(s)
			if err != nil || !(y > 0) {
				return usagef("invalid tenor %q in --tenors (examples: 2y, 18m)", s)
			}
			n := y * float64(*freq)
			if math.Abs(n-math.Round(n)) > 1e-6 {
				return usagef("tenor %q is not a multiple of the fixed leg period 1/%d years", s, *freq)
			}
			ys = append(ys, y)
			labels = append(labels, s)
		}
		fmtOut, err := resolveFormat(ff.format, true, e.stdout, ff.out != "" && ff.out != "-")
		if err != nil {
			return err
		}
		l, err := cf.load(e)
		if err != nil {
			return err
		}
		var forecast term.Curve = l.curve
		if *fwd != "" {
			if forecast, _, err = readCurveFile(e, *fwd); err != nil {
				return err
			}
		}
		out, err := openOutput(ff.out, e.stdout, cf.path, *fwd)
		if err != nil {
			return err
		}
		t := &table{cols: []string{"tenor", "tenor_years", "freq", "par_rate_pct", "annuity"}}
		for i, y := range ys {
			rate, annuity, err := parRate(y, *freq, l.curve, forecast)
			if err != nil {
				return out.finish(fmt.Errorf("tenor %s: %w", labels[i], err))
			}
			t.add(labels[i], y, *freq, toPct(rate), annuity)
		}
		return out.finish(writeTable(out.w, t, fmtOut))
	}
}

// parRate returns the par rate of a swap starting today with fixed payments
// freq times per year until the tenor (years) and its annuity
// sum(Z(t_i))/freq. The par rate is the annualised fixed-leg coupon
// (freq-period rate times freq): the floating leg projected on the forecast
// curve and discounted on the discount curve, divided by the annuity. With
// one curve this is freq (1 - Z(T)) / sum Z(t_i), see swap.ParRateFromTimes.
func parRate(tenor float64, freq int, disc, forecast term.Curve) (par, annuity float64, err error) {
	n := int(math.Round(tenor * float64(freq)))
	if n < 1 {
		return 0, 0, fmt.Errorf("tenor too short")
	}
	var floatLeg float64
	prev := 1.0 // forecast discount factor at t_{i-1}
	for i := 1; i <= n; i++ {
		t := float64(i) / float64(freq)
		zd := disc.Discount(t)
		zf := forecast.Discount(t)
		floatLeg += zd * (prev/zf - 1)
		prev = zf
		annuity += zd / float64(freq)
	}
	if !(annuity > 0) || math.IsInf(annuity, 0) {
		return 0, 0, fmt.Errorf("annuity %v is not positive", annuity)
	}
	return floatLeg / annuity, annuity, nil
}
