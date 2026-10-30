package bond

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/konimarti/fixedincome/internal/solve"
	"github.com/konimarti/fixedincome/pkg/cashflow"
	"github.com/konimarti/fixedincome/pkg/lattice"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/term"
)

// Call is an issuer call right: the issuer may redeem the bond on Date at
// the clean price Price per 100 of face value (the amount paid is
// Price/100*Face plus the accrued interest on Date).
type Call struct {
	Date  time.Time
	Price float64
}

// Put is an investor put right: the holder may sell the bond back to the
// issuer on Date at the clean price Price per 100 of face value (plus
// accrued interest).
type Put struct {
	Date  time.Time
	Price float64
}

// bumpSize is the parallel bump (decimal, 50bp) of the effective duration and
// convexity. Smaller bumps make the second difference noisy, because the
// exercise boundary moves between discrete lattice nodes.
const bumpSize = 5e-3

// Callable is a fixed-coupon bond with Bermudan call (and optionally put)
// rights, valued on a Hull-White trinomial lattice (package lattice).
//
// Each listed date is an exercise date (a Bermudan right: callable at each
// date in the list). A continuous call after a first date is expressed by
// listing the coupon dates (see Straight.CallsOnCoupons). Exercise dates are
// strictly before the maturity date, strictly increasing and a date is not
// both a call and a put date. Exercise dates before the settlement date are
// ignored; a right on the settlement date is exercisable immediately.
//
// Times of the lattice are measured from settle (ACT/365 Fixed); the curve
// passed to the valuation methods is assumed to be dated at settle.
//
// A coupon paid on an exercise date is received by the holder in any case;
// the exercise price is the clean price plus the accrued interest on that
// date (0 on a coupon date). Exercise is optimal for the issuer when the
// continuation value exceeds the strike (V = min(V, K)) and for the holder
// when it falls below it (V = max(V, K)).
type Callable struct {
	Straight Straight
	Calls    []Call
	Puts     []Put
}

// CallableOption is an option of NewCallable.
type CallableOption func(*Callable)

// WithPuts adds put rights to a callable bond.
func WithPuts(puts ...Put) CallableOption {
	return func(c *Callable) { c.Puts = append(c.Puts, puts...) }
}

// NewCallable returns a validated callable bond; calls may be empty if puts
// are given with WithPuts.
func NewCallable(s Straight, calls []Call, opts ...CallableOption) (Callable, error) {
	c := Callable{Straight: s, Calls: append([]Call(nil), calls...)}
	for _, o := range opts {
		o(&c)
	}
	return c, c.Validate()
}

// Validate returns an error if the underlying bond is invalid, a price is
// not positive and finite, the dates of calls or puts are not strictly
// increasing, not before the maturity date or not set, or a date is both a
// call and a put date.
func (c Callable) Validate() error {
	if err := c.Straight.Validate(); err != nil {
		return err
	}
	mat := maturity.Date(c.Straight.Maturity)
	check := func(kind string, dates []time.Time, prices []float64) error {
		for i, d := range dates {
			if d.IsZero() {
				return fmt.Errorf("bond: %s date %d is not set", kind, i)
			}
			if !(prices[i] > 0) || math.IsInf(prices[i], 0) {
				return fmt.Errorf("bond: %s price %v on %s must be positive and finite", kind, prices[i], d.Format("2006-01-02"))
			}
			if !maturity.Date(d).Before(mat) {
				return fmt.Errorf("bond: %s date %s is not before the maturity date %s", kind, d.Format("2006-01-02"), mat.Format("2006-01-02"))
			}
			if i > 0 && !maturity.Date(d).After(maturity.Date(dates[i-1])) {
				return fmt.Errorf("bond: %s dates must be strictly increasing (%s)", kind, d.Format("2006-01-02"))
			}
		}
		return nil
	}
	var cd, pd []time.Time
	var cp, pp []float64
	for _, x := range c.Calls {
		cd, cp = append(cd, x.Date), append(cp, x.Price)
	}
	for _, x := range c.Puts {
		pd, pp = append(pd, x.Date), append(pp, x.Price)
	}
	if err := check("call", cd, cp); err != nil {
		return err
	}
	if err := check("put", pd, pp); err != nil {
		return err
	}
	for _, a := range cd {
		for _, b := range pd {
			if maturity.Date(a).Equal(maturity.Date(b)) {
				return fmt.Errorf("bond: %s is both a call and a put date", a.Format("2006-01-02"))
			}
		}
	}
	return nil
}

// Accrued returns the accrued interest of the underlying bond at settle.
func (c Callable) Accrued(settle time.Time) (float64, error) {
	return c.Straight.Accrued(settle)
}

// CallsOnCoupons returns the calls on every coupon date from first (on or
// after first) until the last coupon date before maturity, all at the clean
// price Price per 100 of face value. It returns an error for an invalid
// bond or price.
func (b Straight) CallsOnCoupons(first time.Time, price float64) ([]Call, error) {
	if !(price > 0) || math.IsInf(price, 0) {
		return nil, fmt.Errorf("bond: call price %v must be positive and finite", price)
	}
	dates, err := b.CouponDates(maturity.Date(first).AddDate(0, 0, -1))
	if err != nil {
		return nil, err
	}
	var calls []Call
	for _, d := range dates {
		if d.Before(maturity.Date(b.Maturity)) {
			calls = append(calls, Call{Date: d, Price: price})
		}
	}
	return calls, nil
}

// model is the lattice with the payoff of the bond at a settlement date.
type model struct {
	tree  *lattice.Tree
	flows []lattice.Flow
	ex    []lattice.Exercise
	empty bool
}

func (m model) value(oas float64) (float64, error) {
	return m.tree.Value(m.flows, m.ex, oas)
}

// build constructs the payoff and the lattice calibrated to the curve.
func (c Callable) build(settle time.Time, curve term.Curve, a, sigma float64, opts []lattice.Option) (model, error) {
	if curve == nil {
		return model{}, errors.New("bond: nil curve")
	}
	if err := c.Validate(); err != nil {
		return model{}, err
	}
	cfs, _, err := c.Straight.flows(settle)
	if err != nil {
		return model{}, err
	}
	m := model{}
	if len(cfs) == 0 {
		m.empty = true
		return m, nil
	}
	s0 := maturity.Date(settle)
	times := make([]float64, 0, len(cfs)+len(c.Calls)+len(c.Puts))
	for _, cf := range cfs {
		m.flows = append(m.flows, lattice.Flow{T: cf.T, Amount: cf.Amount})
		times = append(times, cf.T)
	}
	add := func(date time.Time, price float64, side lattice.Side) error {
		d := maturity.Date(date)
		if d.Before(s0) {
			return nil
		}
		acc, err := c.Straight.Accrued(d)
		if err != nil {
			return err
		}
		t := maturity.YearFraction(s0, d)
		m.ex = append(m.ex, lattice.Exercise{T: t, Strike: price/100*c.Straight.Face + acc, Side: side})
		times = append(times, t)
		return nil
	}
	for _, x := range c.Calls {
		if err := add(x.Date, x.Price, lattice.Call); err != nil {
			return model{}, err
		}
	}
	for _, x := range c.Puts {
		if err := add(x.Date, x.Price, lattice.Put); err != nil {
			return model{}, err
		}
	}
	opts = append([]lattice.Option{lattice.WithTimes(times...)}, opts...)
	m.tree, err = lattice.New(curve, a, sigma, cfs[len(cfs)-1].T, opts...)
	return m, err
}

// Value returns the dirty value at settle with the Hull-White lattice of
// mean reversion a >= 0 and volatility sigma > 0 calibrated to the curve,
// discounted with the additional option-adjusted spread oas (decimal; 0 for
// the model value on the curve). It is 0 for a matured bond. The lattice
// options (e.g. lattice.WithStepsPerYear) set the resolution.
func (c Callable) Value(settle time.Time, curve term.Curve, a, sigma, oas float64, opts ...lattice.Option) (float64, error) {
	m, err := c.build(settle, curve, a, sigma, opts)
	if err != nil || m.empty {
		return 0, err
	}
	return m.value(oas)
}

// OAS returns the option-adjusted spread (continuously compounded, decimal)
// for which Value equals the dirty market price. It returns an error
// wrapping cashflow.ErrNoYield if there is none.
func (c Callable) OAS(dirty float64, settle time.Time, curve term.Curve, a, sigma float64, opts ...lattice.Option) (float64, error) {
	if math.IsNaN(dirty) || math.IsInf(dirty, 0) {
		return math.NaN(), fmt.Errorf("bond: invalid price %v", dirty)
	}
	m, err := c.build(settle, curve, a, sigma, opts)
	if err != nil {
		return math.NaN(), err
	}
	if m.empty {
		return math.NaN(), cashflow.ErrNoCashFlows
	}
	var ferr error
	f := func(s float64) float64 {
		v, err := m.value(s)
		if err != nil {
			ferr = err
			return math.NaN()
		}
		return v - dirty
	}
	s, err := solve.FindRoot(f, [][2]float64{{-0.05, 0.05}, {-0.2, 0.5}, {-1, 2}, {-3, 20}}, 1e-12)
	if ferr != nil {
		return math.NaN(), ferr
	}
	if err != nil {
		return math.NaN(), fmt.Errorf("%w: %v", cashflow.ErrNoYield, err)
	}
	return s, nil
}

// bumped returns the values at the zero curve shifted by -h, 0 and +h with
// oas held constant. A parallel shift of the curve by h is a shift of the
// discount rate by h in every node, which is the same as the spread oas+h on
// the lattice calibrated to the unshifted curve (Z(t)*exp(-h*t) is
// reproduced exactly), so the lattice is built once.
func (c Callable) bumped(settle time.Time, curve term.Curve, a, sigma, oas, h float64, opts []lattice.Option) (dn, v0, up float64, err error) {
	m, err := c.build(settle, curve, a, sigma, opts)
	if err != nil || m.empty {
		return 0, 0, 0, err
	}
	out := [3]float64{}
	for i, shift := range []float64{-h, 0, h} {
		if out[i], err = m.value(oas + shift); err != nil {
			return
		}
	}
	return out[0], out[1], out[2], nil
}

// EffectiveDuration returns -1/V dV/dr for a parallel shift r of the
// continuously compounded zero curve (central difference of +-50bp with oas
// held constant), in years. It is positive for a long position and 0 if the
// value is 0.
func (c Callable) EffectiveDuration(settle time.Time, curve term.Curve, a, sigma, oas float64, opts ...lattice.Option) (float64, error) {
	dn, v0, up, err := c.bumped(settle, curve, a, sigma, oas, bumpSize, opts)
	if err != nil || v0 == 0 {
		return 0, err
	}
	return (dn - up) / (2 * bumpSize * v0), nil
}

// EffectiveConvexity returns 1/V d²V/dr² for a parallel shift r of the
// zero curve (see EffectiveDuration), in years².
func (c Callable) EffectiveConvexity(settle time.Time, curve term.Curve, a, sigma, oas float64, opts ...lattice.Option) (float64, error) {
	dn, v0, up, err := c.bumped(settle, curve, a, sigma, oas, bumpSize, opts)
	if err != nil || v0 == 0 {
		return 0, err
	}
	return (dn + up - 2*v0) / (bumpSize * bumpSize * v0), nil
}

// DV01 returns the change in value for a parallel increase of the zero curve
// by one basis point, V(c+1bp) - V(c), with oas held constant. As
// cashflow.DV01 it is negative for a long position and a one-sided
// difference: it includes half the convexity term (about 0.5*V*C*1e-8 for
// the convexity C) in addition to the first-order change.
func (c Callable) DV01(settle time.Time, curve term.Curve, a, sigma, oas float64, opts ...lattice.Option) (float64, error) {
	m, err := c.build(settle, curve, a, sigma, opts)
	if err != nil || m.empty {
		return 0, err
	}
	v0, err := m.value(oas)
	if err != nil {
		return math.NaN(), err
	}
	v1, err := m.value(oas + 1e-4)
	return v1 - v0, err
}

// OptionValue returns the value of the embedded options to the issuer: the
// straight bond value minus the callable value, both with the spread oas. It
// is positive for a callable bond and negative if the puts dominate.
func (c Callable) OptionValue(settle time.Time, curve term.Curve, a, sigma, oas float64, opts ...lattice.Option) (float64, error) {
	v, err := c.Value(settle, curve, a, sigma, oas, opts...)
	if err != nil {
		return math.NaN(), err
	}
	straight, err := c.Straight.PresentValue(settle, term.WithSpread(curve, oas))
	return straight - v, err
}

// YieldToWorst returns the lowest continuously compounded yield (decimal) of
// the dirty price over the cash flows to maturity and to each call date on
// or after settle (paying the call price plus accrued interest on that
// date), and the redemption date of that path. A call on the settlement
// date has no yield and is skipped; puts are ignored.
func (c Callable) YieldToWorst(dirty float64, settle time.Time) (float64, time.Time, error) {
	if err := c.Validate(); err != nil {
		return math.NaN(), time.Time{}, err
	}
	cfs, _, err := c.Straight.flows(settle)
	if err != nil {
		return math.NaN(), time.Time{}, err
	}
	best, err := cashflow.YieldCC(dirty, cfs)
	if err != nil {
		return math.NaN(), time.Time{}, err
	}
	when := maturity.Date(c.Straight.Maturity)
	s0 := maturity.Date(settle)
	for _, x := range c.Calls {
		d := maturity.Date(x.Date)
		if !d.After(s0) {
			continue
		}
		acc, err := c.Straight.Accrued(d)
		if err != nil {
			return math.NaN(), time.Time{}, err
		}
		var path []cashflow.CashFlow
		for _, cf := range cfs {
			if cf.Date.After(d) {
				break
			}
			path = append(path, cf)
		}
		// the redemption replaces the cash flows after d; a call on a
		// coupon date adds to that coupon
		if n := len(path); n > 0 && path[n-1].Date.Equal(d) {
			path[n-1].Amount += x.Price/100*c.Straight.Face + acc
		} else {
			path = append(path, cashflow.New(s0, d, x.Price/100*c.Straight.Face+acc))
		}
		y, err := cashflow.YieldCC(dirty, path)
		if err != nil {
			if errors.Is(err, cashflow.ErrNoYield) {
				continue
			}
			return math.NaN(), time.Time{}, err
		}
		if y < best {
			best, when = y, d
		}
	}
	return best, when, nil
}
