package bond

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/konimarti/fixedincome/pkg/cashflow"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/term"
)

var _ cashflow.Projected = Floating{}

// Floating is a floating-rate note (FRN) on a term index rate. Use
// NewFloating for the default face value; with a struct literal all fields
// must be set explicitly.
//
// The coupon of a period is Face * (index rate + Margin) * accrual fraction
// of the period (with the schedule's day count convention, e.g. Act360).
// The index is a term rate fixed at the start of the period (IBOR-style
// simple rate): the index rate of the period containing the settlement
// date is CurrentFixing if it is set; the index rates of later periods, and
// of a period starting on the settlement date if CurrentFixing is nil, are
// the simple forward rates of the forecast curve over the accrual period,
// F = (Z(t_start)/Z(t_end) - 1) / accrual fraction. Overnight rates
// compounded in arrears (SARON, SOFR, €STR) are not modelled.
type Floating struct {
	maturity.Schedule
	// Margin is the quoted margin over the index rate (annual decimal)
	Margin float64
	// CurrentFixing is the index rate (decimal, simple) of the coupon
	// period containing the settlement date, including a period starting on
	// the settlement date. It is required (ErrNoFixing otherwise) if the
	// settlement date is strictly inside a period; nil projects the index of
	// a period starting on the settlement date from the forecast curve.
	CurrentFixing *float64
	// Face is the face value (notional, > 0)
	Face float64
	// Redemption is the amount repaid at maturity (currency units, >= 0)
	Redemption float64
}

// NewFloating returns a validated floating-rate note with the given schedule
// and margin (decimal). The face value is DefaultFace (100), the redemption
// equals the face value and there is no current fixing unless set with
// WithFace, WithRedemption and WithFixing.
func NewFloating(s maturity.Schedule, margin float64, opts ...Option) (Floating, error) {
	o, err := applyOptions(opts)
	if err != nil {
		return Floating{}, err
	}
	f := Floating{Schedule: s, Margin: margin, CurrentFixing: o.fixing, Face: o.face, Redemption: *o.redemption}
	return f, f.Validate()
}

// Validate returns an error if the schedule is invalid, a parameter is not
// finite, the face value is not positive or the redemption is negative.
func (f Floating) Validate() error {
	if err := f.Schedule.Validate(); err != nil {
		return err
	}
	fixing := 0.0
	if f.CurrentFixing != nil {
		fixing = *f.CurrentFixing
	}
	if err := checkFinite(param{"margin", f.Margin}, param{"current fixing", fixing}, param{"face", f.Face}, param{"redemption", f.Redemption}); err != nil {
		return err
	}
	return checkAmounts(f.Face, f.Redemption)
}

// CashFlows returns the projected coupon and redemption payments strictly
// after settle in ascending order, with the index rates projected from the
// forecast curve (see Floating). It returns ErrNoFixing if settle is inside a
// coupon period and CurrentFixing is nil.
func (f Floating) CashFlows(settle time.Time, forecast term.Curve) ([]cashflow.CashFlow, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	periods, err := f.Periods(settle)
	if err != nil {
		return nil, err
	}
	s0 := maturity.Date(settle)
	cfs := make([]cashflow.CashFlow, len(periods))
	for i, p := range periods {
		tau, _ := f.AccrualFraction(p.Start, p.End, p.End)
		var index float64
		switch {
		case i == 0 && !p.Start.After(s0) && f.CurrentFixing != nil:
			index = *f.CurrentFixing
		case p.Start.Before(s0):
			return nil, ErrNoFixing
		default:
			if forecast == nil {
				return nil, fmt.Errorf("bond: forecast curve is nil")
			}
			t1, t2 := float64(maturity.Days(s0, p.Start))/365.0, float64(maturity.Days(s0, p.End))/365.0
			index = (forecast.Discount(t1)/forecast.Discount(t2) - 1) / tau
		}
		cfs[i] = cashflow.CashFlow{Date: p.End, T: float64(maturity.Days(s0, p.End)) / 365.0, Amount: f.Face * (index + f.Margin) * tau}
	}
	if n := len(cfs); n > 0 {
		cfs[n-1].Amount += f.Redemption
	}
	return cfs, nil
}

// Accrued returns the accrued interest at settle, Face * (CurrentFixing +
// Margin) * accrual fraction from the previous coupon date to settle. It
// returns ErrNoFixing if settle is inside a coupon period and CurrentFixing
// is nil.
func (f Floating) Accrued(settle time.Time) (float64, error) {
	if err := f.Validate(); err != nil {
		return 0, err
	}
	p, err := f.PeriodAt(settle)
	if errors.Is(err, maturity.ErrMatured) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	s0 := maturity.Date(settle)
	if !p.Start.Before(s0) {
		return 0, nil
	}
	if f.CurrentFixing == nil {
		return 0, ErrNoFixing
	}
	frac, _ := f.AccrualFraction(p.Start, s0, p.End)
	return f.Face * (*f.CurrentFixing + f.Margin) * frac, nil
}

// PresentValue returns the dirty price: the projected cash flows (forecast
// curve) discounted with the discount curve. If forecast and discount are
// the same curve and the margin is 0, the value equals the par-at-reset
// value (Face + current coupon) * Z(next coupon date).
func (f Floating) PresentValue(settle time.Time, discount, forecast term.Curve) (float64, error) {
	cfs, err := f.CashFlows(settle, forecast)
	if err != nil {
		return math.NaN(), err
	}
	return cashflow.PV(cfs, discount), nil
}

// DirtyPrice is the same as PresentValue.
func (f Floating) DirtyPrice(settle time.Time, discount, forecast term.Curve) (float64, error) {
	return f.PresentValue(settle, discount, forecast)
}

// CleanPrice returns the dirty price minus the accrued interest.
func (f Floating) CleanPrice(settle time.Time, discount, forecast term.Curve) (float64, error) {
	dirty, err := f.PresentValue(settle, discount, forecast)
	if err != nil {
		return math.NaN(), err
	}
	accrued, err := f.Accrued(settle)
	if err != nil {
		return math.NaN(), err
	}
	return dirty - accrued, nil
}

// shiftBump is the size of the parallel shift for FloatingDuration and
// FloatingConvexity.
const shiftBump = 1e-4

// shifted returns the present values for parallel shifts of -h, 0 and +h of
// both the discount and the forecast curve.
func shifted(f Floating, settle time.Time, discount, forecast term.Curve) (down, mid, up float64, err error) {
	values := [3]float64{}
	for i, h := range []float64{-shiftBump, 0, shiftBump} {
		values[i], err = f.PresentValue(settle, term.WithSpread(discount, h), term.WithSpread(forecast, h))
		if err != nil {
			return
		}
	}
	return values[0], values[1], values[2], nil
}

// FloatingDuration returns the (positive) modified duration -1/P dP/dy of
// the FRN for a parallel shift y of BOTH the discount and the forecast curve
// (central differences of 1bp), i.e. the rate duration; it is close to the
// time to the next reset date. (cashflow.Duration of the projected cash flows
// instead measures the discount-curve (spread) duration with the forecast
// held fixed.) It returns 0 if the present value is 0.
func FloatingDuration(f Floating, settle time.Time, discount, forecast term.Curve) (float64, error) {
	down, mid, up, err := shifted(f, settle, discount, forecast)
	if err != nil || mid == 0 {
		return 0, err
	}
	return -(up - down) / (2 * shiftBump * mid), nil
}

// FloatingConvexity returns the convexity 1/P d²P/dy² of the FRN for a
// parallel shift y of both the discount and the forecast curve (central
// differences of 1bp). It returns 0 if the present value is 0.
func FloatingConvexity(f Floating, settle time.Time, discount, forecast term.Curve) (float64, error) {
	down, mid, up, err := shifted(f, settle, discount, forecast)
	if err != nil || mid == 0 {
		return 0, err
	}
	return (up - 2*mid + down) / (shiftBump * shiftBump * mid), nil
}
