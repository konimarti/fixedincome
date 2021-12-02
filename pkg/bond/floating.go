package bond

import (
	"github.com/konimarti/bonds/pkg/maturity"
	"github.com/konimarti/bonds/pkg/term"
)

// Floating represents a floating-rate bond
type Floating struct {
	Schedule maturity.Schedule
	// Rate is the current rate in percent which is known today
	// e.g. for semi-annually compounding bonds, rate would be r_2(0)
	Rate       float64
	Redemption float64
}

// Accrued calculated the accrued interest
func (f *Floating) Accrued() float64 {
	return f.Rate * f.Schedule.DayCountFraction()
}

// PresentValue returns the "clean" bond prices (for the "dirty" price just add the accrued interest)
func (f *Floating) PresentValue(ts term.Structure) float64 {
	pv := 0.0

	maturities := f.Schedule.M()
	n := f.Schedule.Compounding()

	// discount face value at next reset date
	pv += (f.Redemption + f.Rate/float64(n)) * ts.Z(maturities[0])

	return pv - f.Accrued()
}

// YearsToMaturity calculates the number of years until maturity
func (f *Floating) YearsToMaturity() float64 {
	return f.Schedule.YearsToMaturity()
}

// Duration calculates the duration of the floating-rate bond
// dP/P = -D * dr
func (f *Floating) Duration(ts term.Structure) float64 {
	duration := 0.0

	maturities := f.Schedule.M()
	n := f.Schedule.Compounding()
	p := f.PresentValue(ts)
	if p == 0.0 {
		return 0.0
	}

	// discount redemption value
	years := maturities[0]
	duration += years * (f.Redemption + f.Rate/float64(n)) * ts.Z(years)

	return -duration / p
}

// Convexity calculates the modified duration of the bond
// dP/P = -D * dr + 1/2 * C * dr^2
func (f *Floating) Convexity(ts term.Structure) float64 {
	convex := 0.0

	maturities := f.Schedule.M()
	n := f.Schedule.Compounding()
	p := f.PresentValue(ts)
	if p == 0.0 {
		return 0.0
	}

	// discount redemption value
	years := maturities[0]
	convex += years * years * (f.Redemption + f.Rate/float64(n)) * ts.Z(years)

	return convex / p
}
