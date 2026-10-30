package maturity_test

import (
	"math"
	"testing"
	"time"

	"github.com/konimarti/fixedincome/pkg/maturity"
)

func date(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}

// actYears returns the actual number of days between settlement and d divided
// by the number of days in the year after settlement (as with ACTACT)
func actYears(settlement, d time.Time) float64 {
	return d.Sub(settlement).Hours() / settlement.AddDate(1, 0, 0).Sub(settlement).Hours()
}

// TestSchedule_EndOfMonth checks that the coupon dates are derived from the
// maturity date without drifting at the end of the month
func TestSchedule_EndOfMonth(t *testing.T) {
	testData := []struct {
		Name       string
		Settlement time.Time
		Maturity   time.Time
		Frequency  int
		// expected coupon dates (descending), written down by hand
		Dates []time.Time
	}{
		{
			Name:       "31-Aug semiannual",
			Settlement: date(2024, 9, 15),
			Maturity:   date(2026, 8, 31),
			Frequency:  2,
			Dates:      []time.Time{date(2026, 8, 31), date(2026, 2, 28), date(2025, 8, 31), date(2025, 2, 28)},
		},
		{
			Name:       "31-May quarterly",
			Settlement: date(2024, 5, 1),
			Maturity:   date(2025, 5, 31),
			Frequency:  4,
			Dates:      []time.Time{date(2025, 5, 31), date(2025, 2, 28), date(2024, 11, 30), date(2024, 8, 31), date(2024, 5, 31)},
		},
		{
			Name:       "29-Feb (leap year) annual",
			Settlement: date(2025, 1, 15),
			Maturity:   date(2028, 2, 29),
			Frequency:  1,
			Dates:      []time.Time{date(2028, 2, 29), date(2027, 2, 28), date(2026, 2, 28), date(2025, 2, 28)},
		},
		{
			Name:       "28-Feb (end of month) semiannual",
			Settlement: date(2025, 9, 1),
			Maturity:   date(2027, 2, 28),
			Frequency:  2,
			Dates:      []time.Time{date(2027, 2, 28), date(2026, 8, 31), date(2026, 2, 28)},
		},
		{
			Name:       "30-Aug (not end of month) semiannual",
			Settlement: date(2025, 1, 15),
			Maturity:   date(2026, 8, 30),
			Frequency:  2,
			Dates:      []time.Time{date(2026, 8, 30), date(2026, 2, 28), date(2025, 8, 30), date(2025, 2, 28)},
		},
		{
			Name:       "15-Jan monthly",
			Settlement: date(2025, 10, 1),
			Maturity:   date(2026, 1, 15),
			Frequency:  12,
			Dates:      []time.Time{date(2026, 1, 15), date(2025, 12, 15), date(2025, 11, 15), date(2025, 10, 15)},
		},
	}

	for _, test := range testData {
		s := maturity.Schedule{
			Settlement: test.Settlement,
			Maturity:   test.Maturity,
			Frequency:  test.Frequency,
			Basis:      "ACTACT",
		}
		m := s.M()
		if len(m) != len(test.Dates) {
			t.Errorf("%s: wrong number of cash flows; got %d (%v), expected %d", test.Name, len(m), m, len(test.Dates))
			continue
		}
		for i, d := range test.Dates {
			expected := actYears(test.Settlement, d)
			if math.Abs(m[i]-expected) > 1e-9 {
				t.Errorf("%s: maturity %d; got %v, expected %v (%s)", test.Name, i, m[i], expected, d.Format("2006-01-02"))
			}
		}
	}
}

// TestSchedule_DayCountFractionEndOfMonth checks the accrued fraction for end
// of month coupon dates
func TestSchedule_DayCountFractionEndOfMonth(t *testing.T) {
	testData := []struct {
		Name       string
		Settlement time.Time
		Maturity   time.Time
		Frequency  int
		Basis      string
		Expected   float64
	}{
		{
			// previous coupon 2024-08-31, next coupon 2025-02-28
			// 30E360: 15 days of 180 days in the period, divided by 2
			Name:       "31-Aug semiannual 30E360",
			Settlement: date(2024, 9, 15),
			Maturity:   date(2026, 8, 31),
			Frequency:  2,
			Basis:      "30E360",
			Expected:   15.0 / 180.0 / 2.0,
		},
		{
			// previous coupon 2024-11-30, next coupon 2025-02-28
			// ACTACT: 15 days of 90 days in the period, divided by 4
			Name:       "31-May quarterly ACTACT",
			Settlement: date(2024, 12, 15),
			Maturity:   date(2025, 5, 31),
			Frequency:  4,
			Basis:      "ACTACT",
			Expected:   15.0 / 90.0 / 4.0,
		},
	}
	for _, test := range testData {
		s := maturity.Schedule{
			Settlement: test.Settlement,
			Maturity:   test.Maturity,
			Frequency:  test.Frequency,
			Basis:      test.Basis,
		}
		if got := s.DayCountFraction(); math.Abs(got-test.Expected) > 1e-9 {
			t.Errorf("%s: got %v, expected %v", test.Name, got, test.Expected)
		}
	}
}

// TestSchedule_InvalidFrequency checks that frequencies which do not divide
// 12 months evenly are rejected
func TestSchedule_InvalidFrequency(t *testing.T) {
	for _, freq := range []int{5, 7, 8, 9, 10, 11, 13, 24} {
		s := maturity.Schedule{
			Settlement: date(2021, 1, 1),
			Maturity:   date(2025, 1, 1),
			Frequency:  freq,
		}
		for name, fn := range map[string]func(){
			"M":                func() { s.M() },
			"DayCountFraction": func() { s.DayCountFraction() },
		} {
			func() {
				defer func() {
					if recover() == nil {
						t.Errorf("frequency %d: expected %s to panic", freq, name)
					}
				}()
				fn()
			}()
		}
	}
	for _, freq := range []int{0, 1, 2, 3, 4, 6, 12} {
		s := maturity.Schedule{
			Settlement: date(2021, 1, 1),
			Maturity:   date(2025, 1, 1),
			Frequency:  freq,
		}
		n := s.Compounding()
		if got := len(s.M()); got != 4*n {
			t.Errorf("frequency %d: got %d cash flows, expected %d", freq, got, 4*n)
		}
	}
}
