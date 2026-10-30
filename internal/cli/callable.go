package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/konimarti/fixedincome/pkg/cashflow"
	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/lattice"
	"github.com/konimarti/fixedincome/pkg/term"
)

// callTerm is a call or put right: the date and the clean price in percent
// of face.
type callTerm struct {
	date  time.Time
	price float64
}

// callTerms are the embedded options of a bond: calls and puts on listed
// dates and optionally calls on every coupon date from callFrom.
type callTerms struct {
	calls    []callTerm
	callFrom *callTerm
	puts     []callTerm
}

// any reports whether there is an embedded option.
func (c *callTerms) any() bool {
	return c != nil && (len(c.calls) > 0 || c.callFrom != nil || len(c.puts) > 0)
}

// hwParams are the Hull-White parameters of the lattice: sigma is the
// absolute short rate volatility (decimal per year), a the mean reversion
// (per year) and steps the maximum number of lattice steps per year.
type hwParams struct {
	sigma, a float64
	steps    int
}

// parseCallTerm parses DATE@PRICE (date YYYY-MM-DD or DD.MM.YYYY, clean
// price in percent of face).
func parseCallTerm(s string) (callTerm, error) {
	i := strings.LastIndex(s, "@")
	if i < 0 {
		return callTerm{}, fmt.Errorf("invalid call term %q (expected DATE@PRICE)", s)
	}
	d, err := parseDate(strings.TrimSpace(s[:i]))
	if err != nil {
		return callTerm{}, fmt.Errorf("invalid date in %q (YYYY-MM-DD or DD.MM.YYYY)", s)
	}
	p, err := parseFinite(s[i+1:])
	if err != nil || !(p > 0) {
		return callTerm{}, fmt.Errorf("invalid price in %q (positive, percent of face)", s)
	}
	return callTerm{date: d, price: p}, nil
}

// parseCallTerms parses a list of DATE@PRICE terms separated by sep; the
// result is sorted by date. An empty string gives no terms.
func parseCallTerms(s, sep string) ([]callTerm, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var out []callTerm
	for _, p := range strings.Split(s, sep) {
		t, err := parseCallTerm(strings.TrimSpace(p))
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].date.Before(out[j].date) })
	return out, nil
}

// callable is the valuation of the embedded options (prices per 100 of
// face, spreads decimals).
type callable struct {
	// oas is the option-adjusted spread over the curve (market only)
	oas float64
	// ytw is the yield to worst (cc, decimal) and ytwDate its redemption date
	// (market only)
	ytw     float64
	ytwDate time.Time
	// valueStraight, valueCallable are the clean values with the straight
	// and the callable bond at the OAS (on the curve without a market
	// price); option is their difference
	valueStraight, valueCallable, option float64
	// straightDuration is the cash-flow duration of the straight bond to
	// maturity at the market price
	straightDuration float64
}

// valueCallable values the bond with its embedded options on the
// Hull-White lattice. dirty is the market dirty price if market is true,
// otherwise the bond is valued on the curve (OAS 0 relative to the curve
// including its spread). It fills the model price, the spreads and the
// effective risk measures of v.
func valueCallable(in bondInput, v *bondValue, b bond.Straight, settle time.Time, curve term.Curve, cfs []cashflow.CashFlow, accrued, dirty float64) error {
	scale := 100 / in.face
	ct := in.opt
	var calls []bond.Call
	for _, c := range ct.calls {
		calls = append(calls, bond.Call{Date: c.date, Price: c.price})
	}
	if ct.callFrom != nil {
		cs, err := b.CallsOnCoupons(ct.callFrom.date, ct.callFrom.price)
		if err != nil {
			return err
		}
		if len(cs) == 0 {
			return fmt.Errorf("no coupon date from %s before maturity", formatDate(ct.callFrom.date))
		}
		calls = append(calls, cs...)
		sort.SliceStable(calls, func(i, j int) bool { return calls[i].Date.Before(calls[j].Date) })
	}
	var puts []bond.Put
	for _, p := range ct.puts {
		puts = append(puts, bond.Put{Date: p.date, Price: p.price})
	}
	c, err := bond.NewCallable(b, calls, bond.WithPuts(puts...))
	if err != nil {
		return err
	}
	opts := []lattice.Option{lattice.WithStepsPerYear(in.hw.steps)}
	a, sigma := in.hw.a, in.hw.sigma
	model, err := c.Value(settle, curve, a, sigma, 0, opts...)
	if err != nil {
		return err
	}
	v.cleanModel = (model - accrued) * scale
	oas := 0.0
	if v.market {
		if oas, err = c.OAS(dirty, settle, curve, a, sigma, opts...); err != nil {
			return fmt.Errorf("oas: %w", err)
		}
		y, d, err := c.YieldToWorst(dirty, settle)
		if err != nil {
			return fmt.Errorf("yield to worst: %w", err)
		}
		v.call.ytw, v.call.ytwDate = y, d
	} else {
		// valued on the curve: dirty and clean are the model values
		v.dirty = model * scale
		v.clean = v.dirty - v.accrued
	}
	v.call.oas = oas
	v.spreadTotal = v.spreadTotal - v.zspread + oas
	atOAS, err := c.Value(settle, curve, a, sigma, oas, opts...)
	if err != nil {
		return err
	}
	straight := cashflow.PV(cfs, term.WithSpread(curve, oas))
	v.call.valueCallable = (atOAS - accrued) * scale
	v.call.valueStraight = (straight - accrued) * scale
	v.call.option = v.call.valueStraight - v.call.valueCallable
	v.call.straightDuration = v.duration
	if v.duration, err = c.EffectiveDuration(settle, curve, a, sigma, oas, opts...); err != nil {
		return err
	}
	if v.convexity, err = c.EffectiveConvexity(settle, curve, a, sigma, oas, opts...); err != nil {
		return err
	}
	if v.dv01, err = c.DV01(settle, curve, a, sigma, oas, opts...); err != nil {
		return err
	}
	v.dv01 *= scale
	return nil
}

// callableCols are the additional columns of a callable bond with a market
// price (without: only the straight_duration and value columns).
var callableCols = []string{"oas_bp", "ytw_cc_pct", "ytw_date", "straight_duration", "option_value", "value_straight", "value_callable"}

// callableVals returns the values of callableCols (nil for the columns
// that need a market price if there is none).
func (v bondValue) callableVals() []any {
	if v.call == nil {
		return make([]any, len(callableCols))
	}
	c := v.call
	if !v.market {
		return []any{nil, nil, nil, c.straightDuration, c.option, c.valueStraight, c.valueCallable}
	}
	return []any{toBP(c.oas), toPct(c.ytw), formatDate(c.ytwDate), c.straightDuration, c.option, c.valueStraight, c.valueCallable}
}
