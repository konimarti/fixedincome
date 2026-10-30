// Package swap implements plain vanilla interest rate swaps and swap rates.
//
// Conventions: rates and margins are decimals (simple rates for the
// floating index, annual fixed rates); discounting times are ACT/365 Fixed
// from the valuation date (see maturity.YearFraction).
package swap

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/konimarti/fixedincome/pkg/cashflow"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/term"
)

var _ cashflow.Projected = InterestRateSwap{}

// ErrNoFixing is returned if the valuation date is inside a floating period
// and CurrentFixing is not set.
var ErrNoFixing = errors.New("swap: current fixing required for the floating period containing the valuation date")

// InterestRateSwap is a plain vanilla fixed-for-floating interest rate swap
// without exchange of notionals.
//
// The fixed leg pays Notional * FixedRate * tau for each period of
// FixedSchedule, the floating leg Notional * (index + Margin) * tau for each
// period of FloatSchedule, where tau is the accrual fraction of the period
// with the schedule's day count convention (e.g. Thirty360E for the fixed
// and Act360 for the floating leg). The index is a term rate (IBOR-style
// simple rate fixed at the start of the period); overnight rates compounded
// in arrears (SARON, SOFR, €STR) are not modelled. The index rate of the
// floating period containing the valuation date (including a period
// starting on it) is CurrentFixing if set; later index rates (and the rate
// of a period starting on the valuation date if CurrentFixing is nil) are
// the simple forward rates of the forecast curve over the period. Payments
// on or before the valuation date are excluded.
type InterestRateSwap struct {
	// Notional is the notional amount (> 0)
	Notional float64
	// FixedRate is the annual fixed rate (decimal)
	FixedRate float64
	// FixedSchedule and FloatSchedule are the schedules of the legs
	FixedSchedule, FloatSchedule maturity.Schedule
	// Margin is the spread over the floating index (decimal)
	Margin float64
	// CurrentFixing is the index rate of the floating period containing the
	// valuation date (decimal); required if the valuation date is strictly
	// inside a period (ErrNoFixing otherwise)
	CurrentFixing *float64
	// Payer is true for the payer of the fixed rate (receives floating)
	Payer bool
}

// Validate returns an error if a schedule is invalid, a parameter is not
// finite or the notional is not positive.
func (s InterestRateSwap) Validate() error {
	if err := s.FixedSchedule.Validate(); err != nil {
		return fmt.Errorf("swap: fixed leg: %w", err)
	}
	if err := s.FloatSchedule.Validate(); err != nil {
		return fmt.Errorf("swap: floating leg: %w", err)
	}
	fixing := 0.0
	if s.CurrentFixing != nil {
		fixing = *s.CurrentFixing
	}
	for _, p := range []struct {
		name  string
		value float64
	}{{"notional", s.Notional}, {"fixed rate", s.FixedRate}, {"margin", s.Margin}, {"current fixing", fixing}} {
		if math.IsNaN(p.value) || math.IsInf(p.value, 0) {
			return fmt.Errorf("swap: %s is not finite: %v", p.name, p.value)
		}
	}
	if !(s.Notional > 0) {
		return fmt.Errorf("swap: notional %v must be positive (use Payer for the direction)", s.Notional)
	}
	return nil
}

// fixedFlows returns the fixed leg cash flows per unit of fixed rate
// (Notional * tau), unsigned.
func (s InterestRateSwap) fixedFlows(settle time.Time) ([]cashflow.CashFlow, error) {
	periods, err := s.FixedSchedule.Periods(settle)
	if err != nil {
		return nil, err
	}
	s0 := maturity.Date(settle)
	cfs := make([]cashflow.CashFlow, len(periods))
	for i, p := range periods {
		tau, _ := s.FixedSchedule.AccrualFraction(p.Start, p.End, p.End)
		cfs[i] = cashflow.CashFlow{Date: p.End, T: maturity.YearFraction(s0, p.End), Amount: s.Notional * tau}
	}
	return cfs, nil
}

// floatFlows returns the floating leg cash flows (including the margin),
// unsigned.
func (s InterestRateSwap) floatFlows(settle time.Time, forecast term.Curve) ([]cashflow.CashFlow, error) {
	periods, err := s.FloatSchedule.Periods(settle)
	if err != nil {
		return nil, err
	}
	s0 := maturity.Date(settle)
	cfs := make([]cashflow.CashFlow, len(periods))
	for i, p := range periods {
		tau, _ := s.FloatSchedule.AccrualFraction(p.Start, p.End, p.End)
		t2 := maturity.YearFraction(s0, p.End)
		var index float64
		switch {
		case i == 0 && !p.Start.After(s0) && s.CurrentFixing != nil:
			index = *s.CurrentFixing
		case p.Start.Before(s0):
			return nil, ErrNoFixing
		default:
			if forecast == nil {
				return nil, fmt.Errorf("swap: forecast curve is nil")
			}
			t1 := maturity.YearFraction(s0, p.Start)
			index = (forecast.Discount(t1)/forecast.Discount(t2) - 1) / tau
		}
		cfs[i] = cashflow.CashFlow{Date: p.End, T: t2, Amount: s.Notional * (index + s.Margin) * tau}
	}
	return cfs, nil
}

// CashFlows returns the net cash flows of the swap after settle in
// ascending order, signed from the point of view of the holder (Payer:
// floating minus fixed payments, receiver: fixed minus floating); payments
// of both legs on the same date are netted.
func (s InterestRateSwap) CashFlows(settle time.Time, forecast term.Curve) ([]cashflow.CashFlow, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	fixed, err := s.fixedFlows(settle)
	if err != nil {
		return nil, err
	}
	float, err := s.floatFlows(settle, forecast)
	if err != nil {
		return nil, err
	}
	sign := 1.0
	if !s.Payer {
		sign = -1.0
	}
	cfs := make([]cashflow.CashFlow, 0, len(fixed)+len(float))
	add := func(cf cashflow.CashFlow, amount float64) {
		if n := len(cfs); n > 0 && cfs[n-1].Date.Equal(cf.Date) {
			cfs[n-1].Amount += amount
			return
		}
		cf.Amount = amount
		cfs = append(cfs, cf)
	}
	i, j := 0, 0
	for i < len(fixed) || j < len(float) {
		if j >= len(float) || (i < len(fixed) && !fixed[i].Date.After(float[j].Date)) {
			add(fixed[i], -sign*s.FixedRate*fixed[i].Amount)
			i++
		} else {
			add(float[j], sign*float[j].Amount)
			j++
		}
	}
	return cfs, nil
}

// Annuity returns the present value of the fixed leg per unit of fixed
// rate, Notional * sum(tau_i * Z(t_i)), with the discount curve.
func (s InterestRateSwap) Annuity(settle time.Time, discount term.Curve) (float64, error) {
	if err := s.Validate(); err != nil {
		return math.NaN(), err
	}
	cfs, err := s.fixedFlows(settle)
	if err != nil {
		return math.NaN(), err
	}
	return cashflow.PV(cfs, discount), nil
}

// FloatLeg returns the present value of the floating leg (including the
// margin), with index rates projected from the forecast curve and
// discounted with the discount curve.
func (s InterestRateSwap) FloatLeg(settle time.Time, discount, forecast term.Curve) (float64, error) {
	if err := s.Validate(); err != nil {
		return math.NaN(), err
	}
	cfs, err := s.floatFlows(settle, forecast)
	if err != nil {
		return math.NaN(), err
	}
	return cashflow.PV(cfs, discount), nil
}

// PresentValue returns the value of the swap as of settle: floating leg
// minus fixed leg for the payer of the fixed rate, fixed minus floating for
// the receiver.
func (s InterestRateSwap) PresentValue(settle time.Time, discount, forecast term.Curve) (float64, error) {
	cfs, err := s.CashFlows(settle, forecast)
	if err != nil {
		return math.NaN(), err
	}
	return cashflow.PV(cfs, discount), nil
}

// ParRate returns the fixed rate (decimal) at which the swap has zero
// value: floating leg / annuity. With a single curve, zero margin and a
// valuation date at the start of the swap (without CurrentFixing), it
// equals (1 - Z(T)) / annuity.
func (s InterestRateSwap) ParRate(settle time.Time, discount, forecast term.Curve) (float64, error) {
	a, err := s.Annuity(settle, discount)
	if err != nil {
		return math.NaN(), err
	}
	if a == 0 {
		return math.NaN(), fmt.Errorf("swap: annuity of the fixed leg is zero (matured swap?)")
	}
	fl, err := s.FloatLeg(settle, discount, forecast)
	if err != nil {
		return math.NaN(), err
	}
	return fl / a, nil
}
