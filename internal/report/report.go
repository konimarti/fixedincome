// Package report renders the result of a curve fit as one self-contained
// HTML file: inline CSS and inline SVG plots, no scripts and no external
// assets. The output is deterministic (no timestamps), so it can be
// compared byte for byte. All text is escaped; bond ids, warnings and file
// names are untrusted.
//
// The package only formats; it takes values in the units of the command
// line (rates and yields in percent, errors in basis points).
package report

import (
	"fmt"
	"html/template"
	"io"
	"math"
	"strconv"
	"strings"
)

// Param is a fitted parameter of a model.
type Param struct {
	Name  string
	Value float64
	Unit  string
}

// Residual is the fit result of one bond (yields in percent, continuously
// compounded; error in bp).
type Residual struct {
	ID       string
	Maturity string
	// TTM is the time to maturity in years
	TTM         float64
	MarketYield float64
	ModelYield  float64
	ErrBp       float64
}

// Model is a fitted curve.
type Model struct {
	// Name is the model name, e.g. "nss"
	Name string
	// Zero is the continuously compounded zero rate in percent at t years
	Zero      func(t float64) float64
	RMSEbp    float64
	Used      int
	Params    []Param
	Residuals []Residual
}

// Report is the content of a fit report.
type Report struct {
	// AsOf is the curve date (settlement), Source the bond file
	AsOf, Source string
	// Bonds is the number of valid bonds read
	Bonds    int
	Models   []Model
	Warnings []string
}

// Write renders the report to w.
func Write(w io.Writer, r Report) error {
	if len(r.Models) == 0 {
		return fmt.Errorf("report: no model")
	}
	v := view{Report: r}
	xmax := 0.0
	for _, m := range r.Models {
		for _, x := range m.Residuals {
			if x.TTM > xmax && !isBad(x.TTM) {
				xmax = x.TTM
			}
		}
	}
	if !(xmax > 0) {
		xmax = 1
	}
	v.PlotA = template.HTML(plotCurves(r.Models, xmax))
	v.PlotB = template.HTML(plotResiduals(r.Models, xmax))
	for i, m := range r.Models {
		mv := modelView{Name: m.Name, Index: i, RMSE: f3(m.RMSEbp), Used: m.Used}
		for _, p := range m.Params {
			mv.Params = append(mv.Params, paramView{p.Name, strconv.FormatFloat(p.Value, 'g', 8, 64), p.Unit})
		}
		v.Models = append(v.Models, mv)
		for _, x := range m.Residuals {
			v.Rows = append(v.Rows, rowView{Model: m.Name, ID: x.ID, Maturity: x.Maturity, TTM: f4(x.TTM),
				Market: f4(x.MarketYield), ModelY: f4(x.ModelYield), Err: f3(x.ErrBp)})
		}
	}
	v.Multi = len(r.Models) > 1
	return page.Execute(w, v)
}

type view struct {
	Report
	Models       []modelView
	Rows         []rowView
	PlotA, PlotB template.HTML
	Multi        bool
}

type modelView struct {
	Name   string
	Index  int
	RMSE   string
	Used   int
	Params []paramView
}

type paramView struct{ Name, Value, Unit string }

type rowView struct{ Model, ID, Maturity, TTM, Market, ModelY, Err string }

func isBad(v float64) bool { return math.IsNaN(v) || math.IsInf(v, 0) }

// fmtN formats a number with d decimals; non-finite values give "n/a" and
// negative zero is printed as 0.
func fmtN(v float64, d int) string {
	if isBad(v) {
		return "n/a"
	}
	s := strconv.FormatFloat(v, 'f', d, 64)
	if strings.Trim(s, "-0.") == "" {
		s = strings.TrimPrefix(s, "-")
	}
	return s
}

func f3(v float64) string { return fmtN(v, 3) }
func f4(v float64) string { return fmtN(v, 4) }

var page = template.Must(template.New("report").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Curve fit report {{.AsOf}}</title>
<style>
:root{color-scheme:light dark;--bg:#fff;--fg:#1b1f24;--muted:#57606a;--grid:#d8dee4;--card:#f6f8fa;--line:#d0d7de;
--c0:#0072B2;--c1:#D55E00;--warn-bg:#fff4d6;--warn-fg:#5c4000;--warn-line:#c99a00}
@media (prefers-color-scheme:dark){:root{--bg:#14171a;--fg:#e6e9ec;--muted:#9aa4af;--grid:#2e353c;--card:#1c2025;--line:#30363d;
--c0:#56B4E9;--c1:#E69F00;--warn-bg:#2d2500;--warn-fg:#f2d780;--warn-line:#9a7500}}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--fg);font:15px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
main{max-width:60rem;margin:0 auto;padding:1.5rem 1rem 3rem}
h1{font-size:1.5rem;margin:0 0 .25rem}
h2{font-size:1.1rem;margin:2rem 0 .5rem}
p.sub{margin:0;color:var(--muted)}
.cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(10rem,1fr));gap:.75rem;margin:1.25rem 0}
.card{background:var(--card);border:1px solid var(--line);border-radius:6px;padding:.6rem .8rem}
.card .k{font-size:.8rem;color:var(--muted)}
.card .v{font-size:1.25rem;font-variant-numeric:tabular-nums}
.warn{background:var(--warn-bg);color:var(--warn-fg);border:1px solid var(--warn-line);border-radius:6px;padding:.5rem 1rem .5rem 2rem}
.warn h2{margin:0 0 .25rem -1rem;font-size:1rem}
.warn ul{margin:0;padding-left:.5rem}
figure{margin:0}
figcaption{font-size:.85rem;color:var(--muted);margin-top:.25rem}
svg{width:100%;height:auto;display:block;background:var(--bg)}
svg text{fill:var(--muted);font:11px system-ui,sans-serif}
svg .grid{stroke:var(--grid);stroke-width:1}
svg .axis{stroke:var(--muted);stroke-width:1}
svg .zero{stroke:var(--fg);stroke-width:1.2}
svg .m0{--c:var(--c0)} svg .m1{--c:var(--c1)}
svg .ln{fill:none;stroke:var(--c);stroke-width:2}
svg .ln.d{stroke-dasharray:6 3}
svg .pt{fill:var(--c);fill-opacity:.8;stroke:var(--bg);stroke-width:1}
svg .obs{fill:none;stroke:var(--muted);stroke-width:1.4}
svg text.lbl{fill:var(--fg)}
.tw{overflow-x:auto}
table{border-collapse:collapse;width:100%;font-variant-numeric:tabular-nums}
caption{text-align:left;font-weight:600;padding:.25rem 0}
th,td{padding:.25rem .6rem;border-bottom:1px solid var(--line);text-align:right;white-space:nowrap}
th{background:var(--card);font-weight:600}
th:first-child,td:first-child,td.l,th.l{text-align:left}
tbody tr:hover{background:var(--card)}
.sw{display:inline-block;width:.7rem;height:.7rem;border-radius:50%;margin-right:.35rem;vertical-align:baseline}
.s0{background:var(--c0)} .s1{background:var(--c1);border-radius:0;transform:rotate(45deg) scale(.85)}
</style>
</head>
<body>
<main>
<h1>Curve fit report</h1>
<p class="sub">As of {{.AsOf}}{{if .Source}} &middot; bonds: {{.Source}}{{end}}</p>
<div class="cards">
<div class="card"><div class="k">As-of date</div><div class="v">{{.AsOf}}</div></div>
<div class="card"><div class="k">Valid bonds</div><div class="v">{{.Bonds}}</div></div>
{{range .Models}}<div class="card"><div class="k"><span class="sw s{{.Index}}"></span>{{.Name}}: yield error RMSE</div><div class="v">{{.RMSE}} bp</div><div class="k">{{.Used}} bonds used</div></div>
{{end}}</div>
{{if .Warnings}}<div class="warn" role="note"><h2>Fit warnings</h2>
<ul>
{{range .Warnings}}<li>{{.}}</li>
{{end}}</ul></div>
{{else}}<p class="sub">No fit warnings.</p>
{{end}}
<h2>Zero curve</h2>
<figure>
{{.PlotA}}
<figcaption>Fitted zero rates (continuously compounded, percent) by maturity. Open circles are the yields of the bonds at their market prices.</figcaption>
</figure>
<h2>Yield errors</h2>
<figure>
{{.PlotB}}
<figcaption>Model yield minus market yield per bond in basis points by time to maturity.</figcaption>
</figure>
<h2>Parameters</h2>
{{range .Models}}<div class="tw"><table>
<caption>{{.Name}}</caption>
<thead><tr><th scope="col">Parameter</th><th scope="col">Value</th><th scope="col">Unit</th></tr></thead>
<tbody>
{{range .Params}}<tr><td>{{.Name}}</td><td>{{.Value}}</td><td class="l">{{.Unit}}</td></tr>
{{end}}</tbody>
</table></div>
{{end}}
<h2>Residuals</h2>
<div class="tw"><table>
<caption>Per-bond residuals (yields continuously compounded, percent)</caption>
<thead><tr>{{if .Multi}}<th scope="col">Model</th>{{end}}<th scope="col">Bond</th><th scope="col">Maturity</th><th scope="col">Years to maturity</th><th scope="col">Market yield %</th><th scope="col">Model yield %</th><th scope="col">Error bp</th></tr></thead>
<tbody>
{{range .Rows}}<tr>{{if $.Multi}}<td class="l">{{.Model}}</td>{{end}}<td class="l">{{.ID}}</td><td>{{.Maturity}}</td><td>{{.TTM}}</td><td>{{.Market}}</td><td>{{.ModelY}}</td><td>{{.Err}}</td></tr>
{{end}}</tbody>
</table></div>
</main>
</body>
</html>
`))
