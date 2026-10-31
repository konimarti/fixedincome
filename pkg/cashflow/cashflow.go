// Package cashflow implements generic analytics on deterministic cash flows:
// present value, yield, z-spread, duration, convexity and key rate
// durations.
//
// Conventions: times are in years from the settlement date (ACT/365 Fixed,
// see maturity.YearFraction); rates, yields and spreads are continuously
// compounded decimals (0.02 = 2%, 0.0001 = 1bp). Prices are "dirty" (full)
// prices, i.e. including accrued interest, in the currency units of the
// cash flow amounts.
package cashflow

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/konimarti/fixedincome/internal/solve"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/rate"
	"github.com/konimarti/fixedincome/pkg/term"
)

// CashFlow is a payment of Amount (currency units) on Date, T years after
// the settlement date (ACT/365 Fixed).
type CashFlow struct {
	Date   time.Time
	T      float64
	Amount float64
}

// New returns the cash flow of amount paid on date with T measured from
// settle (ACT/365 Fixed).
func New(settle, date time.Time, amount float64) CashFlow {
	return CashFlow{Date: maturity.Date(date), T: maturity.YearFraction(settle, date), Amount: amount}
}

// Instrument is a fixed income instrument with deterministic cash flows.
type Instrument interface {
	// CashFlows returns the cash flows strictly after settle in ascending
	// order (empty if the instrument has matured).
	CashFlows(settle time.Time) ([]CashFlow, error)
}

// Projected is a fixed income instrument whose cash flows depend on index
// rates projected from a forecast curve (floating-rate notes, swaps, FRAs).
//
// The analytics of this package applied to projected cash flows (PV, DV01,
// Duration, KeyRateDurations, ZSpread) measure the sensitivity to the
// discount curve with the projected cash flows, i.e. the forecast curve,
// held fixed.
type Projected interface {
	// CashFlows returns the (signed) cash flows strictly after settle in
	// ascending order, with index rates projected from the forecast curve.
	CashFlows(settle time.Time, forecast term.Curve) ([]CashFlow, error)
}

var (
	// ErrNoCashFlows is returned if there are no cash flows.
	ErrNoCashFlows = errors.New("cashflow: no cash flows")
	// ErrNoYield is returned if no yield (or spread) reprices the cash
	// flows to the given price.
	ErrNoYield = errors.New("cashflow: no yield found")
)

// PV returns the present value sum(Amount * c.Discount(T)) of the cash flows.
// It does not allocate.
func PV(cfs []CashFlow, c term.Curve) float64 {
	pv := 0.0
	for _, cf := range cfs {
		pv += cf.Amount * c.Discount(cf.T)
	}
	return pv
}

// yieldBrackets are the intervals (continuously compounded rates, decimal)
// tried successively by the bracketed fallback of YieldCC; the wide
// intervals cover distressed bonds and very short maturities.
var yieldBrackets = [][2]float64{
	{-0.2, 0.2},
	{-0.5, 1.0},
	{-1.0, 5.0},
	{-2.0, 50.0},
	{-10.0, 1e4},
}

// solveYield returns y with sum(amounts[i]*exp(-y*t[i])) = price.
func solveYield(price float64, cfs []CashFlow, amount func(i int) float64) (float64, error) {
	if len(cfs) == 0 {
		return math.NaN(), ErrNoCashFlows
	}
	if math.IsNaN(price) || math.IsInf(price, 0) {
		return math.NaN(), fmt.Errorf("cashflow: invalid price %v", price)
	}
	sum, sumT := 0.0, 0.0
	nonNegative, nonPositive := true, true
	for i, cf := range cfs {
		a := amount(i)
		if math.IsNaN(a) || math.IsInf(a, 0) || math.IsNaN(cf.T) || math.IsInf(cf.T, 0) {
			return math.NaN(), fmt.Errorf("cashflow: invalid cash flow %+v (amount %v)", cf, a)
		}
		sum += a
		sumT += a * cf.T
		nonNegative = nonNegative && a >= 0
		nonPositive = nonPositive && a <= 0
	}
	// the present value of non-negative (non-positive) amounts is positive
	// (negative) for every finite yield
	if (nonNegative && price <= 0) || (nonPositive && price >= 0) {
		return math.NaN(), fmt.Errorf("%w: price %v cannot be reached with cash flows of one sign", ErrNoYield, price)
	}
	f := func(y float64) float64 {
		pv := 0.0
		for i, cf := range cfs {
			pv += amount(i) * math.Exp(-y*cf.T)
		}
		return pv - price
	}
	fdf := func(y float64) (float64, float64) {
		pv, dpv := 0.0, 0.0
		for i, cf := range cfs {
			v := amount(i) * math.Exp(-y*cf.T)
			pv += v
			dpv -= cf.T * v
		}
		return pv - price, dpv
	}
	// initial guess from the amount-weighted average maturity
	y0 := 0.0
	if sum > 0 && price > 0 && sumT > 0 {
		y0 = math.Log(sum/price) / (sumT / sum)
	}
	if math.IsNaN(y0) || math.IsInf(y0, 0) || math.Abs(y0) > 5 {
		y0 = 0.0
	}
	y, err := solve.Newton(fdf, y0, -10.0, 1e4, 1e-14, 100)
	if err != nil {
		y, err = solve.FindRoot(f, yieldBrackets, 1e-14)
		if err != nil {
			return math.NaN(), fmt.Errorf("%w: %v", ErrNoYield, err)
		}
	}
	// reject spurious roots where the present value underflowed
	underflow := true
	for i, cf := range cfs {
		if a := amount(i); a != 0 && a*math.Exp(-y*cf.T) != 0 {
			underflow = false
			break
		}
	}
	if underflow {
		return math.NaN(), fmt.Errorf("%w: present value underflows at yield %v", ErrNoYield, y)
	}
	return y, nil
}

// YieldCC returns the continuously compounded yield (decimal) y such that the
// cash flows discounted with exp(-y*T) (T on the ACT/365F axis) sum to the
// dirty price. It uses Newton's method with the analytic derivative and
// falls back to Brent's method on successively wider brackets (up to
// [-1000%, 1e6%]). It returns ErrNoCashFlows if there are no cash flows,
// an error wrapping ErrNoYield if no yield is found (e.g. a non-positive
// price for non-negative cash flows) and an error for non-finite inputs.
func YieldCC(dirtyPrice float64, cfs []CashFlow) (float64, error) {
	return solveYield(dirtyPrice, cfs, func(i int) float64 { return cfs[i].Amount })
}

// Yield returns the yield (decimal) compounded n times per year (n=1:
// annual), i.e. the n-compounded equivalent n*(exp(y/n)-1) of the
// continuously compounded ACT/365F yield y = YieldCC. Note that this is not
// the ICMA "street" yield to maturity, which discounts with
// (1+y/n)^(-k-fraction) on the coupon period grid; the two differ slightly
// for broken periods and non-ACT/365F conventions.
func Yield(dirtyPrice float64, cfs []CashFlow, n int) (float64, error) {
	if n <= 0 {
		return math.NaN(), fmt.Errorf("cashflow: invalid compounding frequency %d", n)
	}
	y, err := YieldCC(dirtyPrice, cfs)
	if err != nil {
		return math.NaN(), err
	}
	return rate.FromContinuous(y, n)
}

// ZSpread returns the static (zero-volatility) spread s (continuously
// compounded, decimal) such that PV(cfs, term.WithSpread(c, s)) equals the
// dirty price. The curve is not modified. Errors are returned as for YieldCC
// and if the curve returns a non-finite or non-positive discount factor.
func ZSpread(dirtyPrice float64, cfs []CashFlow, c term.Curve) (float64, error) {
	discounted := make([]float64, len(cfs))
	for i, cf := range cfs {
		z := c.Discount(cf.T)
		if !(z > 0) || math.IsInf(z, 0) {
			return math.NaN(), fmt.Errorf("cashflow: invalid discount factor %v of the curve at t=%v", z, cf.T)
		}
		discounted[i] = cf.Amount * z
	}
	return solveYield(dirtyPrice, cfs, func(i int) float64 { return discounted[i] })
}

// Duration returns the (positive) modified duration -1/P dP/dy for a
// parallel shift y of the continuously compounded zero curve, which equals
// the Macaulay duration sum(T*Amount*Z(T))/P in years. It returns 0 if the
// present value P is 0.
func Duration(cfs []CashFlow, c term.Curve) float64 {
	pv, d := 0.0, 0.0
	for _, cf := range cfs {
		v := cf.Amount * c.Discount(cf.T)
		pv += v
		d += cf.T * v
	}
	if pv == 0 {
		return 0
	}
	return d / pv
}

// Convexity returns the convexity 1/P d²P/dy² = sum(T²*Amount*Z(T))/P for a
// parallel shift y of the continuously compounded zero curve, in years². It
// returns 0 if the present value P is 0.
func Convexity(cfs []CashFlow, c term.Curve) float64 {
	pv, cx := 0.0, 0.0
	for _, cf := range cfs {
		v := cf.Amount * c.Discount(cf.T)
		pv += v
		cx += cf.T * cf.T * v
	}
	if pv == 0 {
		return 0
	}
	return cx / pv
}

// DV01 returns the change in present value for a parallel increase of the
// continuously compounded zero curve by one basis point, PV(c+1bp) - PV(c),
// obtained by full revaluation. It is NEGATIVE for a long position in a bond.
func DV01(cfs []CashFlow, c term.Curve) float64 {
	return PV(cfs, term.WithSpread(c, 1e-4)) - PV(cfs, c)
}

// Sensitivity returns the second-order approximation of the relative change
// in present value dP/P = -D*dr + 1/2*C*dr² for a parallel shift dr
// (decimal) of the zero curve, with D = Duration and C = Convexity.
func Sensitivity(cfs []CashFlow, c term.Curve, dr float64) float64 {
	return -Duration(cfs, c)*dr + 0.5*Convexity(cfs, c)*dr*dr
}

// KeyRateDurations returns the key rate durations -1/P dP/dr_k for the key
// tenors keys (years, strictly increasing). The zero curve is bumped by a
// triangular function that is 1 at keys[k] and decreases linearly to 0 at
// the neighbouring keys; the first (last) bump is flat below (above) its
// key, so that the bumps sum to a parallel shift and the key rate durations
// sum to Duration. The derivatives are computed with central differences of
// size bump (decimal, e.g. 1e-4). It returns ErrNoCashFlows if there are no
// cash flows and an error for invalid keys or bump.
func KeyRateDurations(cfs []CashFlow, c term.Curve, keys []float64, bump float64) ([]float64, error) {
	if len(cfs) == 0 {
		return nil, ErrNoCashFlows
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("cashflow: no key tenors")
	}
	for i, k := range keys {
		if math.IsNaN(k) || math.IsInf(k, 0) || (i > 0 && !(k > keys[i-1])) {
			return nil, fmt.Errorf("cashflow: key tenors must be finite and strictly increasing: %v", keys)
		}
	}
	if !(bump > 0) || math.IsInf(bump, 0) {
		return nil, fmt.Errorf("cashflow: invalid bump %v", bump)
	}
	pv := PV(cfs, c)
	krd := make([]float64, len(keys))
	if pv == 0 {
		return krd, nil
	}
	for k := range keys {
		up := PV(cfs, keyRateCurve{base: c, keys: keys, k: k, h: bump})
		down := PV(cfs, keyRateCurve{base: c, keys: keys, k: k, h: -bump})
		krd[k] = -(up - down) / (2 * bump * pv)
	}
	return krd, nil
}

// KeyRateWeight returns the weight w_k(t) of the triangular bump of the
// key tenor keys[k] used by KeyRateDurations: 1 at keys[k], linear down to 0
// at the neighbouring keys, flat beyond the first and the last key. The
// weights of all keys sum to 1 for every t. keys must be valid as for
// KeyRateDurations and 0 <= k < len(keys).
func KeyRateWeight(keys []float64, k int, t float64) float64 {
	return keyRateCurve{keys: keys, k: k}.weight(t)
}

// keyRateCurve is the base curve with the zero rate bumped by h*w_k(t).
type keyRateCurve struct {
	base term.Curve
	keys []float64
	k    int
	h    float64
}

// weight returns the triangular weight w_k(t).
func (c keyRateCurve) weight(t float64) float64 {
	keys, k := c.keys, c.k
	switch {
	case t <= keys[k]:
		if k == 0 {
			return 1
		}
		if t <= keys[k-1] {
			return 0
		}
		return (t - keys[k-1]) / (keys[k] - keys[k-1])
	default:
		if k == len(keys)-1 {
			return 1
		}
		if t >= keys[k+1] {
			return 0
		}
		return (keys[k+1] - t) / (keys[k+1] - keys[k])
	}
}

// Discount returns the bumped discount factor.
func (c keyRateCurve) Discount(t float64) float64 {
	return c.base.Discount(t) * math.Exp(-c.h*c.weight(t)*t)
}

// ZeroRate returns the bumped zero rate.
func (c keyRateCurve) ZeroRate(t float64) float64 {
	return c.base.ZeroRate(t) + c.h*c.weight(t)
}
