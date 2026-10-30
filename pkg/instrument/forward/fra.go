package forward

import (
	"fmt"
	"math"
	"time"

	"github.com/konimarti/fixedincome/pkg/cashflow"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/term"
)

var _ cashflow.Projected = FRA{}

// FRA is a forward rate agreement on a term (IBOR-style) index: the payer
// pays the fixed rate FixedRate and receives the floating simple rate for
// the accrual period from Start to End on Notional, settled at End (which
// has the same value as the discounted settlement at Start).
//
// Its value for the payer is Notional * tau * (F - FixedRate) * Z(t2) with
// the simple forward rate F = (Zf(t1)/Zf(t2) - 1) / tau of the forecast
// curve, where tau = daycount.Days(Start, End) / 360 with DayCount (Act360
// is the money-market convention set by NewFRA; the 30/360 family is also
// supported, ActAct is rejected) and t1, t2 are the ACT/365F times of Start
// and End.
type FRA struct {
	// Notional is the notional amount (> 0)
	Notional float64
	// FixedRate is the agreed (simple) rate, decimal
	FixedRate float64
	// Start and End are the start and end dates of the accrual period
	Start, End time.Time
	// DayCount is the day count convention of the accrual fraction
	DayCount maturity.DayCount
	// Payer is true for the payer of the fixed rate (long the FRA)
	Payer bool
}

// NewFRA returns a validated FRA with the money-market day count convention
// Act360.
func NewFRA(notional, fixedRate float64, start, end time.Time, payer bool) (FRA, error) {
	f := FRA{Notional: notional, FixedRate: fixedRate, Start: start, End: end, DayCount: maturity.Act360, Payer: payer}
	return f, f.Validate()
}

// Validate returns an error if End is not after Start, the day count
// convention is invalid or ActAct, the notional is not positive or a
// parameter is not finite.
func (f FRA) Validate() error {
	if f.Start.IsZero() || !maturity.Date(f.End).After(maturity.Date(f.Start)) {
		return fmt.Errorf("forward: FRA end date %s is not after start date %s", f.End.Format("2006-01-02"), f.Start.Format("2006-01-02"))
	}
	if err := f.DayCount.Validate(); err != nil {
		return fmt.Errorf("forward: %w", err)
	}
	if f.DayCount == maturity.ActAct {
		return fmt.Errorf("forward: day count convention ACTACT (ACT/ACT ICMA) is not defined for an FRA period")
	}
	if math.IsNaN(f.Notional) || math.IsInf(f.Notional, 0) || math.IsNaN(f.FixedRate) || math.IsInf(f.FixedRate, 0) {
		return fmt.Errorf("forward: notional and fixed rate must be finite")
	}
	if !(f.Notional > 0) {
		return fmt.Errorf("forward: notional %v must be positive (use Payer for the direction)", f.Notional)
	}
	return nil
}

// AccrualFraction returns the accrual fraction tau of the FRA period.
func (f FRA) AccrualFraction() (float64, error) {
	if err := f.Validate(); err != nil {
		return math.NaN(), err
	}
	days, err := f.DayCount.Days(f.Start, f.End)
	return days / 360.0, err
}

// ForwardRate returns the simple forward rate F (decimal) of the FRA period
// implied by the forecast curve as of settle.
func (f FRA) ForwardRate(settle time.Time, forecast term.Curve) (float64, error) {
	tau, err := f.AccrualFraction()
	if err != nil {
		return math.NaN(), err
	}
	t1, t2 := maturity.YearFraction(settle, f.Start), maturity.YearFraction(settle, f.End)
	return (forecast.Discount(t1)/forecast.Discount(t2) - 1) / tau, nil
}

// CashFlows returns the single (signed) settlement amount at End,
// Notional * tau * (F - FixedRate) for the payer, with F projected from the
// forecast curve. An error is returned if the FRA is invalid or its rate has
// already been fixed (Start before settle).
func (f FRA) CashFlows(settle time.Time, forecast term.Curve) ([]cashflow.CashFlow, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	if maturity.Date(f.Start).Before(maturity.Date(settle)) {
		return nil, fmt.Errorf("forward: FRA start date %s is before the valuation date", f.Start.Format("2006-01-02"))
	}
	fwd, _ := f.ForwardRate(settle, forecast)
	tau, _ := f.AccrualFraction()
	amount := f.Notional * tau * (fwd - f.FixedRate)
	if !f.Payer {
		amount = -amount
	}
	return []cashflow.CashFlow{cashflow.New(settle, f.End, amount)}, nil
}

// PresentValue returns the value of the FRA as of settle (see FRA) with the
// forward rate from the forecast curve and discounting on the discount
// curve.
func (f FRA) PresentValue(settle time.Time, discount, forecast term.Curve) (float64, error) {
	cfs, err := f.CashFlows(settle, forecast)
	if err != nil {
		return math.NaN(), err
	}
	return cashflow.PV(cfs, discount), nil
}
