package bonds

type Bond interface {
	Accrued() float64
	Pricing(zspread float64, ts TermStructure) (float64, float64)
	RemainingYears() float64
}
