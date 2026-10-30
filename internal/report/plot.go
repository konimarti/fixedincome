package report

import (
	"fmt"
	"html"
	"math"
	"strings"
)

// Plot geometry (SVG user units; the SVG scales to the page width).
const (
	plotW, plotH     = 720.0, 340.0
	marginL, marginR = 56.0, 16.0
	marginT, marginB = 30.0, 44.0
	curvePoints      = 120
	markerR          = 3.5
	tickCount        = 6
	minSpan          = 1e-9
	legendX, legendY = 56.0, 14.0
	legendDX         = 130.0
)

// axis maps data values in [lo, hi] to the pixel range [a, b].
type axis struct {
	lo, hi float64
	a, b   float64
}

func (x axis) at(v float64) float64 { return x.a + (v-x.lo)/(x.hi-x.lo)*(x.b-x.a) }

// frame is a plot area with a linear x and y axis, the ticks and the
// decimals of their labels.
type frame struct {
	x, y   axis
	xt, yt []float64
	xd, yd int
}

// niceStep returns a step of 1, 2 or 5 times a power of ten close to raw.
func niceStep(raw float64) float64 {
	p := math.Pow(10, math.Floor(math.Log10(raw)))
	switch f := raw / p; {
	case f <= 1:
		return p
	case f <= 2:
		return 2 * p
	case f <= 5:
		return 5 * p
	}
	return 10 * p
}

// niceTicks returns ticks that cover [lo, hi], the extended range and the
// decimals needed to print the ticks.
func niceTicks(lo, hi float64) (ticks []float64, from, to float64, dec int) {
	if hi-lo < minSpan {
		lo, hi = lo-1, hi+1
	}
	step := niceStep((hi - lo) / (tickCount - 1))
	from, to = math.Floor(lo/step+1e-9)*step, math.Ceil(hi/step-1e-9)*step
	for v := from; v <= to+step*1e-6; v += step {
		ticks = append(ticks, math.Round(v/step)*step)
	}
	if step < 1 {
		dec = int(math.Ceil(-math.Log10(step) - 1e-9))
	}
	return ticks, from, to, dec
}

// newFrame returns the frame for x in [0, xmax] and y covering [ymin, ymax].
func newFrame(xmax, ymin, ymax float64) frame {
	var f frame
	// the x axis ends just after the last maturity, the ticks lie inside
	xstep := niceStep(xmax / (tickCount - 1))
	for v := 0.0; v <= xmax+xstep*1e-6; v += xstep {
		f.xt = append(f.xt, math.Round(v/xstep)*xstep)
	}
	if xstep < 1 {
		f.xd = int(math.Ceil(-math.Log10(xstep) - 1e-9))
	}
	f.x.lo, f.x.hi = 0, xmax*1.03
	f.yt, f.y.lo, f.y.hi, f.yd = niceTicks(ymin, ymax)
	f.x.a, f.x.b = marginL, plotW-marginR
	f.y.a, f.y.b = plotH-marginB, marginT
	return f
}

// c formats a coordinate.
func c(v float64) string { return fmtN(v, 2) }

// svgOpen starts an SVG with an accessible title and description.
func svgOpen(sb *strings.Builder, id, title, desc string) {
	fmt.Fprintf(sb, `<svg viewBox="0 0 %g %g" role="img" aria-labelledby="%s-t %s-d" xmlns="http://www.w3.org/2000/svg">`+"\n",
		plotW, plotH, id, id)
	fmt.Fprintf(sb, "<title id=\"%s-t\">%s</title>\n<desc id=\"%s-d\">%s</desc>\n", id, html.EscapeString(title), id, html.EscapeString(desc))
}

// draw draws the grid, the axes, the tick labels and the axis titles.
func (f frame) draw(sb *strings.Builder, xTitle, yTitle string) {
	for _, v := range f.yt {
		y := f.y.at(v)
		fmt.Fprintf(sb, `<line class="grid" x1="%s" x2="%s" y1="%s" y2="%s"/>`+"\n", c(f.x.a), c(f.x.b), c(y), c(y))
		fmt.Fprintf(sb, `<text x="%s" y="%s" text-anchor="end" dominant-baseline="middle">%s</text>`+"\n", c(f.x.a-6), c(y), fmtN(v, f.yd))
	}
	for _, v := range f.xt {
		x := f.x.at(v)
		fmt.Fprintf(sb, `<line class="grid" x1="%s" x2="%s" y1="%s" y2="%s"/>`+"\n", c(x), c(x), c(f.y.a), c(f.y.b))
		fmt.Fprintf(sb, `<text x="%s" y="%s" text-anchor="middle">%s</text>`+"\n", c(x), c(f.y.a+16), fmtN(v, f.xd))
	}
	fmt.Fprintf(sb, `<line class="axis" x1="%s" x2="%s" y1="%s" y2="%s"/>`+"\n", c(f.x.a), c(f.x.b), c(f.y.a), c(f.y.a))
	fmt.Fprintf(sb, `<line class="axis" x1="%s" x2="%s" y1="%s" y2="%s"/>`+"\n", c(f.x.a), c(f.x.a), c(f.y.a), c(f.y.b))
	fmt.Fprintf(sb, `<text x="%s" y="%s" text-anchor="middle">%s</text>`+"\n", c((f.x.a+f.x.b)/2), c(plotH-6), html.EscapeString(xTitle))
	mid := c((f.y.a + f.y.b) / 2)
	fmt.Fprintf(sb, `<text x="12" y="%s" text-anchor="middle" transform="rotate(-90 12 %s)">%s</text>`+"\n", mid, mid, html.EscapeString(yTitle))
}

// marker draws the marker of model i (circle or diamond) at (x, y) with a
// tooltip; shape and colour distinguish the models.
func marker(sb *strings.Builder, i int, x, y float64, tip string) {
	if i%2 == 0 {
		fmt.Fprintf(sb, `<circle class="pt" cx="%s" cy="%s" r="%g"><title>%s</title></circle>`+"\n", c(x), c(y), markerR, html.EscapeString(tip))
		return
	}
	r := markerR + 1
	fmt.Fprintf(sb, `<path class="pt" d="M%s %sl%s %sl%s %sl%s %sz"><title>%s</title></path>`+"\n",
		c(x), c(y-r), c(r), c(r), c(-r), c(r), c(-r), c(-r), html.EscapeString(tip))
}

// legendItem draws the legend entry of model i with a line or a marker.
func legendItem(sb *strings.Builder, i int, name string, line bool) {
	x := legendX + float64(i)*legendDX
	fmt.Fprintf(sb, `<g class="m%d">`, i%2)
	if line {
		d := ""
		if i%2 == 1 {
			d = " d"
		}
		fmt.Fprintf(sb, `<line class="ln%s" x1="%s" x2="%s" y1="%s" y2="%s"/>`, d, c(x), c(x+22), c(legendY), c(legendY))
	} else {
		marker(sb, i, x+11, legendY, name)
	}
	fmt.Fprintf(sb, `<text class="lbl" x="%s" y="%s" dominant-baseline="middle">%s</text></g>`+"\n", c(x+28), c(legendY), html.EscapeString(name))
}

// rangeOf returns the range of the finite values.
func rangeOf(vals []float64) (lo, hi float64, ok bool) {
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, v := range vals {
		if isBad(v) {
			continue
		}
		lo, hi = math.Min(lo, v), math.Max(hi, v)
		ok = true
	}
	return
}

// plotCurves draws the zero curves of the models over [0, xmax] and the
// market yields of the bonds.
func plotCurves(models []Model, xmax float64) string {
	type pt struct{ x, y float64 }
	curves := make([][]pt, len(models))
	var ys []float64
	for i, m := range models {
		if m.Zero == nil {
			continue
		}
		for k := 0; k <= curvePoints; k++ {
			x := xmax * float64(k) / curvePoints
			if y := m.Zero(x); !isBad(y) {
				curves[i] = append(curves[i], pt{x, y})
				ys = append(ys, y)
			}
		}
	}
	// the market yields of the bonds are the same for all models
	var obs []Residual
	for _, m := range models {
		if len(m.Residuals) > len(obs) {
			obs = m.Residuals
		}
	}
	for _, r := range obs {
		ys = append(ys, r.MarketYield)
	}
	lo, hi, ok := rangeOf(ys)
	if !ok {
		lo, hi = -1, 1
	}
	pad := (hi - lo) * 0.05
	f := newFrame(xmax, lo-pad, hi+pad)

	var sb strings.Builder
	var names []string
	for _, m := range models {
		names = append(names, m.Name)
	}
	svgOpen(&sb, "plot-zero", "Fitted zero curves", fmt.Sprintf("Zero rates in percent of the model %s from 0 to %s years with the market yields of the bonds.",
		strings.Join(names, " and "), fmtN(xmax, 2)))
	f.draw(&sb, "years to maturity", "zero rate, cc (%)")
	for i, m := range models {
		d := ""
		if i%2 == 1 {
			d = " d"
		}
		var path strings.Builder
		for k, p := range curves[i] {
			cmd := "L"
			if k == 0 {
				cmd = "M"
			}
			fmt.Fprintf(&path, "%s%s %s", cmd, c(f.x.at(p.x)), c(f.y.at(p.y)))
		}
		if path.Len() > 0 {
			fmt.Fprintf(&sb, `<g class="m%d"><path class="ln%s" fill="none" d="%s"><title>%s</title></path></g>`+"\n", i%2, d, path.String(), html.EscapeString(m.Name))
		}
	}
	for _, r := range obs {
		if isBad(r.TTM) || isBad(r.MarketYield) {
			continue
		}
		fmt.Fprintf(&sb, `<circle class="obs" fill="none" cx="%s" cy="%s" r="%g"><title>%s</title></circle>`+"\n", c(f.x.at(r.TTM)), c(f.y.at(r.MarketYield)), markerR,
			html.EscapeString(fmt.Sprintf("%s: %s y, market yield %s %%", r.ID, fmtN(r.TTM, 2), fmtN(r.MarketYield, 3))))
	}
	for i, m := range models {
		legendItem(&sb, i, m.Name, true)
	}
	x := legendX + float64(len(models))*legendDX
	fmt.Fprintf(&sb, `<circle class="obs" fill="none" cx="%s" cy="%s" r="%g"/><text class="lbl" x="%s" y="%s" dominant-baseline="middle">market yield</text>`+"\n",
		c(x+11), c(legendY), markerR, c(x+28), c(legendY))
	sb.WriteString("</svg>")
	return sb.String()
}

// plotResiduals draws the yield errors in bp against the time to maturity.
func plotResiduals(models []Model, xmax float64) string {
	es := []float64{0}
	for _, m := range models {
		for _, r := range m.Residuals {
			es = append(es, r.ErrBp)
		}
	}
	lo, hi, _ := rangeOf(es)
	pad := (hi - lo) * 0.08
	f := newFrame(xmax, lo-pad, hi+pad)

	var sb strings.Builder
	svgOpen(&sb, "plot-err", "Yield errors per bond", "Model yield minus market yield in basis points against the years to maturity of each bond.")
	f.draw(&sb, "years to maturity", "yield error (bp)")
	y0 := c(f.y.at(0))
	fmt.Fprintf(&sb, `<line class="zero" x1="%s" x2="%s" y1="%s" y2="%s"/>`+"\n", c(f.x.a), c(f.x.b), y0, y0)
	for i, m := range models {
		fmt.Fprintf(&sb, `<g class="m%d">`+"\n", i%2)
		for _, r := range m.Residuals {
			if isBad(r.TTM) || isBad(r.ErrBp) {
				continue
			}
			marker(&sb, i, f.x.at(r.TTM), f.y.at(r.ErrBp),
				fmt.Sprintf("%s (%s): %s y, error %s bp", r.ID, m.Name, fmtN(r.TTM, 2), fmtN(r.ErrBp, 3)))
		}
		sb.WriteString("</g>\n")
	}
	for i, m := range models {
		legendItem(&sb, i, m.Name, false)
	}
	sb.WriteString("</svg>")
	return sb.String()
}
