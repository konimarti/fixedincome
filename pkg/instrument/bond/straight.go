package bond

import (
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/term"
)

// Straight represents a straight-bond fixed income security
type Straight struct {
	maturity.Schedule
	Coupon     float64
	Redemption float64
}

// Accrued calculated the accrued interest
func (b *Straight) Accrued() float64 {
	return b.Coupon * b.DayCountFraction()
}

// PresentValue returns the "dirty" bond prices
// (for the "clean" price just subtract the accrued interest)
func (b *Straight) PresentValue(ts term.Structure) float64 {
	dcf := 0.0

	// discount coupon payments
	effCoupon := b.EffectiveCoupon(b.Coupon)
	for _, m := range b.M() {
		dcf += effCoupon * ts.Z(m)
	}

	// discount redemption value
	dcf += b.Redemption * ts.Z(b.Last())

	return dcf
}

// Duration calculates the duration of the bond
// dP/P = -D * dr
func (b *Straight) Duration(ts term.Structure) float64 {
	duration := 0.0

	p := b.PresentValue(ts)
	if p == 0.0 {
		return 0.0
	}

	// discount coupon payments
	effCoupon := b.EffectiveCoupon(b.Coupon)
	for _, m := range b.M() {
		duration += m * effCoupon * ts.Z(m)
	}

	// discount redemption value
	duration += b.Last() * b.Redemption * ts.Z(b.Last())

	return -duration / p
}

// Convexity calculates the modified duration of the bond
// dP/P = -D * dr + 1/2 * C * dr^2
func (b *Straight) Convexity(ts term.Structure) float64 {
	convex := 0.0

	p := b.PresentValue(ts)
	if p == 0.0 {
		return 0.0
	}

	// discount coupon payments
	effCoupon := b.EffectiveCoupon(b.Coupon)
	for _, m := range b.M() {
		convex += m * m * effCoupon * ts.Z(m)
	}

	// discount redemption value
	convex += b.Last() * b.Last() * b.Redemption * ts.Z(b.Last())

	return convex / p
}
