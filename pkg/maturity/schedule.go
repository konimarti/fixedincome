package maturity

import (
	"fmt"
	"sort"
	"time"

	"github.com/konimarti/daycount"
)

// Schedule contain the information about the term maturities of fixed income security's cash flows
type Schedule struct {
	// Settlement represent the date of valuation (or settlement)
	Settlement time.Time
	// Maturity represents the maturity date
	Maturity time.Time
	// Frequency is the compounding frequency per year (default: 1x per year).
	// Valid frequencies are 1, 2, 3, 4, 6 and 12; values <= 0 are treated as 1.
	// M() and DayCountFraction() panic for other frequencies.
	Frequency int
	// Basis represents the day count convention (default: "" for 30E/360 ISDA)
	Basis string
}

// Compounding returns the annual compounding frequency
func (m *Schedule) Compounding() int {
	n := 1
	if m.Frequency > 0 {
		n = m.Frequency
	}
	return n
}

// EffectiveCoupon calculates the coupon that is payable given the annual coupon rate
func (m *Schedule) EffectiveCoupon(annualCoupon float64) float64 {
	n := float64(m.Compounding())
	return annualCoupon / n
}

// step returns the number of months between two cash flows; it panics if the
// compounding frequency does not divide the year into whole months
func (m *Schedule) step() int {
	n := m.Compounding()
	if n > 12 {
		panic("more than 12 compounding periods not implemented yet")
	}
	if 12%n != 0 {
		panic(fmt.Sprintf("invalid compounding frequency %d: must be one of 1, 2, 3, 4, 6, 12", n))
	}
	return 12 / n
}

// couponDate returns the date that lies k*step months before the maturity
// date. The day of the maturity date is kept unless it does not exist in the
// target month, in which case the last day of the month is used. If the
// maturity date is the last day of its month, the coupon date is the last day
// of the target month (end-of-month rule).
func (m *Schedule) couponDate(k, step int) time.Time {
	mat := m.Maturity
	// first day of the target month
	first := time.Date(mat.Year(), mat.Month()-time.Month(k*step), 1,
		mat.Hour(), mat.Minute(), mat.Second(), mat.Nanosecond(), mat.Location())
	lastDay := daysIn(first)
	day := mat.Day()
	if day > lastDay || mat.Day() == daysIn(mat) {
		day = lastDay
	}
	return first.AddDate(0, 0, day-1)
}

// daysIn returns the number of days in the month of t
func daysIn(t time.Time) int {
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// M returns a slice of the effective maturities in years of the bond's cash
// flows in descending order. It panics for an invalid compounding frequency.
func (m *Schedule) M() []float64 {
	maturities := []float64{}

	step := m.step()

	// walk back from maturity date to quote date
	quote := m.Settlement
	for k, current := 0, m.Maturity; current.Sub(quote) > 0; k, current = k+1, m.couponDate(k+1, step) {
		frac, err := daycount.Fraction(quote, current, quote.AddDate(1, 0, 0), m.Basis)
		if err != nil {
			panic(err)
		}
		maturities = append(maturities, frac)
	}

	return maturities
}

// Last returns the latest maturity value in years (i.e. the years to maturity)
func (m *Schedule) Last() float64 {
	t := m.M()
	if len(t) == 0 {
		return 0.0
	}
	sort.Float64s(t)
	return t[len(t)-1]
}

// Next returns the next maturity value in years
func (m *Schedule) Next() float64 {
	t := m.M()
	if len(t) == 0 {
		return 0.0
	}
	sort.Float64s(t)
	return t[0]
}

// DayCountFraction returns year fraction since last coupon. It returns 0 if
// the settlement date is not before the maturity date and panics for an
// invalid compounding frequency.
func (m *Schedule) DayCountFraction() float64 {
	step := m.step()

	if !m.Settlement.Before(m.Maturity) {
		return 0.0
	}

	d1 := m.Maturity
	d2 := m.Settlement
	d3 := time.Time{}

	// iterate maturity date backwards until last coupon date before settlement date
	for k := 1; d1.Sub(d2) > 0; k++ {
		d3 = d1
		d1 = m.couponDate(k, step)
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
