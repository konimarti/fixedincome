package bonds

// Bond represents a fixed income security
type Bond struct {
	Schedule   Maturities
	Coupon     float64
	Redemption float64
}

// Accrued calculated the accrued interest
func (b *Bond) Accrued() float64 {
	return b.Coupon * b.Schedule.DaysSinceLastCouponInYears()
}

// Pricing returns the "dirty" and the "clean" prices (adjusted for accrued interest)
func (b *Bond) Pricing(spread float64, ts TermStructure) (float64, float64) {
	dcf := 0.0

	maturities := b.Schedule.M()
	n := b.Schedule.Compounding()

	// discount coupon payments
	for _, m := range maturities {
		dcf += b.Coupon / float64(n) * ts.Z(m, spread, n)
	}

	// discount redemption value
	dcf += b.Redemption * ts.Z(b.YearsToMaturity(), spread, n)

	return dcf, dcf - b.Accrued()
}

// YearsToMaturity calculates the number of years until maturity
func (b *Bond) YearsToMaturity() float64 {
	return b.Schedule.YearsToMaturity()
}

// Duration calculates the modified duration of the bond
func (b *Bond) Duration(spread float64, ts TermStructure) float64 {
	duration := 0.0

	maturities := b.Schedule.M()
	n := b.Schedule.Compounding()
	p, _ := b.Pricing(spread, ts)
	if p == 0.0 {
		return 0.0
	}

	// discount coupon payments
	for _, m := range maturities {
		duration += m * b.Coupon / float64(n) * ts.Z(m, spread, n)
	}

	// discount redemption value
	years := b.YearsToMaturity()
	duration += years * b.Redemption * ts.Z(years, spread, n)

	return duration / p
}
