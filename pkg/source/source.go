// Package source fetches and parses the Nelson-Siegel-Svensson (NSS) yield
// curve parameters that central banks publish: the Swiss National Bank
// (cube "rendopar" of data.snb.ch, CHF risk-free spot rates of the
// Confederation bonds) and the European Central Bank (euro area yield curve,
// data-api.ecb.europa.eu).
//
// This is the only package of the library that talks to the network (the
// Client); the parsers ParseSNB and ParseECB work on any io.Reader, so that
// downloaded files can be used offline.
//
// The bond reference data of SIX Swiss Exchange (SIXQuery, ParseSIX,
// Client.FetchSIX; prices are delayed closing prices) is fetched by the same
// Client; its terms of use are at
// https://www.six-group.com/en/products-services/the-swiss-stock-exchange/market-data/news-tools/terms-conditions.html
// (not verified; check them before redistributing the data).
//
// Both banks publish the betas in percent and the decay parameters tau1,
// tau2 in years. The returned Curve has the library convention (betas as
// decimals, 0.01 = 1%), the conversion happens in this package only (see
// Curve).
//
// Terms of use of the data: see the SNB data portal https://data.snb.ch and
// the ECB statistics usage policy
// https://www.ecb.europa.eu/stats/ecb_statistics/governance_and_quality_framework/html/usage_policy.en.html
// and check them before redistributing the data.
package source

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/konimarti/fixedincome/pkg/term"
)

// Curve is a fetched NSS curve with its metadata.
type Curve struct {
	// NSS is the validated curve; the betas are decimals (the published
	// 1.68 percent is 0.0168), Tau1 and Tau2 are in years. term.Marshal
	// writes the percent of the curve file schema again.
	NSS term.NSS
	// Meta has the as-of date (the date of the parameters), the currency
	// ("CHF", "EUR") and the source ("SNB rendopar", "ECB AAA", ...).
	Meta term.Meta
	// Notes are observations about the data that a caller should show as
	// warnings: a newer date that was skipped because it is incomplete, and
	// implausibly large parameter values.
	Notes []string
}

// DateError is the error for a requested date without a complete parameter
// set.
type DateError struct {
	// Label is "SNB" or "ECB", Date the requested date, Why "no data" or
	// "missing b3, t2", Earlier the nearest earlier complete date (empty:
	// none in the data).
	Label, Date, Why, Earlier string
}

func (e *DateError) Error() string {
	hint := "no earlier complete date in the data"
	if e.Earlier != "" {
		hint = "the nearest earlier complete date is " + e.Earlier
	}
	return fmt.Sprintf("%s: no complete parameter set for %s (%s); %s", e.Label, e.Date, e.Why, hint)
}

// parameter codes in the order of the NSS parameters b0, b1, b2, b3, t1, t2
var paramNames = [6]string{"b0", "b1", "b2", "b3", "t1", "t2"}

// paramSet collects the published parameters (in percent / years) of a date.
type paramSet struct {
	v    [6]float64
	have [6]bool
}

func (p *paramSet) complete() bool {
	for _, h := range p.have {
		if !h {
			return false
		}
	}
	return true
}

func (p *paramSet) missing() string {
	var m []string
	for i, h := range p.have {
		if !h {
			m = append(m, paramNames[i])
		}
	}
	return strings.Join(m, ", ")
}

// collection maps ISO dates to their parameters.
type collection map[string]*paramSet

// add stores the value of parameter index i at date; a second value for the
// same (date, parameter) is an error.
func (c collection) add(date string, i int, v float64, line int) error {
	p := c[date]
	if p == nil {
		p = &paramSet{}
		c[date] = p
	}
	if p.have[i] {
		return fmt.Errorf("line %d: duplicate value for %s on %s", line, paramNames[i], date)
	}
	p.v[i], p.have[i] = v, true
	return nil
}

// pick selects the parameters of asof (zero: the latest date with all six
// parameters) and builds the validated curve (percent to decimals).
func (c collection) pick(asof time.Time, label, currency, src string) (Curve, error) {
	var date string
	if asof.IsZero() {
		for d, p := range c {
			if p.complete() && d > date {
				date = d
			}
		}
		if date == "" {
			return Curve{}, fmt.Errorf("%s: no date with all six parameters b0, b1, b2, b3, t1, t2", label)
		}
	} else {
		date = asof.Format(term.DateFormat)
		p := c[date]
		if p == nil || !p.complete() {
			earlier := ""
			for d, q := range c {
				if q.complete() && d < date && d > earlier {
					earlier = d
				}
			}
			why := "no data"
			if p != nil {
				why = "missing " + p.missing()
			}
			return Curve{}, &DateError{Label: label, Date: date, Why: why, Earlier: earlier}
		}
	}
	p := c[date]
	n, err := term.NewNSS(p.v[0]/100, p.v[1]/100, p.v[2]/100, p.v[3]/100, p.v[4], p.v[5])
	if err != nil {
		return Curve{}, fmt.Errorf("%s: parameters of %s: %w", label, date, err)
	}
	d, err := time.Parse(term.DateFormat, date)
	if err != nil {
		return Curve{}, fmt.Errorf("%s: %w", label, err)
	}
	cv := Curve{NSS: n, Meta: term.Meta{AsOf: d, Currency: currency, Source: src}}
	if err := plausible(label, date, p, &cv); err != nil {
		return Curve{}, err
	}
	if asof.IsZero() {
		skipped := ""
		for dd, q := range c {
			if !q.complete() && dd > date && dd > skipped {
				skipped = dd
			}
		}
		if skipped != "" {
			cv.Notes = append(cv.Notes, fmt.Sprintf("skipped newer incomplete date %s (missing %s)", skipped, c[skipped].missing()))
		}
	}
	return cv, nil
}

// plausible rejects absurd parameters (|beta| > 1000 percent, tau > 1000 years)
// and notes suspicious ones (|beta| > 100 percent, tau > 100 years).
func plausible(label, date string, p *paramSet, cv *Curve) error {
	for i, v := range p.v {
		switch {
		case i < 4 && math.Abs(v) > 1000, i >= 4 && v > 1000:
			return fmt.Errorf("%s: parameter %s = %g on %s is implausible (betas are in percent, taus in years)", label, paramNames[i], v, date)
		case i < 4 && math.Abs(v) > 100, i >= 4 && v > 100:
			cv.Notes = append(cv.Notes, fmt.Sprintf("parameter %s = %g on %s is unusually large", paramNames[i], v, date))
		}
	}
	return nil
}

// maxLine is the longest accepted line of a data file.
const maxLine = 1 << 20

// splitFields splits a line at sep; fields may be enclosed in double quotes
// (a doubled quote is a quote). Line breaks inside quotes are not supported.
func splitFields(line string, sep byte) []string {
	var out []string
	i := 0
	for {
		var sb strings.Builder
		if i < len(line) && line[i] == '"' {
			i++
			for i < len(line) {
				if line[i] == '"' {
					if i+1 < len(line) && line[i+1] == '"' {
						sb.WriteByte('"')
						i += 2
						continue
					}
					i++
					break
				}
				sb.WriteByte(line[i])
				i++
			}
			// tolerate junk between the closing quote and the separator
			for i < len(line) && line[i] != sep {
				sb.WriteByte(line[i])
				i++
			}
		} else {
			for i < len(line) && line[i] != sep {
				sb.WriteByte(line[i])
				i++
			}
		}
		out = append(out, sb.String())
		if i >= len(line) {
			return out
		}
		i++ // separator
	}
}

// numberRE is plain decimal syntax: no hex floats, underscores, Inf or NaN.
var numberRE = regexp.MustCompile(`^[+-]?([0-9]+\.?[0-9]*|\.[0-9]+)([eE][+-]?[0-9]+)?$`)

// parseValue parses a finite number in plain decimal syntax.
func parseValue(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if !numberRE.MatchString(s) {
		return 0, fmt.Errorf("invalid number %q", clip(s))
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("number %q is not finite", clip(s))
	}
	return v, nil
}

// parseISODate parses YYYY-MM-DD (a trailing time is ignored).
func parseISODate(s string) (string, error) {
	s = strings.TrimSpace(s)
	if len(s) > 10 && (s[10] == ' ' || s[10] == 'T') {
		s = s[:10]
	}
	if _, err := time.Parse(term.DateFormat, s); err != nil {
		return "", fmt.Errorf("invalid date %q", clip(s))
	}
	return s, nil
}

// clip shortens s for error messages.
func clip(s string) string {
	if len(s) > 40 {
		return s[:40] + "..."
	}
	return s
}
