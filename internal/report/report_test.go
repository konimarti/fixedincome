package report

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

func TestNiceTicks(t *testing.T) {
	ticks, from, to, dec := niceTicks(-0.83, 0.17)
	if from > -0.83 || to < 0.17 || dec != 1 || len(ticks) < 4 {
		t.Errorf("ticks %v [%v, %v] dec %d", ticks, from, to, dec)
	}
	// a degenerate range still gives a usable axis
	ticks, from, to, _ = niceTicks(2, 2)
	if !(to > from) || len(ticks) < 2 {
		t.Errorf("degenerate: %v [%v, %v]", ticks, from, to)
	}
}

func TestWriteEscapesAndSurvivesNaN(t *testing.T) {
	m := Model{Name: "nss", Zero: func(x float64) float64 { return math.NaN() }, Used: 1,
		Residuals: []Residual{{ID: "<b>&", Maturity: "2030-01-01", TTM: 2, MarketYield: 1, ModelYield: 1.1, ErrBp: math.NaN()}},
		Params:    []Param{{Name: "b0", Value: 1, Unit: "%"}}}
	var buf bytes.Buffer
	if err := Write(&buf, Report{AsOf: "2021-01-01", Source: "<f>", Bonds: 1, Models: []Model{m}, Warnings: []string{"<i>w</i>"}}); err != nil {
		t.Fatal(err)
	}
	h := buf.String()
	if strings.Contains(h, "<b>&") || strings.Contains(h, "<i>") || strings.Contains(h, "<f>") || strings.Contains(h, "NaN") {
		t.Error("unescaped text or NaN in the report")
	}
	if err := Write(&buf, Report{}); err == nil {
		t.Error("report without a model")
	}
}
