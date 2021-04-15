package bonds

import (
	"time"
)

type Maturities struct {
	QuoteDate    time.Time
	MaturityDate time.Time
	Frequency    int // interest payment frequency, default: 1x per year
}

//Frequency
func (m *Maturities) GetFrequency() int {
	freq := 1
	if m.Frequency > 0 {
		freq = m.Frequency
	}
	return freq
}

//Maturities returns a slice of the effective maturities in years of the bond's cash flows
func (m *Maturities) M() []float64 {
	maturities := []float64{}

	step := 12 / m.GetFrequency()

	// walk back from maturity date to quote date
	quote := m.QuoteDate
	for current := m.MaturityDate; current.Sub(quote) > 0; current = current.AddDate(0, -step, 0) {
		maturities = append(maturities, ActualDifferenceInYears(quote, current))
	}

	return maturities
}

//RemainingYears
func (m *Maturities) RemainingYears() float64 {
	if m.MaturityDate.Before(m.QuoteDate) {
		return 0.0
	}
	return ActualDifferenceInYears(m.QuoteDate, m.MaturityDate)
}

// Days since last coupon payment based on European 30/360 method
// Source: https://sqlsunday.com/2014/08/17/30-360-day-count-convention/
func (m *Maturities) DaysSinceLastCouponInYears() float64 {
	if m.MaturityDate.Before(m.QuoteDate) {
		return 0.0
	}
	step := 12 / m.GetFrequency()
	d1 := m.MaturityDate
	d2 := m.QuoteDate
	for ; d1.Sub(d2) > 0; d1 = d1.AddDate(0, -step, 0) {
	}

	// correct the dates according to European 30/360
	if d1.Day() == 31 {
		d1 = d1.AddDate(0, 0, -1)
	}
	if d2.Day() == 31 {
		d2 = d2.AddDate(0, 0, -1)
	}

	// comply with ISDA guideline for February
	d1Day := float64(d1.Day())
	if d1.Month() == 2 && d1Day > 27.0 {
		d1Day = 30.0
	}
	d2Day := float64(d2.Day())
	if d2.Month() == 2 && d2Day > 27.0 {
		d2Day = 30.0
	}

	days := 360.0*float64(d2.Year()-d1.Year()) + 30.0*float64(d2.Month()-d1.Month()) + d2Day - d1Day

	// fmt.Println("d1=", current)
	// fmt.Println("d2=", quote)
	// fmt.Println("days=", days)

	return days / 360.0
}

// helper functions

// Actual difference between two dates in years
func ActualDifferenceInYears(start, stop time.Time) float64 {
	return float64(stop.Sub(start).Hours()) / 24.0 / 365.0
}

// Actual difference between two dates in years
// func ActualDifferenceInYears(start, stop time.Time) float64 {
// 	years := 0.0
// 	// same year, just take difference in days and divided by numbers of days in year
// 	if start.Year() == stop.Year() {
// 		years = float64(stop.YearDay()-start.YearDay()) / DaysInYear(start.Year())
// 	} else {
// 		// "maturity" for current year
// 		years += 1.0 - float64(start.YearDay())/DaysInYear(start.Year())
// 		// "maturity" for last year
// 		years += float64(stop.YearDay()) / DaysInYear(stop.Year())
// 		// "maturity" for years in between
// 		for y := start.Year() + 1; y < stop.Year(); y += 1 {
// 			years += 1.0
// 		}
// 	}
// 	// hour adjustment
// 	years -= float64(start.Hour()) / 24.0 / DaysInYear(start.Year())
// 	years += float64(stop.Hour()) / 24.0 / DaysInYear(stop.Year())
//
// 	return years
// }

// DaysInYear calculates the number of days in the given year
// func DaysInYear(year int) float64 {
// 	return float64(time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC).YearDay())
// }

// LastDay of a given year
// func LastDay(year int) time.Time {
// 	return time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC)
// }
// FirstDay of a given year
// func FirstDay(year int) time.Time {
// 	return time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
// }
