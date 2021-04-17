package bonds

// FixedCouponBond
type FixedCouponBond struct {
	//QuotePrice      float64
	Schedule        Maturities
	CouponRate      float64
	RedemptionValue float64
}

// Accrued calculated the accrued interest
func (b *FixedCouponBond) Accrued() float64 {
	return b.CouponRate * b.Schedule.DaysSinceLastCouponInYears()
}

// Pricing returns the "dirty" and the "clean" prices (adjusted for accrued interest)
func (b *FixedCouponBond) Pricing(zspread float64, ts TermStructure) (float64, float64) {
	maturities := b.Schedule.M()
	freq := b.Schedule.GetFrequency()
	dcf := 0.0

	// discount coupon payments
	for _, m := range maturities {
		dcf += b.CouponRate / float64(freq) * ts.D(m, zspread, freq)
	}

	// discount redemption value
	dcf += b.RedemptionValue * ts.D(b.RemainingYears(), zspread, freq)

	return dcf, dcf - b.Accrued()
}

// RemainingYears
func (b *FixedCouponBond) RemainingYears() float64 {
	return b.Schedule.RemainingYears()
}
