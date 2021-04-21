package bonds

import (
	"time"

	"github.com/konimarti/daycount"
)

// Maturities contain the information about the term maturities of the bond's cash flows
type Maturities struct {
	// Settlement represent the date of valuation (or settlement)
	Settlement time.Time
	// Maturity represents the maturity date
	Maturity time.Time
	// Frequency is the compounding frequency per year (default: 1x per year)
	Frequency int
	// Basis represents the day count convention (default: "" for 30E/360 ISDA)
	Basis string
}

//Compounding returns the annual compounding frequency
func (m *Maturities) Compounding() int {
	n := 1
	if m.Frequency > 0 {
		n = m.Frequency
	}
	return n
}

//M returns a slice of the effective maturities in years of the bond's cash flows
func (m *Maturities) M() []float64 {
	maturities := []float64{}

	step := 12 / m.Compounding()

	// walk back from maturity date to quote date
	quote := m.Settlement
	for current := m.Maturity; current.Sub(quote) > 0; current = current.AddDate(0, -step, 0) {
		maturities = append(maturities, ActualDifferenceInYears(quote, current))
	}

	return maturities
}

//YearsToMaturity calculates the time in years to redemption
func (m *Maturities) YearsToMaturity() float64 {
	if m.Maturity.Before(m.Settlement) {
		return 0.0
	}
	return ActualDifferenceInYears(m.Settlement, m.Maturity)
}

// DayCountFraction returns year fraction since last coupon
func (m *Maturities) DayCountFraction() float64 {
	if m.Maturity.Before(m.Settlement) {
		return 0.0
	}
	step := 12 / m.Compounding()
	d1 := m.Maturity
	d2 := m.Settlement
	d3 := time.Time{}

	// iterate maturity date backwards until last coupon date before settlement date
	for ; d1.Sub(d2) > 0; d1 = d1.AddDate(0, -step, 0) {
		d3 = d1
	}

	// calculate day count fraction
	frac := daycount.Fraction(d1, d2, d3, m.Compounding(), m.Basis)

	return frac
}

// helper functions

// Difference between two dates in years (Act/Act)
func ActualDifferenceInYears(start, stop time.Time) float64 {
	return float64(stop.Sub(start).Hours()) / 24.0 / 365.25
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
