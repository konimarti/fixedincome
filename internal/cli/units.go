package cli

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/konimarti/fixedincome/pkg/fit"
	"github.com/konimarti/fixedincome/pkg/maturity"
)

// This file contains all unit conversions of the command line boundary:
// percent and basis points (command line, CSV and JSON files) to decimals
// (library) and back, and the parsing of dates and tenors.

// fromPct converts a percent value (2.5) to a decimal (0.025).
func fromPct(x float64) float64 { return x / 100 }

// toPct converts a decimal (0.025) to percent (2.5).
func toPct(x float64) float64 { return x * 100 }

// fromBP converts basis points (50) to a decimal (0.005).
func fromBP(x float64) float64 { return x / 1e4 }

// toBP converts a decimal (0.005) to basis points (50).
func toBP(x float64) float64 { return x * 1e4 }

// dateLayout is the ISO date format of all outputs.
const dateLayout = "2006-01-02"

// formatDate formats a date as YYYY-MM-DD.
func formatDate(t time.Time) string { return t.Format(dateLayout) }

// parseDate parses YYYY-MM-DD or DD.MM.YYYY.
func parseDate(s string) (time.Time, error) {
	return fit.ParseDate(s)
}

// clock returns the current time (replaced in tests).
var clock = time.Now

// today returns today's date (UTC midnight of the local calendar date).
func today() time.Time {
	n := clock()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

var tenorRE = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?|\.[0-9]+)([dwmyDWMY])$`)

// tenor is a period such as 6m or 2y.
type tenor struct {
	n    float64
	unit byte // d, w, m, y
}

// parseTenor parses a tenor: a number followed by d (days), w (weeks), m
// (months) or y (years).
func parseTenor(s string) (tenor, error) {
	m := tenorRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return tenor{}, fmt.Errorf("invalid tenor %q (examples: 6m, 2y, 10d)", s)
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil || n <= 0 || math.IsInf(n, 0) {
		return tenor{}, fmt.Errorf("invalid tenor %q", s)
	}
	return tenor{n: n, unit: strings.ToLower(m[2])[0]}, nil
}

// years returns the length of the tenor in years on the ACT/365F axis
// (6m = 0.5, 1w = 7/365).
func (t tenor) years() float64 {
	switch t.unit {
	case 'y':
		return t.n
	case 'm':
		return t.n / 12
	case 'w':
		return 7 * t.n / 365
	}
	return t.n / 365
}

// addTo returns the date t after the tenor (month-end clamped); the number
// of months and years must be integers.
func (t tenor) addTo(d time.Time) (time.Time, error) {
	if t.n != math.Trunc(t.n) || t.n > 1e4 {
		return time.Time{}, fmt.Errorf("tenor %g%c must be a whole number", t.n, t.unit)
	}
	n := int(t.n)
	switch t.unit {
	case 'd':
		return d.AddDate(0, 0, n), nil
	case 'w':
		return d.AddDate(0, 0, 7*n), nil
	case 'm':
		return addMonths(d, n), nil
	}
	return addMonths(d, 12*n), nil
}

// addMonths adds n months, clamping the day to the end of the target month.
func addMonths(d time.Time, n int) time.Time {
	y, m, day := d.Date()
	first := time.Date(y, m+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if day > last {
		day = last
	}
	return time.Date(first.Year(), first.Month(), day, 0, 0, 0, 0, time.UTC)
}

// parseYears parses a time in years: a tenor (6m, 2y) or a plain number of
// years (2.5).
func parseYears(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if t, err := parseTenor(s); err == nil {
		return t.years(), nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("invalid time %q (examples: 6m, 2y, 2.5)", s)
	}
	return v, nil
}

// parseExpiry parses an expiry: a date (YYYY-MM-DD, DD.MM.YYYY), a tenor or
// a plain number of years; dates are converted to years from settle on the
// ACT/365F axis.
func parseExpiry(s string, settle time.Time) (float64, error) {
	if d, err := parseDate(s); err == nil {
		return maturity.YearFraction(settle, d), nil
	}
	return parseYears(s)
}

// parseMaturityDate parses a maturity: a date or a tenor from settle.
func parseMaturityDate(s string, settle time.Time) (time.Time, error) {
	if d, err := parseDate(s); err == nil {
		return d, nil
	}
	if t, err := parseTenor(s); err == nil {
		return t.addTo(settle)
	}
	return time.Time{}, fmt.Errorf("invalid maturity %q (YYYY-MM-DD, DD.MM.YYYY or a tenor such as 5y)", s)
}

// maxGridPoints limits the size of a --grid.
const maxGridPoints = 200000

// parseGrid parses "start:end:step" (tenors or years) into times in years.
func parseGrid(s string) ([]float64, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid grid %q (expected start:end:step, e.g. 1m:30y:1m)", s)
	}
	var v [3]float64
	for i, p := range parts {
		x, err := parseYears(p)
		if err != nil {
			return nil, fmt.Errorf("invalid grid %q: %v", s, err)
		}
		v[i] = x
	}
	start, end, step := v[0], v[1], v[2]
	if start < 0 || !(step > 0) || end < start {
		return nil, fmt.Errorf("invalid grid %q: need 0 <= start <= end and step > 0", s)
	}
	n := math.Floor((end-start)/step + 1e-9)
	if n+1 > maxGridPoints {
		return nil, fmt.Errorf("grid %q has more than %d points", s, maxGridPoints)
	}
	ts := make([]float64, int(n)+1)
	for i := range ts {
		ts[i] = start + float64(i)*step
	}
	return ts, nil
}

// parseTimes parses a comma-separated list of times (tenors or years).
func parseTimes(s string) ([]float64, error) {
	var ts []float64
	for _, p := range strings.Split(s, ",") {
		t, err := parseYears(p)
		if err != nil {
			return nil, err
		}
		if t < 0 {
			return nil, fmt.Errorf("negative time %q", p)
		}
		ts = append(ts, t)
	}
	return ts, nil
}
