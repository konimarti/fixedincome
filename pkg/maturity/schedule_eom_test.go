package maturity_test

import (
	"testing"
	"time"

	"github.com/konimarti/fixedincome/pkg/maturity"
)

// TestSchedule_EndOfMonth checks that the coupon dates are derived from the
// maturity date without drifting at the end of the month
func TestSchedule_EndOfMonth(t *testing.T) {
	testData := []struct {
		Name       string
		Settlement time.Time
		Maturity   time.Time
		Frequency  maturity.Frequency
		EOM        maturity.EOMRule
		// expected coupon dates (ascending), written down by hand
		Dates []time.Time
	}{
		{
			Name:       "31-Aug semiannual",
			Settlement: date(2024, 9, 15),
			Maturity:   date(2026, 8, 31),
			Frequency:  2,
			Dates:      []time.Time{date(2025, 2, 28), date(2025, 8, 31), date(2026, 2, 28), date(2026, 8, 31)},
		},
		{
			Name:       "31-May quarterly",
			Settlement: date(2024, 5, 1),
			Maturity:   date(2025, 5, 31),
			Frequency:  4,
			Dates:      []time.Time{date(2024, 5, 31), date(2024, 8, 31), date(2024, 11, 30), date(2025, 2, 28), date(2025, 5, 31)},
		},
		{
			Name:       "29-Feb (leap year) annual",
			Settlement: date(2025, 1, 15),
			Maturity:   date(2028, 2, 29),
			Frequency:  1,
			Dates:      []time.Time{date(2025, 2, 28), date(2026, 2, 28), date(2027, 2, 28), date(2028, 2, 29)},
		},
		{
			Name:       "28-Feb (end of month) semiannual",
			Settlement: date(2025, 9, 1),
			Maturity:   date(2027, 2, 28),
			Frequency:  2,
			Dates:      []time.Time{date(2026, 2, 28), date(2026, 8, 31), date(2027, 2, 28)},
		},
		{
			Name:       "28-Feb with EOM=false semiannual",
			Settlement: date(2025, 9, 1),
			Maturity:   date(2027, 2, 28),
			Frequency:  2,
			EOM:        maturity.EOMOff,
			Dates:      []time.Time{date(2026, 2, 28), date(2026, 8, 28), date(2027, 2, 28)},
		},
		{
			Name:       "30-Aug (not end of month) semiannual",
			Settlement: date(2025, 1, 15),
			Maturity:   date(2026, 8, 30),
			Frequency:  2,
			Dates:      []time.Time{date(2025, 2, 28), date(2025, 8, 30), date(2026, 2, 28), date(2026, 8, 30)},
		},
		{
			Name:       "30-Apr with EOM=true semiannual",
			Settlement: date(2025, 1, 15),
			Maturity:   date(2026, 4, 30),
			Frequency:  2,
			EOM:        maturity.EOMOn,
			Dates:      []time.Time{date(2025, 4, 30), date(2025, 10, 31), date(2026, 4, 30)},
		},
		{
			Name:       "15-Jan monthly",
			Settlement: date(2025, 10, 1),
			Maturity:   date(2026, 1, 15),
			Frequency:  12,
			Dates:      []time.Time{date(2025, 10, 15), date(2025, 11, 15), date(2025, 12, 15), date(2026, 1, 15)},
		},
		{
			Name:       "settlement on coupon date (ex-coupon)",
			Settlement: date(2025, 8, 31),
			Maturity:   date(2026, 8, 31),
			Frequency:  2,
			Dates:      []time.Time{date(2026, 2, 28), date(2026, 8, 31)},
		},
		{
			Name:       "settlement on maturity date",
			Settlement: date(2026, 8, 31),
			Maturity:   date(2026, 8, 31),
			Frequency:  2,
			Dates:      nil,
		},
	}

	for _, test := range testData {
		s := maturity.Schedule{
			Maturity:  test.Maturity,
			Frequency: test.Frequency,
			DayCount:  maturity.ActAct,
			EOM:       test.EOM,
		}
		dates, err := s.CouponDates(test.Settlement)
		if err != nil {
			t.Fatal(err)
		}
		if !sameDates(dates, test.Dates) {
			t.Errorf("%s: got %v, expected %v", test.Name, dates, test.Dates)
		}
	}
}
