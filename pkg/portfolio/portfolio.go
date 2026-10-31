// Package portfolio computes the interest rate risk of a portfolio of bonds
// (long and short positions): market values, duration, convexity, DV01, key
// rate durations and revaluation under curve scenarios.
//
// Conventions as in the rest of the library: decimals, continuously
// compounded zero rates, ACT/365 Fixed. The spread of every position (the
// z-spread of a straight bond, the OAS of a callable bond; solved from the
// market price by the caller) is held constant in all calculations: only the
// zero curve is bumped or shocked. A position has a Factor (nominal/face; <
// 0 for a short position or a liability) that scales all amounts; durations
// and convexities are ratios and do not depend on the sign of the factor, DV01
// and key rate DV01 (currency per basis point) are negative for a long and
// positive for a short position.
package portfolio

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/konimarti/fixedincome/pkg/cashflow"
	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/lattice"
	"github.com/konimarti/fixedincome/pkg/term"
)

// DefaultKeyTenors are the key rate tenors (years): 3m, 6m, 1y, 2y, 3y, 5y,
// 7y, 10y, 15y, 20y, 30y.
var DefaultKeyTenors = []float64{0.25, 0.5, 1, 2, 3, 5, 7, 10, 15, 20, 30}

const (
	// DefaultBump is the default bump (decimal, 1bp) of the key rate
	// durations of cash flows.
	DefaultBump = 1e-4
	// DefaultCallableBump is the default bump (decimal, 10bp) of the key rate
	// durations of callable bonds. The lattice value is only piecewise smooth
	// (the exercise boundary moves between discrete nodes), but in tests the
	// sum of the key rate durations is stable for 1-10bp (within 0.5% of the
	// effective duration, which uses a parallel 50bp difference), while
	// larger bumps pick up the curvature of the value (+1% at 25bp, +4% at
	// 50bp).
	DefaultCallableBump = 10e-4
	// bp is one basis point
	bp = 1e-4
)

// Callable is the lattice valuation of a callable or putable position (one
// unit of the bond: the amounts are for the face of the Bond).
type Callable struct {
	Bond   bond.Callable
	Settle time.Time
	// MeanReversion and Vol are the parameters of the Hull-White lattice
	// (decimal, Vol > 0 absolute per year)
	MeanReversion, Vol float64
	// Options are the lattice options (e.g. lattice.WithStepsPerYear)
	Options []lattice.Option
}

// Position is a holding of a bond. The amounts of CashFlows and Accrued are
// for one unit of the bond (its face); Factor = nominal/face scales them.
type Position struct {
	ID string
	// Groups are labels for grouping (e.g. "issuer", "rating")
	Groups map[string]string
	// CashFlows are the cash flows of a straight bond; for a callable bond
	// they are ignored in favour of Callable
	CashFlows []cashflow.CashFlow
	Callable  *Callable
	// Accrued is the accrued interest at the settlement date
	Accrued float64
	// Factor is nominal/face: negative for a short position or a liability
	Factor float64
	// Spread is the z-spread of a straight bond or the OAS of a callable bond
	// over the curve (decimal), held constant
	Spread float64
}

// Config configures the calculation.
type Config struct {
	// Keys are the key rate tenors in years (strictly increasing, > 0);
	// default DefaultKeyTenors
	Keys []float64
	// Bump is the central difference bump of the key rate durations of cash
	// flows (decimal, default DefaultBump); CallableBump that of callable
	// bonds (default DefaultCallableBump)
	Bump, CallableBump float64
	// Scenarios are revalued in full
	Scenarios []Scenario
}

func (cfg Config) withDefaults() (Config, error) {
	if len(cfg.Keys) == 0 {
		cfg.Keys = DefaultKeyTenors
	}
	for i, k := range cfg.Keys {
		if !(k > 0) || math.IsInf(k, 0) || (i > 0 && !(k > cfg.Keys[i-1])) {
			return cfg, fmt.Errorf("portfolio: key tenors must be positive and strictly increasing: %v", cfg.Keys)
		}
	}
	if cfg.Bump == 0 {
		cfg.Bump = DefaultBump
	}
	if cfg.CallableBump == 0 {
		cfg.CallableBump = DefaultCallableBump
	}
	if !(cfg.Bump > 0) || !(cfg.CallableBump > 0) || math.IsInf(cfg.Bump, 0) || math.IsInf(cfg.CallableBump, 0) {
		return cfg, errors.New("portfolio: bumps must be positive")
	}
	for _, s := range cfg.Scenarios {
		if s.Shift == nil {
			return cfg, fmt.Errorf("portfolio: scenario %q has no shift", s.Name)
		}
	}
	return cfg, nil
}

// Result is the risk of one position. Amounts include the factor.
type Result struct {
	ID     string
	Groups map[string]string
	// Dirty and Clean are the market values with and without accrued interest
	Dirty, Clean, Accrued float64
	// Duration is the modified duration -1/V dV/dr (years) and Convexity
	// 1/V d²V/dr² (years²) for a parallel shift of the zero curve (effective
	// measures for a callable bond), DV01 the change of the value for +1bp
	// (full revaluation, negative for a long position)
	Duration, Convexity, DV01 float64
	// KRD are the key rate durations -1/V dV/dr_k (years) for the keys of
	// the Config; they sum to Duration. KeyDV01 are the changes of the value
	// per 1bp at each key, dV/dr_k*1bp (first order, negative for a long
	// position); they sum to -Duration*Dirty*1bp.
	KRD, KeyDV01 []float64
	// Change are the changes of the value under the scenarios of the Config
	// (full revaluation)
	Change []float64
}

// valuer values one unit of a position under a (shifted) curve.
type valuer func(c term.Curve) (float64, error)

// Evaluate computes the risk of the position on the curve (the curve
// includes any curve spread but not the Spread of the position).
func Evaluate(curve term.Curve, p Position, cfg Config) (Result, error) {
	cfg, err := cfg.withDefaults()
	if err != nil {
		return Result{}, err
	}
	if curve == nil {
		return Result{}, errors.New("portfolio: nil curve")
	}
	if math.IsNaN(p.Factor) || math.IsInf(p.Factor, 0) || p.Factor == 0 {
		return Result{}, fmt.Errorf("portfolio: position %q: invalid factor %v", p.ID, p.Factor)
	}
	r := Result{ID: p.ID, Groups: p.Groups}
	var v0, dur, cvx, dv01 float64
	var krd []float64
	var val valuer
	if c := p.Callable; c != nil {
		val = func(cv term.Curve) (float64, error) {
			return c.Bond.Value(c.Settle, cv, c.MeanReversion, c.Vol, p.Spread, c.Options...)
		}
		if v0, err = val(curve); err != nil {
			return r, err
		}
		a, s, oas := c.MeanReversion, c.Vol, p.Spread
		if dur, err = c.Bond.EffectiveDuration(c.Settle, curve, a, s, oas, c.Options...); err != nil {
			return r, err
		}
		if cvx, err = c.Bond.EffectiveConvexity(c.Settle, curve, a, s, oas, c.Options...); err != nil {
			return r, err
		}
		if dv01, err = c.Bond.DV01(c.Settle, curve, a, s, oas, c.Options...); err != nil {
			return r, err
		}
		if krd, err = latticeKRD(val, curve, cfg.Keys, cfg.CallableBump, v0); err != nil {
			return r, err
		}
	} else {
		if len(p.CashFlows) == 0 {
			return r, cashflow.ErrNoCashFlows
		}
		risk := term.WithSpread(curve, p.Spread)
		val = func(cv term.Curve) (float64, error) {
			return cashflow.PV(p.CashFlows, term.WithSpread(cv, p.Spread)), nil
		}
		v0 = cashflow.PV(p.CashFlows, risk)
		dur = cashflow.Duration(p.CashFlows, risk)
		cvx = cashflow.Convexity(p.CashFlows, risk)
		dv01 = cashflow.DV01(p.CashFlows, risk)
		if krd, err = cashflow.KeyRateDurations(p.CashFlows, risk, cfg.Keys, cfg.Bump); err != nil {
			return r, err
		}
	}
	if math.IsNaN(v0) || math.IsInf(v0, 0) {
		return r, fmt.Errorf("portfolio: position %q: invalid value %v", p.ID, v0)
	}
	f := p.Factor
	r.Dirty, r.Accrued = f*v0, f*p.Accrued
	r.Clean = r.Dirty - r.Accrued
	r.Duration, r.Convexity, r.DV01 = dur, cvx, f*dv01
	r.KRD = krd
	r.KeyDV01 = make([]float64, len(krd))
	for k := range krd {
		r.KeyDV01[k] = -krd[k] * r.Dirty * bp
	}
	r.Change = make([]float64, len(cfg.Scenarios))
	for i, s := range cfg.Scenarios {
		v, err := val(Shift(curve, s.Shift))
		if err != nil {
			return r, fmt.Errorf("scenario %s: %w", s.Name, err)
		}
		r.Change[i] = f * (v - v0)
	}
	return r, nil
}

// latticeKRD returns the key rate durations by central differences of the
// full revaluation on the curve bumped by h*w_k(t) (the hat weights of
// cashflow.KeyRateWeight) with the spread held fixed: the lattice is
// recalibrated to every bumped curve.
func latticeKRD(val valuer, curve term.Curve, keys []float64, h, v0 float64) ([]float64, error) {
	krd := make([]float64, len(keys))
	if v0 == 0 {
		return krd, nil
	}
	for k := range keys {
		k := k
		w := func(t float64) float64 { return cashflow.KeyRateWeight(keys, k, t) }
		up, err := val(Shift(curve, func(t float64) float64 { return h * w(t) }))
		if err != nil {
			return nil, err
		}
		dn, err := val(Shift(curve, func(t float64) float64 { return -h * w(t) }))
		if err != nil {
			return nil, err
		}
		krd[k] = -(up - dn) / (2 * h * v0)
	}
	return krd, nil
}

// Total is the aggregate risk of a set of positions. Duration, Convexity and
// KRD are money-weighted: sum(V_i*D_i)/sum(V_i) with the dirty values V_i
// (negative for shorts), so they are ratios to the net value and are
// meaningless if the net value is close to zero (a hedged book): then
// Degenerate is true and they are 0. DV01, KeyDV01 and Change are plain sums
// and always well defined; SumVD and SumVC (sum(V_i*D_i), sum(V_i*C_i)) too.
type Total struct {
	N    int
	Keys []float64
	// Dirty, Clean and Accrued are the net market values; a net value below
	// 1e-9 of Gross is set to 0. Gross is the sum of the absolute dirty
	// values.
	Dirty, Clean, Accrued, Gross float64
	// Degenerate is true if the net dirty value is below 1e-6 of Gross
	Degenerate                bool
	Duration, Convexity, DV01 float64
	SumVD, SumVC              float64
	KRD, KeyDV01              []float64
	Change                    []float64
}

// Aggregate sums the results of the positions rs, computed with the same
// Config (keys and scenarios).
func Aggregate(cfg Config, rs []Result) (Total, error) {
	cfg, err := cfg.withDefaults()
	if err != nil {
		return Total{}, err
	}
	t := Total{N: len(rs), Keys: cfg.Keys, KRD: make([]float64, len(cfg.Keys)),
		KeyDV01: make([]float64, len(cfg.Keys)), Change: make([]float64, len(cfg.Scenarios))}
	for _, r := range rs {
		if len(r.KRD) != len(cfg.Keys) || len(r.Change) != len(cfg.Scenarios) {
			return t, fmt.Errorf("portfolio: result %q does not match the config", r.ID)
		}
		t.Dirty += r.Dirty
		t.Clean += r.Clean
		t.Accrued += r.Accrued
		t.Gross += math.Abs(r.Dirty)
		t.DV01 += r.DV01
		t.SumVD += r.Dirty * r.Duration
		t.SumVC += r.Dirty * r.Convexity
		for k := range r.KRD {
			t.KeyDV01[k] += r.KeyDV01[k]
		}
		for i := range r.Change {
			t.Change[i] += r.Change[i]
		}
	}
	if math.Abs(t.Dirty) < 1e-9*t.Gross {
		t.Dirty = 0
	}
	if math.Abs(t.Clean) < 1e-9*t.Gross {
		t.Clean = 0
	}
	t.Degenerate = math.Abs(t.Dirty) < 1e-6*t.Gross || t.Dirty == 0
	if !t.Degenerate {
		t.Duration, t.Convexity = t.SumVD/t.Dirty, t.SumVC/t.Dirty
		for k := range t.KRD {
			t.KRD[k] = -t.KeyDV01[k] / (t.Dirty * bp)
		}
	}
	return t, nil
}

// Group is the aggregate of the positions with the same label.
type Group struct {
	Key   string
	Total Total
}

// GroupBy aggregates the results by the label Groups[label] (missing or
// empty: "unknown") in the order of first appearance.
func GroupBy(cfg Config, rs []Result, label string) ([]Group, error) {
	var order []string
	by := map[string][]Result{}
	for _, r := range rs {
		k := r.Groups[label]
		if k == "" {
			k = "unknown"
		}
		if _, ok := by[k]; !ok {
			order = append(order, k)
		}
		by[k] = append(by[k], r)
	}
	var out []Group
	for _, k := range order {
		t, err := Aggregate(cfg, by[k])
		if err != nil {
			return nil, err
		}
		out = append(out, Group{Key: k, Total: t})
	}
	return out, nil
}

// Approx returns the approximation of the change of the net value under the
// scenario s from the risk measures: for a parallel shift dr
// V*(-D*dr + C*dr²/2); otherwise the first order change sum(KeyDV01_k/1bp *
// s.Shift(key_k)) from the key rate durations (no convexity; exact for a
// shift that is piecewise linear between the keys).
func (t Total) Approx(s Scenario) float64 {
	if s.Parallel {
		return -t.SumVD*s.Size + 0.5*t.SumVC*s.Size*s.Size
	}
	sum := 0.0
	for k, key := range t.Keys {
		sum += t.KeyDV01[k] / bp * s.Shift(key)
	}
	return sum
}
