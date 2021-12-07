package bond

import (
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/term"
)

// Straight represents a straight-bond fixed income security
type Straight struct {
	Schedule   maturity.Schedule
	Coupon     float64
	Redemption float64
}

// Accrued calculated the accrued interest
func (b *Straight) Accrued() float64 {
	return b.Coupon * b.Schedule.DayCountFraction()
}

// PresentValue returns the "dirty" bond prices
// (for the "clean" price just subtract the accrued interest)
func (b *Straight) PresentValue(ts term.Structure) float64 {
	dcf := 0.0

	maturities := b.Schedule.M()
	n := b.Schedule.Compounding()

	// discount coupon payments
	for _, m := range maturities {
		dcf += b.Coupon / float64(n) * ts.Z(m)
	}

	// discount redemption value
	dcf += b.Redemption * ts.Z(b.YearsToMaturity())

	return dcf - b.Accrued()
}

// YearsToMaturity calculates the number of years until maturity
func (b *Straight) YearsToMaturity() float64 {
	return b.Schedule.YearsToMaturity()
}

// Duration calculates the duration of the bond
// dP/P = -D * dr
func (b *Straight) Duration(ts term.Structure) float64 {
	duration := 0.0

	maturities := b.Schedule.M()
	n := b.Schedule.Compounding()
	p := b.PresentValue(ts)
	if p == 0.0 {
		return 0.0
	}

	// discount coupon payments
	for _, m := range maturities {
		duration += m * b.Coupon / float64(n) * ts.Z(m)
	}

	// discount redemption value
	years := b.YearsToMaturity()
	duration += years * b.Redemption * ts.Z(years)

	return -duration / p
}

// Convexity calculates the modified duration of the bond
// dP/P = -D * dr + 1/2 * C * dr^2
func (b *Straight) Convexity(ts term.Structure) float64 {
	convex := 0.0

	maturities := b.Schedule.M()
	n := b.Schedule.Compounding()
	p := b.PresentValue(ts)
	if p == 0.0 {
		return 0.0
	}

	// discount coupon payments
	for _, m := range maturities {
		convex += m * m * b.Coupon / float64(n) * ts.Z(m)
	}

	// discount redemption value
	years := b.YearsToMaturity()
	convex += years * years * b.Redemption * ts.Z(years)

	return convex / p
}
