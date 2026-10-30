// Package fit estimates spot-rate term structures (Nelson-Siegel-Svensson
// and cubic splines) from the quoted prices of fixed-coupon bonds.
//
// Conventions: as everywhere in this module, rates and yields are
// continuously compounded decimals (0.01 = 1%) on the ACT/365 Fixed time
// axis from the settlement date; times to maturity are in years. Quoted
// (clean) prices are in PERCENT OF FACE; internally they are converted to
// currency units per bond (CleanPrice*Face/100) to match the cash flows.
// The fit minimises the (weighted) squared differences between the model
// yields and the market yields of the bonds (yield objective); a price
// objective is available for comparison.
//
// Settlement: Options.Settle is the date at which the quoted prices are
// paid and the curve starts (t = 0). No settlement lag is applied: pass the
// settlement date (e.g. trade date + 2 business days), or the trade date if
// the curve should start at the trade date.
//
// Typical use:
//
//	bonds, err := fit.ReadBondsCSV(f)
//	o := fit.Options{Settle: settle}
//	p, err := fit.PrepareBonds(bonds, o)
//	res, err := fit.FitNSS(p, o)
//
// A Prepared bond set is immutable and can be fitted concurrently.
package fit

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/konimarti/fixedincome/pkg/cashflow"
	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/rate"
	"github.com/konimarti/fixedincome/pkg/term"
)

// Bond is a fixed-coupon bond with its quoted (clean) price.
type Bond struct {
	// ID identifies the bond in the results (e.g. the ISIN); optional
	ID string
	// Maturity is the maturity date (redemption at Face)
	Maturity time.Time
	// Coupon is the annual coupon rate (decimal, 0.0125 = 1.25%)
	Coupon float64
	// Frequency is the number of coupons per year; 0 means maturity.Annual
	Frequency maturity.Frequency
	// DayCount is the accrual convention (default maturity.Thirty360E)
	DayCount maturity.DayCount
	// CleanPrice is the quoted price excluding accrued interest in PERCENT
	// OF FACE (e.g. 101.25), independent of Face
	CleanPrice float64
	// Face is the face value in currency units; 0 means 100. It scales
	// the cash flows and accrued interest but not the quoted price.
	Face float64
	// Weight is the weight of the bond's residual for WeightCustom (> 0);
	// ignored otherwise
	Weight float64
}

// WeightScheme selects the weights w_i of the residuals of the fit.
type WeightScheme int

const (
	// WeightUnit gives all bonds the weight 1 (default).
	WeightUnit WeightScheme = iota
	// WeightInvDuration weights each residual with 1/(D_i P_i), the
	// inverse of the Macaulay duration (years) times the dirty price
	// (percent of face) at the market yield, normalised to a mean weight
	// of 1. Since dP = -D P dy, the weighted price residuals of
	// ObjectivePrice then approximate the yield residuals.
	WeightInvDuration
	// WeightCustom uses Bond.Weight (normalised to a mean weight of 1).
	WeightCustom
)

// Objective selects the residuals of the fit.
type Objective int

const (
	// ObjectiveYield minimises the squared differences of the model and
	// market yields (continuously compounded, default).
	ObjectiveYield Objective = iota
	// ObjectivePrice minimises the squared differences of the model and
	// market dirty prices (in percent of face). For comparison only: price
	// errors over-weight long bonds unless combined with
	// WeightInvDuration.
	ObjectivePrice
)

// ParseObjective parses "yield" or "price" (case-insensitive).
func ParseObjective(s string) (Objective, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "yield", "":
		return ObjectiveYield, nil
	case "price":
		return ObjectivePrice, nil
	}
	return 0, fmt.Errorf("fit: unknown objective %q (yield, price)", s)
}

// Options are the settings of PrepareBonds, FitNSS and FitSpline. Only
// Settle is required; the zero values of the other fields select the
// defaults:
//
//	Field          zero value means                    other values
//	Settle         (required)                          settlement date, t = 0
//	MinTTM         DefaultMinTTM (0.1 years)           > 0 minimum, < 0 keep all
//	Weights        WeightUnit                          WeightInvDuration, WeightCustom
//	Objective      ObjectiveYield                      ObjectivePrice
//	OvernightRate  nil: no short-end constraint        cc decimal, see OvernightToCC
//	Start          nil: grid starts only               additional NSS start
//	MaxIter        200 LM iterations per polish run    > 0
//	Ridge          DefaultRidge (1e-5)                 > 0 strength, < 0 off
//	Huber          0: least squares                    > 0 Huber threshold in bp
//	Lambda         nil: generalized cross-validation   *0 none, *x > 0 fixed, *x < 0 GCV
//	Knots          nil: quantile placement             explicit knots (years)
//	NumKnots       n/3 clamped to [5, 8]               > 0 quantile knots
type Options struct {
	// Settle is the settlement date of the prices (curve date, t = 0);
	// required. No settlement lag is applied.
	Settle time.Time
	// MinTTM is the minimum time to maturity in years of the bonds used
	// (shorter ones are dropped with a warning); 0 means DefaultMinTTM, a
	// negative value keeps all bonds that have not matured
	MinTTM float64
	// Weights is the weighting scheme of the residuals
	Weights WeightScheme
	// Objective selects yield (default) or price residuals
	Objective Objective
	// OvernightRate, if set, constrains the short end of the curve:
	// r(0) = *OvernightRate (continuously compounded on the ACT/365F axis,
	// decimal; see OvernightToCC). FitNSS imposes B0 + B1 = *OvernightRate
	// exactly, FitSpline adds a stiff penalty residual.
	OvernightRate *float64
	// Start is an additional starting point of the NSS fit (e.g. the
	// previous day's curve); the multi-start grid is always searched
	Start *term.NSS
	// MaxIter is the maximum number of Levenberg-Marquardt iterations of
	// a polish run; 0 means 200
	MaxIter int
	// Ridge is the strength of the ridge regularisation of B2 and B3 of
	// the NSS fit; 0 means DefaultRidge, negative disables it. The penalty
	// is n*(Ridge*B)² per beta for n bonds, i.e. a beta of 1 (100%) costs
	// as much as a yield error of Ridge per bond.
	Ridge float64
	// Huber, if > 0, replaces the squared loss of the bond residuals by
	// the Huber loss with the threshold Huber (bp of yield error): errors
	// beyond the threshold count linearly, which limits the influence of
	// outliers. It is solved by iteratively reweighted least squares (IRLS
	// weights min(1, Huber/|e_i|) in the Levenberg-Marquardt problem). 0
	// means ordinary least squares.
	Huber float64
	// Lambda is the smoothing parameter of FitSpline (penalty Lambda *
	// integral of the squared second derivative of the zero rate,
	// approximated at the knots; decimal rates and years). nil or a
	// negative value selects Lambda by generalized cross-validation (the
	// default, also of termfit), 0 is an unpenalised least-squares spline,
	// a positive value a fixed smoothing. See LambdaGCV and Lambda.
	Lambda *float64
	// Knots are the spline knots in years (strictly increasing, > 0); if
	// empty, knots are placed as described at FitSpline
	Knots []float64
	// NumKnots is the number of spline knots placed between the shortest
	// and longest maturity (a short knot at half the shortest maturity is
	// added); 0 means n/3 clamped to [5, 8] and at most n-2 for n bonds
	NumKnots int
}

// LambdaGCV is the Options.Lambda value that selects the smoothing
// parameter by generalized cross-validation (equivalent to nil).
const LambdaGCV = -1.0

// Lambda returns a pointer to v for Options.Lambda.
func Lambda(v float64) *float64 { return &v }

// DefaultMinTTM is the default minimum time to maturity (years, about 5
// weeks): money market bills closer to maturity are dominated by
// idiosyncratic effects and distort the short end.
const DefaultMinTTM = 0.1

// DefaultRidge is the default ridge strength of the NSS fit (see
// Options.Ridge): a beta of 100% costs as much as a yield error of 0.1bp
// per bond.
const DefaultRidge = 1e-5

// MaxMarketYield is the magnitude of a market yield (decimal) above which
// PrepareBonds warns of an implausible price.
const MaxMarketYield = 0.25

func (o Options) minTTM() float64 {
	switch {
	case o.MinTTM == 0:
		return DefaultMinTTM
	case o.MinTTM < 0:
		return 0
	}
	return o.MinTTM
}

func (o Options) maxIter() int {
	if o.MaxIter <= 0 {
		return 200
	}
	return o.MaxIter
}

// OvernightToCC converts an overnight rate quoted as a simple ACT/360 rate
// (decimal, e.g. SARON) to the continuously compounded rate on the ACT/365F
// time axis of the curves: one day accrues r/360 and spans 1/365 years, so
// rc = 365 * ln(1 + r/360). Example: -0.75% gives -0.7604%.
func OvernightToCC(rateACT360 float64) float64 {
	return 365 * math.Log1p(rateACT360/360)
}

// prepared is a bond with its cash flows and market data at settlement.
type prepared struct {
	Bond
	cfs              []cashflow.CashFlow
	t, amt, la       []float64 // cash-flow times (years), amounts and log amounts
	ttm              float64
	accrued, dirty   float64 // currency units per bond of Face
	yMkt, yMktAnnual float64 // market yield (cc, decimal) and annual equivalent
	duration         float64 // Macaulay duration at yMkt (years)
	faceFactor       float64 // 100/Face: currency units to percent of face
}

// Prepared is a set of bonds with cash flows, accrued interest and market
// yields computed once at the settlement date, sorted by time to
// maturity. It is immutable and safe for concurrent use.
type Prepared struct {
	settle   time.Time
	bonds    []prepared
	warnings []string
	minTTM   float64
}

// PreparedBond describes a bond of a Prepared set.
type PreparedBond struct {
	Bond
	// TTM is the time to maturity in years (ACT/365F)
	TTM float64
	// Accrued is the accrued interest and DirtyPrice =
	// CleanPrice*Face/100 + Accrued the dirty price, both in currency
	// units per bond of Face
	Accrued, DirtyPrice float64
	// Yield is the market yield (continuously compounded, decimal) and
	// YieldAnnual its annually compounded equivalent
	Yield, YieldAnnual float64
	// Duration is the Macaulay duration at the market yield (years)
	Duration float64
	// CashFlows are the cash flows after the settlement date
	CashFlows []cashflow.CashFlow
}

// Len returns the number of bonds used for fitting.
func (p *Prepared) Len() int { return len(p.bonds) }

// Settle returns the settlement date.
func (p *Prepared) Settle() time.Time { return p.settle }

// Warnings returns the warnings of the preparation (e.g. dropped bonds).
func (p *Prepared) Warnings() []string { return append([]string(nil), p.warnings...) }

// Bonds returns copies of the prepared bonds, sorted by time to maturity.
func (p *Prepared) Bonds() []PreparedBond {
	out := make([]PreparedBond, len(p.bonds))
	for i, b := range p.bonds {
		out[i] = PreparedBond{Bond: b.Bond, TTM: b.ttm, Accrued: b.accrued, DirtyPrice: b.dirty,
			Yield: b.yMkt, YieldAnnual: b.yMktAnnual, Duration: b.duration,
			CashFlows: append([]cashflow.CashFlow(nil), b.cfs...)}
	}
	return out
}

func finiteValue(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// name returns the bond ID or a description for messages.
func (b Bond) name(i int) string {
	if b.ID != "" {
		return b.ID
	}
	return fmt.Sprintf("bond %d (%s %.4g%%)", i+1, b.Maturity.Format(dateLayouts[0]), b.Coupon*100)
}

// PrepareBonds computes the cash flows after o.Settle, the accrued
// interest, dirty price (CleanPrice*Face/100 + accrued, currency units) and
// market yield (continuously compounded) of each bond once. Bonds that have
// matured, mature within o.MinTTM years or have no market yield are dropped
// with a warning (see Prepared.Warnings); a market yield beyond
// ±MaxMarketYield is kept with a warning (e.g. a price quoted as a fraction
// of face instead of percent). Invalid bonds (non-finite or non-positive
// price, invalid frequency or day count, zero maturity date, invalid weight
// for WeightCustom) are an error.
func PrepareBonds(bonds []Bond, o Options) (*Prepared, error) {
	if o.Settle.IsZero() {
		return nil, errors.New("fit: settlement date required")
	}
	if !finiteValue(o.MinTTM) {
		return nil, fmt.Errorf("fit: invalid MinTTM %v", o.MinTTM)
	}
	settle := maturity.Date(o.Settle)
	p := &Prepared{settle: settle, minTTM: o.minTTM()}
	for i, b := range bonds {
		if b.Frequency == 0 {
			b.Frequency = maturity.Annual
		}
		if b.Face == 0 {
			b.Face = bond.DefaultFace
		}
		if b.Maturity.IsZero() {
			return nil, fmt.Errorf("fit: %s: missing maturity date", b.name(i))
		}
		if !finiteValue(b.CleanPrice) || !(b.CleanPrice > 0) {
			return nil, fmt.Errorf("fit: %s: invalid price %v", b.name(i), b.CleanPrice)
		}
		if !finiteValue(b.Coupon) || b.Coupon < 0 {
			return nil, fmt.Errorf("fit: %s: invalid coupon %v", b.name(i), b.Coupon)
		}
		if !finiteValue(b.Face) || !(b.Face > 0) {
			return nil, fmt.Errorf("fit: %s: invalid face value %v", b.name(i), b.Face)
		}
		if o.Weights == WeightCustom && (!finiteValue(b.Weight) || !(b.Weight > 0)) {
			return nil, fmt.Errorf("fit: %s: invalid weight %v", b.name(i), b.Weight)
		}
		if !maturity.Date(b.Maturity).After(settle) {
			p.warnings = append(p.warnings, fmt.Sprintf("%s: matured on %s, dropped", b.name(i), b.Maturity.Format(dateLayouts[0])))
			continue
		}
		ttm := maturity.YearFraction(settle, b.Maturity)
		if ttm < p.minTTM {
			p.warnings = append(p.warnings, fmt.Sprintf("%s: time to maturity %.3fy below the minimum %.3fy, dropped", b.name(i), ttm, p.minTTM))
			continue
		}
		s, err := bond.NewStraight(maturity.Schedule{Maturity: b.Maturity, Frequency: b.Frequency, DayCount: b.DayCount},
			b.Coupon, bond.WithFace(b.Face))
		if err != nil {
			return nil, fmt.Errorf("fit: %s: %w", b.name(i), err)
		}
		cfs, err := s.CashFlows(settle)
		if err != nil {
			return nil, fmt.Errorf("fit: %s: %w", b.name(i), err)
		}
		acc, err := s.Accrued(settle)
		if err != nil {
			return nil, fmt.Errorf("fit: %s: %w", b.name(i), err)
		}
		pb := prepared{Bond: b, cfs: cfs, ttm: ttm, accrued: acc, dirty: b.CleanPrice*b.Face/100 + acc, faceFactor: 100 / b.Face}
		for _, cf := range cfs {
			pb.t = append(pb.t, cf.T)
			pb.amt = append(pb.amt, cf.Amount)
			pb.la = append(pb.la, math.Log(cf.Amount))
		}
		y, err := cashflow.YieldCC(pb.dirty, cfs)
		if err != nil {
			p.warnings = append(p.warnings, fmt.Sprintf("%s: no market yield (%v), dropped", b.name(i), err))
			continue
		}
		if math.Abs(y) > MaxMarketYield {
			msg := fmt.Sprintf("%s: implausible market yield %.2f%% (clean price %v)", b.name(i), y*100, b.CleanPrice)
			if b.CleanPrice < 20 {
				msg += ": prices must be quoted in percent of face, not as a fraction"
			}
			p.warnings = append(p.warnings, msg)
		}
		pb.yMkt = y
		pb.yMktAnnual, _ = rate.FromContinuous(y, 1)
		_, pb.duration = priceDuration(pb.t, pb.amt, y)
		p.bonds = append(p.bonds, pb)
	}
	sort.SliceStable(p.bonds, func(i, j int) bool { return p.bonds[i].ttm < p.bonds[j].ttm })
	if len(p.bonds) == 0 {
		return nil, errors.New("fit: no bonds left to fit")
	}
	return p, nil
}

// weights returns the residual weights of the bonds (mean 1).
func (p *Prepared) weights(s WeightScheme) ([]float64, error) {
	w := make([]float64, len(p.bonds))
	sum := 0.0
	for i, b := range p.bonds {
		switch s {
		case WeightUnit:
			w[i] = 1
		case WeightInvDuration:
			w[i] = 1 / (b.duration * b.dirty * b.faceFactor)
		case WeightCustom:
			w[i] = b.Weight
			if !finiteValue(w[i]) || !(w[i] > 0) {
				return nil, fmt.Errorf("fit: %s: invalid weight %v", b.name(i), w[i])
			}
		default:
			return nil, fmt.Errorf("fit: unknown weight scheme %d", s)
		}
		sum += w[i]
	}
	for i := range w {
		w[i] *= float64(len(w)) / sum
	}
	return w, nil
}

// dateLayouts are the accepted date formats of ParseDate.
var dateLayouts = []string{"2006-01-02", "02.01.2006"}

// ParseDate parses a date in ISO format (YYYY-MM-DD) or DD.MM.YYYY.
func ParseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range dateLayouts {
		if d, err := time.Parse(layout, s); err == nil {
			return d, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date %q (formats: YYYY-MM-DD, DD.MM.YYYY)", s)
}

// ReadBondsCSV reads bonds from comma-separated data with the columns
// maturity (YYYY-MM-DD or DD.MM.YYYY), coupon (annual, in PERCENT), clean
// price (PERCENT of face) and an optional id. The header row is optional;
// if it is present, the columns are identified by name (case-insensitive:
// maturity, coupon, price, id and the optional frequency, daycount, face,
// weight), may appear in any order and must be unique. Without a header
// the bonds are annual 30E/360 bonds with a face value of 100. Rows
// without an id get the id "row<N>" (N is the line number). Empty lines
// and a UTF-8 byte order mark are skipped. Semicolon- or tab-separated
// data is rejected with a hint.
func ReadBondsCSV(r io.Reader) ([]Bond, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("fit: %w", err)
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	cr := csv.NewReader(bytes.NewReader(data))
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	records, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("fit: invalid CSV: %w", err)
	}
	if len(records) > 0 && len(records[0]) == 1 {
		for _, d := range []struct{ sep, name string }{{";", "semicolons"}, {"\t", "tabs"}} {
			if strings.Contains(records[0][0], d.sep) {
				return nil, fmt.Errorf("fit: CSV line 1 is separated by %s; the bond file must be comma-separated", d.name)
			}
		}
	}
	col := map[string]int{"maturity": 0, "coupon": 1, "price": 2, "id": 3}
	start := 0
	if len(records) > 0 && isHeader(records[0]) {
		col = map[string]int{}
		for i, name := range records[0] {
			key := strings.ToLower(strings.TrimSpace(name))
			if _, dup := col[key]; dup {
				return nil, fmt.Errorf("fit: CSV header %v: duplicate column %q", records[0], key)
			}
			col[key] = i
		}
		for _, req := range []string{"maturity", "coupon", "price"} {
			if _, ok := col[req]; !ok {
				return nil, fmt.Errorf("fit: CSV header %v: missing column %q", records[0], req)
			}
		}
		start = 1
	}
	field := func(row []string, name string) (string, bool) {
		i, ok := col[name]
		if !ok || i >= len(row) {
			return "", false
		}
		return strings.TrimSpace(row[i]), true
	}
	var bonds []Bond
	for n := start; n < len(records); n++ {
		row := records[n]
		line := n + 1
		if len(row) == 0 || (len(row) == 1 && strings.TrimSpace(row[0]) == "") {
			continue
		}
		var b Bond
		var ok bool
		var v string
		if v, ok = field(row, "maturity"); !ok {
			return nil, fmt.Errorf("fit: CSV line %d: missing maturity", line)
		}
		if b.Maturity, err = ParseDate(v); err != nil {
			return nil, fmt.Errorf("fit: CSV line %d: %v", line, err)
		}
		if v, ok = field(row, "coupon"); !ok {
			return nil, fmt.Errorf("fit: CSV line %d: missing coupon", line)
		}
		c, err := parseNumber(v)
		if err != nil {
			return nil, fmt.Errorf("fit: CSV line %d: invalid coupon: %v", line, err)
		}
		b.Coupon = c / 100
		if v, ok = field(row, "price"); !ok {
			return nil, fmt.Errorf("fit: CSV line %d: missing price", line)
		}
		if b.CleanPrice, err = parseNumber(v); err != nil {
			return nil, fmt.Errorf("fit: CSV line %d: invalid price: %v", line, err)
		}
		if v, ok = field(row, "id"); ok && v != "" {
			b.ID = v
		} else {
			b.ID = fmt.Sprintf("row%d", line)
		}
		if v, ok = field(row, "frequency"); ok && v != "" {
			if b.Frequency, err = maturity.ParseFrequency(v); err != nil {
				return nil, fmt.Errorf("fit: CSV line %d: %v", line, err)
			}
		}
		if v, ok = field(row, "daycount"); ok && v != "" {
			if b.DayCount, err = maturity.ParseDayCount(v); err != nil {
				return nil, fmt.Errorf("fit: CSV line %d: %v", line, err)
			}
		}
		if v, ok = field(row, "face"); ok && v != "" {
			if b.Face, err = parseNumber(v); err != nil || !(b.Face > 0) {
				return nil, fmt.Errorf("fit: CSV line %d: invalid face %q", line, v)
			}
		}
		if v, ok = field(row, "weight"); ok && v != "" {
			if b.Weight, err = parseNumber(v); err != nil {
				return nil, fmt.Errorf("fit: CSV line %d: invalid weight: %v", line, err)
			}
		}
		bonds = append(bonds, b)
	}
	if len(bonds) == 0 {
		return nil, errors.New("fit: no bonds in CSV data")
	}
	return bonds, nil
}

// parseNumber parses a finite float.
func parseNumber(s string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, err
	}
	if !finiteValue(v) {
		return 0, fmt.Errorf("not finite: %v", s)
	}
	return v, nil
}

// isHeader reports whether the CSV row is a header: the first field is not
// a date and none of the other fields is numeric (a row with at least two
// fields; a header with too few columns is reported as missing a column).
func isHeader(row []string) bool {
	if len(row) < 2 {
		return false
	}
	if _, err := ParseDate(row[0]); err == nil {
		return false
	}
	for _, f := range row[1:] {
		if _, err := strconv.ParseFloat(strings.TrimSpace(f), 64); err == nil {
			return false
		}
	}
	return true
}
