// Package maturity implements coupon schedules, accrual fractions and the
// time axis used for discounting.
//
// Day counts are those of github.com/konimarti/daycount (see DayCount). All
// dates are normalised to UTC midnight of their calendar date (the time of
// day and the location offset are ignored).
package maturity

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrMatured is returned if the settlement date is on or after the
	// maturity date where a current coupon period is required.
	ErrMatured = errors.New("maturity: settlement date is on or after the maturity date")
	// ErrSettlement is returned for an invalid settlement date (zero, or
	// more than 200 years before the maturity date).
	ErrSettlement = errors.New("maturity: invalid settlement date")
)

// maxMonths is the maximum number of months between settlement and maturity.
const maxMonths = 200 * 12

// Frequency is the number of coupon payments per year.
type Frequency int

// Valid coupon frequencies.
const (
	Annual     Frequency = 1
	SemiAnnual Frequency = 2
	Triannual  Frequency = 3
	Quarterly  Frequency = 4
	BiMonthly  Frequency = 6
	Monthly    Frequency = 12
)

// Validate returns an error unless f is one of 1, 2, 3, 4, 6 or 12.
func (f Frequency) Validate() error {
	switch f {
	case Annual, SemiAnnual, Triannual, Quarterly, BiMonthly, Monthly:
		return nil
	}
	return fmt.Errorf("maturity: invalid frequency %d: must be one of 1, 2, 3, 4, 6, 12", int(f))
}

// Months returns the number of months of a regular coupon period (12/f), or
// 0 for an invalid frequency.
func (f Frequency) Months() int {
	if f.Validate() != nil {
		return 0
	}
	return 12 / int(f)
}

// ParseFrequency parses a coupon frequency: a number (1, 2, 3, 4, 6, 12) or
// one of "annual", "semiannual" (or "semi-annual"), "triannual",
// "quarterly", "bimonthly", "monthly" (case-insensitive).
func ParseFrequency(s string) (Frequency, error) {
	n := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), "-", "")
	var f Frequency
	switch n {
	case "annual", "annually", "yearly":
		f = Annual
	case "semiannual", "semiannually":
		f = SemiAnnual
	case "triannual":
		f = Triannual
	case "quarterly":
		f = Quarterly
	case "bimonthly":
		f = BiMonthly
	case "monthly":
		f = Monthly
	default:
		i, err := strconv.Atoi(n)
		if err != nil {
			return 0, fmt.Errorf("maturity: invalid frequency %q", s)
		}
		f = Frequency(i)
	}
	return f, f.Validate()
}

// EOMRule is the end-of-month rule of a schedule.
type EOMRule int

// End-of-month rules.
const (
	// EOMAuto infers the rule: it applies if the maturity date is the last
	// day of its month. Note that this includes 28 February in a non-leap
	// year: a bond maturing on 2027-02-28 then pays on 31 August (semiannual);
	// use EOMOff to pay on the 28th.
	EOMAuto EOMRule = iota
	// EOMOn puts all coupon dates on the last day of their month.
	EOMOn
	// EOMOff keeps the day of the maturity date (or the last day of the
	// month if that day does not exist).
	EOMOff
)

// Validate returns an error for an unknown rule.
func (e EOMRule) Validate() error {
	if e < EOMAuto || e > EOMOff {
		return fmt.Errorf("maturity: invalid end-of-month rule %d", int(e))
	}
	return nil
}

// Date returns t normalised to UTC midnight of its calendar date.
func Date(t time.Time) time.Time {
	if t.Location() == time.UTC && t.Nanosecond() == 0 && t.Unix()%86400 == 0 {
		return t
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// Days returns the number of calendar days from from to to (negative if to
// is before from).
func Days(from, to time.Time) int {
	return int(Date(to).Sub(Date(from)) / (24 * time.Hour))
}

// YearFraction returns the time in years from from to to on the curve time
// axis: ACT/365 Fixed, i.e. the number of calendar days divided by 365
// (negative if to is before from). It is used for all discounting times
// across instruments.
func YearFraction(from, to time.Time) float64 {
	return float64(Days(from, to)) / 365.0
}

// Schedule describes the coupon dates and the day count convention of a
// fixed income security.
//
// Regular coupon dates are generated backwards from the maturity date in
// steps of 12/Frequency months (no drift: every date is derived directly
// from the maturity date). If the day of the maturity date does not exist in
// a month, the last day of that month is used; with the end-of-month rule
// (see EOMRule) all coupon dates are the last day of their month.
//
// The first coupon period is:
//   - without Issue: regular (the periods extend back without limit);
//   - with Issue but without FirstCoupon: from the issue date to the first
//     regular date after it (a short stub, or regular if the issue date is a
//     regular date);
//   - with Issue and FirstCoupon: from the issue date to FirstCoupon, which
//     must be a regular date; this may be a long first period (spanning
//     more than one regular period) or a short one.
type Schedule struct {
	// Issue is the issue date (start of the first accrual period); optional
	Issue time.Time
	// FirstCoupon is the first coupon date; optional, requires Issue
	FirstCoupon time.Time
	// Maturity is the maturity date (last coupon and redemption date)
	Maturity time.Time
	// Frequency is the number of coupons per year (see Frequency.Validate)
	Frequency Frequency
	// DayCount is the day count convention (default Thirty360E)
	DayCount DayCount
	// EOM is the end-of-month rule (default EOMAuto)
	EOM EOMRule
}

// Validate returns an error if the maturity date is zero, the frequency, the
// day count convention or the end-of-month rule is invalid, the issue date is
// not before the maturity date or the first coupon date is not a regular date
// between the issue and the maturity date.
func (s Schedule) Validate() error {
	if s.Maturity.IsZero() {
		return fmt.Errorf("maturity: maturity date is not set")
	}
	if err := s.Frequency.Validate(); err != nil {
		return err
	}
	if err := s.DayCount.Validate(); err != nil {
		return err
	}
	if err := s.EOM.Validate(); err != nil {
		return err
	}
	mat := Date(s.Maturity)
	if !s.Issue.IsZero() && !Date(s.Issue).Before(mat) {
		return fmt.Errorf("maturity: issue date %s is not before maturity date %s",
			s.Issue.Format("2006-01-02"), s.Maturity.Format("2006-01-02"))
	}
	if !s.FirstCoupon.IsZero() {
		if s.Issue.IsZero() {
			return fmt.Errorf("maturity: first coupon date requires an issue date")
		}
		fc := Date(s.FirstCoupon)
		if !fc.After(Date(s.Issue)) || fc.After(mat) {
			return fmt.Errorf("maturity: first coupon date %s is not after the issue date and on or before the maturity date", fc.Format("2006-01-02"))
		}
		g := s.gen()
		m := months(fc, mat)
		if m%g.step != 0 || !g.date(m/g.step).Equal(fc) {
			return fmt.Errorf("maturity: first coupon date %s is not a regular coupon date", fc.Format("2006-01-02"))
		}
	}
	return nil
}

// months returns the number of calendar months from the month of a to the
// month of b.
func months(a, b time.Time) int {
	return (b.Year()-a.Year())*12 + int(b.Month()-a.Month())
}

// gen generates the regular coupon dates of a schedule.
type gen struct {
	y, m, day, step int
	eom             bool
}

func (s Schedule) gen() gen {
	mat := Date(s.Maturity)
	g := gen{y: mat.Year(), m: int(mat.Month()), day: mat.Day(), step: s.Frequency.Months()}
	switch s.EOM {
	case EOMOn:
		g.eom = true
	case EOMAuto:
		g.eom = g.day == daysIn(g.y, g.m)
	}
	return g
}

// date returns the regular coupon date k periods before maturity.
func (g gen) date(k int) time.Time {
	mm := g.m - 1 - k*g.step
	y := g.y + floorDiv(mm, 12)
	m := mm - 12*floorDiv(mm, 12) + 1
	last := daysIn(y, m)
	d := g.day
	if d > last || g.eom {
		d = last
	}
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

var monthDays = [13]int{0, 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}

// daysIn returns the number of days in the given month.
func daysIn(year, month int) int {
	if month == 2 && year%4 == 0 && (year%100 != 0 || year%400 == 0) {
		return 29
	}
	return monthDays[month]
}

// kAfter returns the largest k with g.date(k) strictly after d (-1 if
// d is on or after the maturity date).
func (g gen) kAfter(d time.Time, mat time.Time) int {
	m := months(d, mat)
	if m < 0 {
		return -1
	}
	k := m / g.step
	if !g.date(k).After(d) {
		k--
	}
	return k
}

// kFirst returns the index of the first coupon period, or -1 if the
// periods extend back without limit.
func (s Schedule) kFirst(g gen, mat time.Time) int {
	switch {
	case !s.FirstCoupon.IsZero():
		return months(Date(s.FirstCoupon), mat) / g.step
	case !s.Issue.IsZero():
		return g.kAfter(Date(s.Issue), mat)
	}
	return -1
}

// checkSettle validates the settlement date.
func checkSettle(settle, mat time.Time) error {
	if settle.IsZero() {
		return fmt.Errorf("%w: settlement date is not set", ErrSettlement)
	}
	if months(Date(settle), mat) > maxMonths {
		return fmt.Errorf("%w: settlement date %s is more than 200 years before maturity", ErrSettlement, settle.Format("2006-01-02"))
	}
	return nil
}

// Period is an accrual period of a schedule.
type Period struct {
	// Start is the accrual start date (previous coupon date or issue date)
	Start time.Time
	// End is the accrual end and payment date (coupon date)
	End time.Time
	// RefStart is the regular (possibly notional) coupon date one period
	// before End; RefStart equals Start for a regular period, is before
	// Start for a short and after Start for a long first period.
	RefStart time.Time
}

// Regular returns true if the period is a full regular coupon period.
func (p Period) Regular() bool {
	return p.Start.Equal(p.RefStart)
}

// Periods returns the accrual periods whose payment date is strictly after
// settle, in ascending order (empty if settle is on or after the maturity
// date). A payment on the settlement date is not included (ex-coupon
// semantics). It returns an error for an invalid schedule or settlement
// date.
func (s Schedule) Periods(settle time.Time) ([]Period, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return s.periods(settle)
}

// periods returns the periods of a valid schedule.
func (s Schedule) periods(settle time.Time) ([]Period, error) {
	mat := Date(s.Maturity)
	if err := checkSettle(settle, mat); err != nil {
		return nil, err
	}
	g := s.gen()
	ks, kf := g.kAfter(Date(settle), mat), s.kFirst(g, mat)
	if kf >= 0 && ks > kf {
		ks = kf
	}
	if ks < 0 {
		return nil, nil
	}
	periods := make([]Period, ks+1)
	ref := g.date(ks + 1)
	issue := Date(s.Issue)
	for k := ks; k >= 0; k-- {
		end := g.date(k)
		start := ref
		if k == kf {
			start = issue
		}
		periods[ks-k] = Period{Start: start, End: end, RefStart: ref}
		ref = end
	}
	return periods, nil
}

// PeriodAt returns the accrual period containing settle, i.e. the first
// period whose payment date is strictly after settle (its start is after
// settle if settle is before the issue date). It returns ErrMatured if
// settle is on or after the maturity date.
func (s Schedule) PeriodAt(settle time.Time) (Period, error) {
	if err := s.Validate(); err != nil {
		return Period{}, err
	}
	return s.periodAt(settle)
}

func (s Schedule) periodAt(settle time.Time) (Period, error) {
	mat := Date(s.Maturity)
	if err := checkSettle(settle, mat); err != nil {
		return Period{}, err
	}
	g := s.gen()
	ks, kf := g.kAfter(Date(settle), mat), s.kFirst(g, mat)
	if kf >= 0 && ks > kf {
		ks = kf
	}
	if ks < 0 {
		return Period{}, ErrMatured
	}
	p := Period{End: g.date(ks), RefStart: g.date(ks + 1)}
	p.Start = p.RefStart
	if ks == kf {
		p.Start = Date(s.Issue)
	}
	return p, nil
}

// CouponDates returns the coupon dates strictly after settle up to and
// including the maturity date in ascending order (UTC midnight). A coupon
// paid on the settlement date is not included.
func (s Schedule) CouponDates(settle time.Time) ([]time.Time, error) {
	periods, err := s.Periods(settle)
	if err != nil || len(periods) == 0 {
		return nil, err
	}
	dates := make([]time.Time, len(periods))
	for i, p := range periods {
		dates[i] = p.End
	}
	return dates, nil
}

// PreviousCoupon returns the start of the accrual period containing settle,
// i.e. the latest coupon date on or before settle, or the issue date if it
// is later. It returns ErrMatured if settle is on or after the maturity date.
func (s Schedule) PreviousCoupon(settle time.Time) (time.Time, error) {
	p, err := s.PeriodAt(settle)
	return p.Start, err
}

// NextCoupon returns the first coupon date strictly after settle. It returns
// ErrMatured if settle is on or after the maturity date.
func (s Schedule) NextCoupon(settle time.Time) (time.Time, error) {
	p, err := s.PeriodAt(settle)
	return p.End, err
}

// AccrualFraction returns the year fraction accrued from start (previous
// coupon or issue date) to settle in the coupon period ending at the coupon
// date end, such that the accrued interest is annualCoupon * AccrualFraction:
//
//   - ActAct (ACT/ACT ICMA): sum over the regular (notional) coupon periods
//     overlapping [start, settle] of days in the overlap / days of the
//     notional period / frequency; for a regular period this is
//     days(start, settle) / days(start, end) / frequency, and short and long
//     first periods use the notional periods ending at end;
//   - 30/360 family and Act360: daycount.Days(start, settle) / 360.
//
// For the 30/360 family a full regular period gives 1/frequency (except for
// periods ending at the end of February), whereas Act360 gives the actual
// days / 360.
func (s Schedule) AccrualFraction(start, settle, end time.Time) (float64, error) {
	if err := s.Validate(); err != nil {
		return 0, err
	}
	return s.accrual(start, settle, end), nil
}

// accrual computes the accrual fraction for a valid schedule.
func (s Schedule) accrual(start, settle, end time.Time) float64 {
	start, settle, end = Date(start), Date(settle), Date(end)
	if s.DayCount != ActAct {
		return s.DayCount.days(start, settle) / 360.0
	}
	g := s.gen()
	mat := Date(s.Maturity)
	k := months(end, mat) / g.step
	sum := 0.0
	nEnd := g.date(k)
	for i := 0; i <= maxMonths; i++ {
		nStart := g.date(k + i + 1)
		lo, hi := start, settle
		if nStart.After(lo) {
			lo = nStart
		}
		if nEnd.Before(hi) {
			hi = nEnd
		}
		if hi.After(lo) {
			sum += ActAct.days(lo, hi) / ActAct.days(nStart, nEnd)
		}
		if !nStart.After(start) {
			break
		}
		nEnd = nStart
	}
	return sum / float64(s.Frequency)
}

// CouponFraction returns the year fraction of the full accrual period p by
// which the annual coupon rate is multiplied to obtain the coupon amount of
// a fixed-rate bond:
//
//   - regular period, 30/360 family and ActAct: 1/frequency (standard bond
//     market practice);
//   - Act360 (any period) and irregular (stub) periods:
//     AccrualFraction(p.Start, p.End, p.End), so that the coupon equals the
//     interest accrued over the period (no jump of the clean price on the
//     coupon date).
func (s Schedule) CouponFraction(p Period) (float64, error) {
	if err := s.Validate(); err != nil {
		return 0, err
	}
	return s.couponFraction(p), nil
}

func (s Schedule) couponFraction(p Period) float64 {
	if p.Regular() && s.DayCount != Act360 {
		return 1.0 / float64(s.Frequency)
	}
	return s.accrual(p.Start, p.End, p.End)
}
