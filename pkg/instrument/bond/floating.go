package bond

import (
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/term"
)

// Floating represents a floating-rate bond
type Floating struct {
	maturity.Schedule
	// Rate is the current rate in percent for next coupon payment
	// which is known today
	Rate       float64
	Redemption float64
}

// Accrued calculated the accrued interest
func (f *Floating) Accrued() float64 {
	return f.Rate * f.Schedule.DayCountFraction()
}

// PresentValue returns the "dirty" bond prices (for the "clean" price just subtract the accrued interest)
func (f *Floating) PresentValue(ts term.Structure) float64 {
	pv := 0.0

	// discount face value at next reset date
	effRate := f.EffectiveCoupon(f.Rate)
	pv += (f.Redemption + effRate) * ts.Z(f.Next())

	return pv
}

// Duration calculates the duration of the floating-rate bond
// dP/P = -D * dr
func (f *Floating) Duration(ts term.Structure) float64 {
	p := f.PresentValue(ts)
	if p == 0.0 {
		return 0.0
	}

	// discount redemption value
	duration := f.Next() * (f.Redemption + f.EffectiveCoupon(f.Rate)) * ts.Z(f.Next())

	return -duration / p
}

// Convexity calculates the modified duration of the bond
// dP/P = -D * dr + 1/2 * C * dr^2
func (f *Floating) Convexity(ts term.Structure) float64 {
	p := f.PresentValue(ts)
	if p == 0.0 {
		return 0.0
	}

	convex := f.Next() * f.Next() * (f.Redemption + f.EffectiveCoupon(f.Rate)) * ts.Z(f.Next())

	return convex / p
}
