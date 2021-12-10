package maturity

import (
	"time"

	"github.com/konimarti/daycount"
)

// Schedule contain the information about the term maturities of fixed income security's cash flows
type Schedule struct {
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
func (m *Schedule) Compounding() int {
	n := 1
	if m.Frequency > 0 {
		n = m.Frequency
	}
	return n
}

//M returns a slice of the effective maturities in years of the bond's cash flows
func (m *Schedule) M() []float64 {
	maturities := []float64{}

	if m.Compounding() > 12 {
		panic("more than 12 compounding periods not implemented yet")
	}
	step := 12 / m.Compounding()

	// walk back from maturity date to quote date
	quote := m.Settlement
	for current := m.Maturity; current.Sub(quote) > 0; current = current.AddDate(0, -step, 0) {
		frac, err := daycount.Fraction(quote, current, quote.AddDate(1, 0, 0), m.Basis)
		if err != nil {
			panic(err)
		}
		maturities = append(maturities, frac)
	}

	return maturities
}

//YearsToMaturity calculates the time in years to redemption
func (m *Schedule) YearsToMaturity() float64 {
	if m.Maturity.Before(m.Settlement) {
		return 0.0
	}
	frac, err := daycount.Fraction(m.Settlement, m.Maturity, m.Settlement.AddDate(1, 0, 0), m.Basis)
	if err != nil {
		panic(err)
	}
	return frac
}

// DayCountFraction returns year fraction since last coupon
func (m *Schedule) DayCountFraction() float64 {
	if m.Maturity.Before(m.Settlement) {
		return 0.0
	}

	d1 := m.Maturity
	d2 := m.Settlement
	d3 := time.Time{}

	// iterate maturity date backwards until last coupon date before settlement date
	if m.Compounding() > 12 {
		panic("more than 12 compounding periods not implemented yet")
	}
	step := 12 / m.Compounding()
	for ; d1.Sub(d2) > 0; d1 = d1.AddDate(0, -step, 0) {
		d3 = d1
	}

	// calculate day count fraction
	frac, err := daycount.Fraction(d1, d2, d3, m.Basis)
	if err != nil {
		panic(err)
	}

	return frac / float64(m.Compounding())
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
