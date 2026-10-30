package maturity_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/konimarti/fixedincome/pkg/maturity"
)

func date(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}

// annual returns the dates on the given day and month for the years from..to
func annual(from, to, m, d int) []time.Time {
	var dates []time.Time
	for y := from; y <= to; y++ {
		dates = append(dates, date(y, m, d))
	}
	return dates
}

func sameDates(a, b []time.Time) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Equal(b[i]) {
			return false
		}
	}
	return true
}

func TestSchedule(t *testing.T) {
	semi := []time.Time{}
	for y := 2021; y <= 2028; y++ {
		semi = append(semi, date(y, 7, 1), date(y+1, 1, 1))
	}

	testData := []struct {
		Quote     time.Time
		Maturity  time.Time
		Frequency maturity.Frequency
		Expected  []time.Time
		// expected calendar days to maturity (computed independently)
		ExpectedDays int
	}{
		{date(2021, 1, 1), date(2029, 1, 1), maturity.Annual, annual(2022, 2029, 1, 1), 2922},
		{date(2021, 1, 1), date(2029, 1, 1), maturity.SemiAnnual, semi, 2922},
		{date(2021, 1, 1), date(2024, 7, 1), maturity.Annual, annual(2021, 2024, 7, 1), 1277},
		{date(2021, 1, 1), date(2019, 1, 1), maturity.Annual, nil, 0},
		{date(2021, 4, 16), date(2029, 10, 16), maturity.Annual, annual(2021, 2029, 10, 16), 3105},
	}

	for nr, test := range testData {
		s := maturity.Schedule{Maturity: test.Maturity, Frequency: test.Frequency}
		dates, err := s.CouponDates(test.Quote)
		if err != nil {
			t.Fatal(err)
		}
		if !sameDates(dates, test.Expected) {
			t.Errorf("test nr %d: got %v, expected %v", nr, dates, test.Expected)
		}
		if test.ExpectedDays > 0 {
			if got := maturity.YearFraction(test.Quote, test.Maturity); math.Abs(got-float64(test.ExpectedDays)/365.0) > 1e-15 {
				t.Errorf("test nr %d: years to maturity; got %v, expected %v", nr, got, float64(test.ExpectedDays)/365.0)
			}
		}
	}
}

// TestSchedule_AccrualFraction checks the accrued year fraction (accrued
// interest = annual coupon * fraction) against hand-computed values
func TestSchedule_AccrualFraction(t *testing.T) {
	testData := []struct {
		Name      string
		Quote     time.Time
		Maturity  time.Time
		Frequency maturity.Frequency
		DayCount  maturity.DayCount
		Expected  float64
	}{
		// previous coupon 2021-05-25, 30E/360: 322 days
		{"annual 30E360", date(2021, 4, 17), date(2022, 5, 25), maturity.Annual, maturity.Thirty360E, 322.0 / 360.0},
		// previous coupon 2015-04-15, 30E/360: 61 days
		{"semiannual 30E360", date(2015, 6, 16), date(2018, 4, 15), maturity.SemiAnnual, maturity.Thirty360E, 61.0 / 360.0},
		// previous coupon 2024-08-31 (30), settle 2024-09-15: 15 days
		{"31-Aug semiannual 30E360", date(2024, 9, 15), date(2026, 8, 31), maturity.SemiAnnual, maturity.Thirty360E, 15.0 / 360.0},
		// previous coupon 2024-11-30, next 2025-02-28: ACT/ACT ICMA 15 of 90 days, quarterly
		{"31-May quarterly ACTACT", date(2024, 12, 15), date(2025, 5, 31), maturity.Quarterly, maturity.ActAct, 15.0 / 90.0 / 4.0},
		// ACT/360: previous coupon 2021-01-15, 46 actual days
		{"ACT360", date(2021, 3, 2), date(2025, 7, 15), maturity.SemiAnnual, maturity.Act360, 46.0 / 360.0},
		// BONDBASIS (30/360): previous coupon 2007-02-28, settle 2007-03-31:
		// D1=28, D2=31 (D1 < 30) -> 33 days (ISDA 30/360 example)
		{"BONDBASIS Feb end", date(2007, 3, 31), date(2009, 8, 28), maturity.SemiAnnual, maturity.BondBasis, 33.0 / 360.0},
		// EUROBOND (30E/360): D1=28, D2=31->30: 32 days
		{"EUROBOND Feb end", date(2007, 3, 31), date(2009, 8, 28), maturity.SemiAnnual, maturity.Eurobond, 32.0 / 360.0},
		// settlement on a coupon date: nothing accrued
		{"on coupon date", date(2021, 5, 25), date(2022, 5, 25), maturity.Annual, maturity.Thirty360E, 0.0},
	}
	for _, test := range testData {
		s := maturity.Schedule{Maturity: test.Maturity, Frequency: test.Frequency, DayCount: test.DayCount}
		p, err := s.PeriodAt(test.Quote)
		if err != nil {
			t.Fatal(err)
		}
		prev, _ := s.PreviousCoupon(test.Quote)
		next, _ := s.NextCoupon(test.Quote)
		if !prev.Equal(p.Start) || !next.Equal(p.End) {
			t.Errorf("%s: PreviousCoupon/NextCoupon inconsistent with PeriodAt", test.Name)
		}
		got, err := s.AccrualFraction(p.Start, test.Quote, p.End)
		if err != nil || math.Abs(got-test.Expected) > 1e-12 {
			t.Errorf("%s: got %v (%v), expected %v (prev %s, next %s)", test.Name, got, err, test.Expected, prev.Format("2006-01-02"), next.Format("2006-01-02"))
		}
	}
}

func TestSchedule_Validate(t *testing.T) {
	for _, freq := range []maturity.Frequency{-1, 0, 5, 7, 8, 9, 10, 11, 13, 24} {
		s := maturity.Schedule{Maturity: date(2025, 1, 1), Frequency: freq}
		if err := s.Validate(); err == nil {
			t.Errorf("frequency %d: expected error", freq)
		}
		if _, err := s.CouponDates(date(2021, 1, 1)); err == nil {
			t.Errorf("frequency %d: expected error from CouponDates", freq)
		}
		if _, err := s.AccrualFraction(date(2021, 1, 1), date(2021, 2, 1), date(2022, 1, 1)); err == nil {
			t.Errorf("frequency %d: expected error from AccrualFraction", freq)
		}
		if _, err := s.CouponFraction(maturity.Period{}); err == nil {
			t.Errorf("frequency %d: expected error from CouponFraction", freq)
		}
		if freq.Months() != 0 {
			t.Errorf("frequency %d: Months() = %d, expected 0", freq, freq.Months())
		}
	}
	for _, freq := range []maturity.Frequency{1, 2, 3, 4, 6, 12} {
		s := maturity.Schedule{Maturity: date(2025, 1, 1), Frequency: freq}
		dates, err := s.CouponDates(date(2021, 1, 1))
		if err != nil {
			t.Errorf("frequency %d: %v", freq, err)
		}
		if got := len(dates); got != 4*int(freq) {
			t.Errorf("frequency %d: got %d cash flows, expected %d", freq, got, 4*int(freq))
		}
	}
	invalid := []maturity.Schedule{
		{Frequency: 1},
		{Maturity: date(2025, 1, 1), Frequency: 1, DayCount: 99},
		{Maturity: date(2025, 1, 1), Frequency: 1, EOM: 7},
		{Maturity: date(2025, 1, 1), Frequency: 1, Issue: date(2025, 1, 1)},
		{Maturity: date(2025, 1, 1), Frequency: 1, FirstCoupon: date(2022, 1, 1)},                          // no issue
		{Maturity: date(2025, 1, 1), Frequency: 1, Issue: date(2021, 3, 1), FirstCoupon: date(2022, 2, 1)}, // not regular
		{Maturity: date(2025, 1, 1), Frequency: 1, Issue: date(2022, 3, 1), FirstCoupon: date(2022, 1, 1)}, // before issue
	}
	for i, s := range invalid {
		if err := s.Validate(); err == nil {
			t.Errorf("invalid schedule %d: expected error", i)
		}
	}
}

// TestSchedule_Settlement checks that invalid settlement dates are rejected
func TestSchedule_Settlement(t *testing.T) {
	s := maturity.Schedule{Maturity: date(2025, 1, 1), Frequency: 2}
	for _, settle := range []time.Time{{}, date(1824, 12, 1)} {
		if _, err := s.Periods(settle); !errors.Is(err, maturity.ErrSettlement) {
			t.Errorf("settle %v: expected ErrSettlement, got %v", settle, err)
		}
		if _, err := s.PeriodAt(settle); !errors.Is(err, maturity.ErrSettlement) {
			t.Errorf("settle %v: PeriodAt expected ErrSettlement, got %v", settle, err)
		}
	}
	if _, err := s.PeriodAt(date(2025, 1, 1)); !errors.Is(err, maturity.ErrMatured) {
		t.Errorf("expected ErrMatured, got %v", err)
	}
	if p, err := s.Periods(date(2025, 1, 1)); err != nil || len(p) != 0 {
		t.Errorf("matured: got %v, %v", p, err)
	}
	// 200 years is fine
	if p, err := s.Periods(date(1825, 1, 1)); err != nil || len(p) != 400 {
		t.Errorf("200 years: got %d periods, %v", len(p), err)
	}
}

// TestSchedule_PeriodAt checks PeriodAt against the first element of Periods
func TestSchedule_PeriodAt(t *testing.T) {
	schedules := []maturity.Schedule{
		{Maturity: date(2026, 8, 31), Frequency: 2},
		{Maturity: date(2026, 8, 30), Frequency: 4},
		{Maturity: date(2028, 2, 29), Frequency: 12, EOM: maturity.EOMOff},
		{Issue: date(2021, 1, 15), Maturity: date(2024, 6, 1), Frequency: 1},
		{Issue: date(2020, 1, 15), FirstCoupon: date(2021, 6, 1), Maturity: date(2024, 6, 1), Frequency: 1},
	}
	for i, s := range schedules {
		for settle := date(2019, 12, 1); settle.Before(s.Maturity); settle = settle.AddDate(0, 0, 3) {
			ps, err := s.Periods(settle)
			if err != nil {
				t.Fatal(err)
			}
			p, err := s.PeriodAt(settle)
			if err != nil || p != ps[0] {
				t.Fatalf("schedule %d settle %s: PeriodAt %+v, Periods[0] %+v (%v)", i, settle.Format("2006-01-02"), p, ps[0], err)
			}
			for j := 1; j < len(ps); j++ {
				if !ps[j].Start.Equal(ps[j-1].End) || !ps[j].Regular() {
					t.Fatalf("schedule %d: periods not contiguous/regular: %+v", i, ps)
				}
			}
		}
	}
}

func TestSchedule_Issue(t *testing.T) {
	// short first coupon period from the issue date to 2021-06-01
	s := maturity.Schedule{
		Issue:     date(2021, 1, 15),
		Maturity:  date(2024, 6, 1),
		Frequency: maturity.Annual,
		DayCount:  maturity.ActAct,
	}
	periods, err := s.Periods(date(2020, 12, 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(periods) != 4 {
		t.Fatalf("got %d periods, expected 4", len(periods))
	}
	first := periods[0]
	if !first.Start.Equal(date(2021, 1, 15)) || !first.End.Equal(date(2021, 6, 1)) || !first.RefStart.Equal(date(2020, 6, 1)) || first.Regular() {
		t.Errorf("wrong first period %+v", first)
	}
	if !periods[1].Regular() {
		t.Errorf("second period is not regular")
	}
	// ACT/ACT ICMA stub: 137 days of a notional period of 365 days
	if got, _ := s.CouponFraction(first); math.Abs(got-137.0/365.0) > 1e-15 {
		t.Errorf("stub coupon fraction; got %v, expected %v", got, 137.0/365.0)
	}
	if got, _ := s.CouponFraction(periods[1]); got != 1.0 {
		t.Errorf("regular coupon fraction; got %v", got)
	}
	// accrual in the stub starts at the issue date
	if prev, _ := s.PreviousCoupon(date(2021, 3, 1)); !prev.Equal(date(2021, 1, 15)) {
		t.Errorf("previous coupon in stub: got %v", prev)
	}
	if got, _ := s.AccrualFraction(date(2021, 1, 15), date(2021, 3, 1), date(2021, 6, 1)); math.Abs(got-45.0/365.0) > 1e-15 {
		t.Errorf("stub accrual: got %v, expected %v", got, 45.0/365.0)
	}
	// 30E/360 stub: 136 days / 360
	s.DayCount = maturity.Thirty360E
	if got, _ := s.CouponFraction(first); math.Abs(got-136.0/360.0) > 1e-15 {
		t.Errorf("30E360 stub coupon fraction; got %v, expected %v", got, 136.0/360.0)
	}
	// issue on a regular date: no stub
	s.Issue = date(2021, 6, 1)
	if p, _ := s.Periods(date(2021, 1, 1)); len(p) != 3 || !p[0].Regular() || !p[0].Start.Equal(date(2021, 6, 1)) {
		t.Errorf("issue on regular date: %+v", p)
	}
}

// TestSchedule_LongFirstCoupon checks a long first coupon period and its
// ACT/ACT ICMA accrual over the two notional periods
func TestSchedule_LongFirstCoupon(t *testing.T) {
	// issue 2020-01-15, first coupon 2021-06-01, then annual to 2024-06-01;
	// notional periods 2019-06-01..2020-06-01 (366 days) and
	// 2020-06-01..2021-06-01 (365 days)
	s := maturity.Schedule{
		Issue:       date(2020, 1, 15),
		FirstCoupon: date(2021, 6, 1),
		Maturity:    date(2024, 6, 1),
		Frequency:   maturity.Annual,
		DayCount:    maturity.ActAct,
	}
	periods, err := s.Periods(date(2020, 1, 15))
	if err != nil {
		t.Fatal(err)
	}
	if len(periods) != 4 {
		t.Fatalf("got %d periods: %+v", len(periods), periods)
	}
	first := periods[0]
	if !first.Start.Equal(date(2020, 1, 15)) || !first.End.Equal(date(2021, 6, 1)) || !first.RefStart.Equal(date(2020, 6, 1)) || first.Regular() {
		t.Errorf("wrong long first period %+v", first)
	}
	// 2020-01-15 .. 2020-06-01: 138 days of 366; full second notional period
	expected := 138.0/366.0 + 1.0
	if got, _ := s.CouponFraction(first); math.Abs(got-expected) > 1e-15 {
		t.Errorf("long stub coupon fraction %v, expected %v", got, expected)
	}
	// accrued at 2020-04-01: 77 days of 366
	if got, _ := s.AccrualFraction(first.Start, date(2020, 4, 1), first.End); math.Abs(got-77.0/366.0) > 1e-15 {
		t.Errorf("long stub accrual (first notional) %v, expected %v", got, 77.0/366.0)
	}
	// accrued at 2020-09-01: 138/366 + 92/365
	if got, _ := s.AccrualFraction(first.Start, date(2020, 9, 1), first.End); math.Abs(got-(138.0/366.0+92.0/365.0)) > 1e-15 {
		t.Errorf("long stub accrual (second notional) %v, expected %v", got, 138.0/366.0+92.0/365.0)
	}
	// short first coupon via FirstCoupon equals the implicit short stub
	short := s
	short.FirstCoupon = date(2020, 6, 1)
	implicit := s
	implicit.FirstCoupon = time.Time{}
	a, _ := short.Periods(date(2020, 1, 15))
	b, _ := implicit.Periods(date(2020, 1, 15))
	if len(a) != len(b) || a[0] != b[0] {
		t.Errorf("short first coupon %+v != implicit stub %+v", a[0], b[0])
	}
}

func TestYearFraction_Timezones(t *testing.T) {
	zone := time.FixedZone("UTC+2", 2*3600)
	from := time.Date(2021, 3, 27, 23, 30, 0, 0, zone)
	to := time.Date(2021, 3, 29, 0, 15, 0, 0, time.UTC)
	if got := maturity.YearFraction(from, to); got != 2.0/365.0 {
		t.Errorf("got %v, expected 2/365", got)
	}
	if got := maturity.YearFraction(to, from); got != -2.0/365.0 {
		t.Errorf("got %v, expected -2/365", got)
	}
	if d := maturity.Date(from); !d.Equal(date(2021, 3, 27)) {
		t.Errorf("Date: got %v", d)
	}
}

func TestParse(t *testing.T) {
	for name, expected := range map[string]maturity.DayCount{
		"": maturity.Thirty360E, "30E360": maturity.Thirty360E, "30e/360": maturity.Thirty360E,
		"Eurobond": maturity.Eurobond, "30E/360 ICMA": maturity.Eurobond,
		"BONDBASIS": maturity.BondBasis, "30/360": maturity.BondBasis,
		"ACT360": maturity.Act360, "act/360": maturity.Act360, "Actual/360": maturity.Act360,
		"ACTACT": maturity.ActAct, "ACT/ACT ICMA": maturity.ActAct,
	} {
		got, err := maturity.ParseDayCount(name)
		if err != nil || got != expected {
			t.Errorf("ParseDayCount(%q) = %v, %v; expected %v", name, got, err, expected)
		}
	}
	for _, name := range []string{"ACT365", "ACT/ACT ISDA", "foo"} {
		if _, err := maturity.ParseDayCount(name); err == nil {
			t.Errorf("ParseDayCount(%q): expected error", name)
		}
	}
	for _, d := range []maturity.DayCount{maturity.Thirty360E, maturity.Eurobond, maturity.BondBasis, maturity.Act360, maturity.ActAct} {
		if back, err := maturity.ParseDayCount(d.String()); err != nil || back != d {
			t.Errorf("round trip %v", d)
		}
	}
	for s, expected := range map[string]maturity.Frequency{"1": 1, "2": 2, "12": 12, "Annual": 1, "semi-annual": 2, "QUARTERLY": 4, "monthly": 12} {
		got, err := maturity.ParseFrequency(s)
		if err != nil || got != expected {
			t.Errorf("ParseFrequency(%q) = %v, %v", s, got, err)
		}
	}
	for _, s := range []string{"0", "5", "weekly", ""} {
		if _, err := maturity.ParseFrequency(s); err == nil {
			t.Errorf("ParseFrequency(%q): expected error", s)
		}
	}
}

func BenchmarkPeriods(b *testing.B) {
	s := maturity.Schedule{Maturity: date(2051, 5, 28), Frequency: maturity.SemiAnnual}
	settle := date(2021, 4, 1)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := s.Periods(settle); err != nil {
			b.Fatal(err)
		}
	}
}
