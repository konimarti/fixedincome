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
	freq := b.Schedule.GetFrequency()

	// discount coupon payments
	for _, m := range maturities {
		dcf += b.Coupon / float64(freq) * ts.Z(m, spread, freq)
	}

	// discount redemption value
	dcf += b.Redemption * ts.Z(b.YearsToMaturity(), spread, freq)

	return dcf, dcf - b.Accrued()
}

// YearsToMaturity calculates the number of years until maturity
func (b *Bond) YearsToMaturity() float64 {
	return b.Schedule.YearsToMaturity()
}
