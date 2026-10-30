// Package bond implements fixed-coupon (straight) and floating-rate bonds.
//
// Conventions: coupon rates and margins are annual decimals (0.0125 =
// 1.25%); amounts are in currency units of the face value. Cash flows
// strictly after the settlement date are received (a coupon paid on the
// settlement date belongs to the seller, accrued interest is 0); a bond
// settled on or after its maturity date has no cash flows and a present
// value of 0. Discounting times are ACT/365 Fixed from the settlement date
// (see maturity.YearFraction).
//
// All instruments have PresentValue(settle, discount[, forecast]); bonds
// additionally have DirtyPrice (= PresentValue), CleanPrice and Accrued.
// Risk measures are the functions of package cashflow applied to the cash
// flows, and FloatingDuration/FloatingConvexity for floating-rate notes.
package bond

import (
	"errors"
	"math"
	"time"

	"github.com/konimarti/fixedincome/pkg/cashflow"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/term"
)

var _ cashflow.Instrument = Straight{}

// Straight is a fixed-coupon bond. Use NewStraight for the default face
// value; with a struct literal all fields must be set explicitly.
//
// The coupon of a period is Coupon * Face * CouponFraction (see
// maturity.Schedule.CouponFraction): Coupon*Face/Frequency for a regular
// period with a 30/360 or ACT/ACT ICMA convention (standard bond market
// practice), and Coupon*Face*accrual fraction for ACT/360 and for irregular
// first periods. Accrued interest is Coupon*Face times the accrual fraction
// of the day count convention (see maturity.Schedule.AccrualFraction).
type Straight struct {
	maturity.Schedule
	// Coupon is the annual coupon rate (decimal of Face)
	Coupon float64
	// Face is the face value (notional, > 0)
	Face float64
	// Redemption is the amount repaid at maturity (currency units, >= 0)
	Redemption float64
}

// NewStraight returns a validated straight bond with the given schedule and
// annual coupon rate (decimal). The face value is DefaultFace (100) and the
// redemption equals the face value unless set with WithFace and
// WithRedemption.
func NewStraight(s maturity.Schedule, coupon float64, opts ...Option) (Straight, error) {
	o, err := applyOptions(opts)
	if err != nil {
		return Straight{}, err
	}
	if o.fixing != nil {
		return Straight{}, errors.New("bond: WithFixing is not applicable to a straight bond")
	}
	b := Straight{Schedule: s, Coupon: coupon, Face: o.face, Redemption: *o.redemption}
	return b, b.Validate()
}

// Validate returns an error if the schedule is invalid, a parameter is not
// finite, the face value is not positive or the redemption is negative.
func (b Straight) Validate() error {
	if err := b.Schedule.Validate(); err != nil {
		return err
	}
	if err := checkFinite(param{"coupon", b.Coupon}, param{"face", b.Face}, param{"redemption", b.Redemption}); err != nil {
		return err
	}
	return checkAmounts(b.Face, b.Redemption)
}

// flows returns the cash flows and the accrued interest at settle, building
// the schedule only once.
func (b Straight) flows(settle time.Time) ([]cashflow.CashFlow, float64, error) {
	if err := b.Validate(); err != nil {
		return nil, 0, err
	}
	periods, err := b.Periods(settle)
	if err != nil {
		return nil, 0, err
	}
	s0 := maturity.Date(settle)
	cfs := make([]cashflow.CashFlow, len(periods))
	for i, p := range periods {
		frac, _ := b.CouponFraction(p)
		cfs[i] = cashflow.CashFlow{Date: p.End, T: float64(maturity.Days(s0, p.End)) / 365.0, Amount: b.Coupon * b.Face * frac}
	}
	accrued := 0.0
	if n := len(cfs); n > 0 {
		cfs[n-1].Amount += b.Redemption
		if p := periods[0]; p.Start.Before(s0) {
			frac, _ := b.AccrualFraction(p.Start, s0, p.End)
			accrued = b.Coupon * b.Face * frac
		}
	}
	return cfs, accrued, nil
}

// CashFlows returns the coupon and redemption payments strictly after
// settle in ascending order; the redemption is added to the last coupon.
// It returns an empty slice if the bond has matured.
func (b Straight) CashFlows(settle time.Time) ([]cashflow.CashFlow, error) {
	cfs, _, err := b.flows(settle)
	return cfs, err
}

// Accrued returns the accrued interest at settle: Coupon*Face times the
// accrual fraction from the previous coupon (or issue) date to settle. It is
// 0 on a coupon date, before the issue date and after maturity.
func (b Straight) Accrued(settle time.Time) (float64, error) {
	if err := b.Validate(); err != nil {
		return 0, err
	}
	p, err := b.PeriodAt(settle)
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
	frac, _ := b.AccrualFraction(p.Start, s0, p.End)
	return b.Coupon * b.Face * frac, nil
}

// PresentValue returns the dirty price: the present value of the cash flows
// with the curve c.
func (b Straight) PresentValue(settle time.Time, c term.Curve) (float64, error) {
	cfs, _, err := b.flows(settle)
	if err != nil {
		return math.NaN(), err
	}
	return cashflow.PV(cfs, c), nil
}

// DirtyPrice returns the present value of the cash flows (full price
// including accrued interest) with the curve c; same as PresentValue.
func (b Straight) DirtyPrice(settle time.Time, c term.Curve) (float64, error) {
	return b.PresentValue(settle, c)
}

// CleanPrice returns the dirty price minus the accrued interest.
func (b Straight) CleanPrice(settle time.Time, c term.Curve) (float64, error) {
	cfs, accrued, err := b.flows(settle)
	if err != nil {
		return math.NaN(), err
	}
	return cashflow.PV(cfs, c) - accrued, nil
}
