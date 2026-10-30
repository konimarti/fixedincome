package cli

import (
	"fmt"
	"time"

	"github.com/konimarti/fixedincome/internal/report"
	"github.com/konimarti/fixedincome/pkg/term"
)

// fitReport converts the outcomes of a curve fit to the content of the HTML
// report. The residuals are those of the residuals file (residualRows),
// converted back from its table cells, so that both files show the same
// numbers.
func fitReport(asof time.Time, source string, nBonds int, outs []fitOutcome, warnings []string) report.Report {
	r := report.Report{AsOf: formatDate(asof), Source: source, Bonds: nBonds, Warnings: warnings}
	t := residualRows(outs)
	idx := map[string]int{}
	for i, c := range t.cols {
		idx[c] = i
	}
	for _, o := range outs {
		if o.err != nil {
			continue
		}
		curve := o.res.Curve
		m := report.Model{Name: o.model, RMSEbp: o.res.RMSEbp, Used: o.nPrepared,
			Zero: func(x float64) float64 { return toPct(curve.ZeroRate(x)) }}
		for _, row := range t.rows {
			if row[idx["model"]] != o.model {
				continue
			}
			num := func(col string) float64 { v, _ := row[idx[col]].(float64); return v }
			m.Residuals = append(m.Residuals, report.Residual{ID: row[idx["id"]].(string), Maturity: row[idx["maturity"]].(string),
				TTM: num("ttm_years"), MarketYield: num("yield_cc_market_pct"), ModelYield: num("yield_cc_model_pct"), ErrBp: num("error_bp")})
		}
		m.Params = fitParams(o)
		r.Models = append(r.Models, m)
	}
	return r
}

// fitParams lists the fitted parameters in the units of the curve file:
// betas and spline zero rates in percent, decay parameters and knots in
// years.
func fitParams(o fitOutcome) []report.Param {
	p := o.res.Params
	if o.model == term.ModelNSS && len(p) == 6 {
		return []report.Param{{Name: "b0", Value: toPct(p[0]), Unit: "%"}, {Name: "b1", Value: toPct(p[1]), Unit: "%"},
			{Name: "b2", Value: toPct(p[2]), Unit: "%"}, {Name: "b3", Value: toPct(p[3]), Unit: "%"},
			{Name: "t1", Value: p[4], Unit: "years"}, {Name: "t2", Value: p[5], Unit: "years"}}
	}
	var ps []report.Param
	for i, k := range o.res.Knots {
		if i < len(p) {
			ps = append(ps, report.Param{Name: fmt.Sprintf("zero rate at %.4g y", k), Value: toPct(p[i]), Unit: "%"})
		}
	}
	return append(ps, report.Param{Name: "lambda", Value: o.res.Lambda})
}
