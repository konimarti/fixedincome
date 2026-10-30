// Package lattice implements a one-factor Hull-White trinomial lattice for
// valuing cash flows with exercise rights (callable and putable bonds).
//
// The short rate is r(t) = alpha(t) + x(t) with the zero-mean process
// dx = -a x dt + sigma dW (a >= 0 per year, sigma > 0 absolute rate units
// per square root of a year, decimals; a = 0 is the Ho-Lee model). Times are
// in years on the ACT/365 Fixed axis from the curve date.
//
// Construction (Hull-White 1994): x lives on the lattice j*dx with
// dx = sigma*sqrt(3*dtmax). A node (i, j) branches to j+1, j, j-1 (to j, j-1,
// j-2 at the upper and j+2, j+1, j at the lower edge) with probabilities that
// match the exact conditional mean and variance of x over the step, and the
// lattice is truncated at |j| <= max(1, ceil(0.184/(a*dtmax))) (no
// truncation for a = 0). alpha is fitted step by step with forward induction on the
// Arrow-Debreu prices Q(i,j), so that sum_j Q(i,j) = Z(0,t_i) holds exactly
// (to rounding) for every time t_i of the grid, including the event times.
//
// Time grid: the grid is non-uniform. It contains every requested event time
// (WithTimes) exactly; each interval between two consecutive event times is
// divided into the smallest number of equal steps with dt <= dtmax =
// 1/StepsPerYear (at least MinSteps). The spacing dx is kept the same for all
// steps, with dtmax as the reference step: for a step dt <= dtmax the exact
// mean and variance over dt are matched. The probabilities are non-negative
// because mean and variance both scale with dt, provided a*dtmax is small
// (roughly below 0.1; for very large a more steps per year are needed and
// New returns an error otherwise). Alternatives
// (one uniform dt with event dates snapped to the nearest node, or a
// per-segment dx with interpolation) would put the call dates or the zero
// bond repricing off by up to dt/2; this construction is exact at event dates
// at the price of coarser resolution (relative to the local dt) in short
// intervals, which only improves with the steps per year.
//
// Discounting over a step is exp(-(alpha_i + j*dx + s)*dt_i) with the
// short rate held constant over the step and s the optional parallel spread
// (OAS); the lattice is calibrated without s.
package lattice

import (
	"fmt"
	"math"
	"sort"

	"github.com/konimarti/fixedincome/pkg/term"
)

// Defaults of the time grid.
const (
	// DefaultStepsPerYear is the default maximum number of steps per year.
	DefaultStepsPerYear = 50
	// DefaultMinSteps is the default minimum number of steps between two
	// consecutive event times.
	DefaultMinSteps = 1
)

// timeTol is the tolerance for matching a time to a node of the grid.
const timeTol = 1e-9

// Side is the holder of an exercise right.
type Side int

const (
	// Call is a right of the issuer: the bond is redeemed at the strike
	// if the continuation value exceeds it, V = min(V, strike).
	Call Side = iota
	// Put is a right of the investor: V = max(V, strike).
	Put
)

// Flow is a deterministic payment of Amount at time T (years).
type Flow struct {
	T      float64
	Amount float64
}

// Exercise is an exercise right at time T (years) with the strike amount
// Strike (currency units, >= 0, dirty: including any accrued interest).
type Exercise struct {
	T      float64
	Strike float64
	Side   Side
}

// Option is an option of New.
type Option func(*config)

type config struct {
	stepsPerYear int
	minSteps     int
	times        []float64
}

// WithStepsPerYear sets the maximum step size dtmax = 1/n years (n >= 1).
// Default: DefaultStepsPerYear.
func WithStepsPerYear(n int) Option {
	return func(c *config) { c.stepsPerYear = n }
}

// WithMinSteps sets the minimum number of steps between two consecutive
// event times (n >= 1). Default: DefaultMinSteps.
func WithMinSteps(n int) Option {
	return func(c *config) { c.minSteps = n }
}

// WithTimes adds event times (years, in [0, horizon]) that are nodes of the
// time grid. Cash flows and exercise dates passed to Value must be at such
// times (or at multiples of the grid, see Times).
func WithTimes(ts ...float64) Option {
	return func(c *config) { c.times = append(c.times, ts...) }
}

// maxNodeSteps is the maximum number of steps times nodes per step of a
// lattice.
const maxNodeSteps = 5e8

// Tree is a calibrated Hull-White trinomial lattice. It is immutable and
// safe for concurrent use.
type Tree struct {
	a, sigma float64
	dx       float64
	dtmax    float64
	jmax     int
	t        []float64 // time grid, t[0] = 0
	alpha    []float64 // alpha[i]: short rate shift for the step t[i] -> t[i+1]
	width    []int     // width[i]: nodes j = -width[i] .. width[i] at t[i]
}

// New returns the lattice for mean reversion a >= 0 and volatility sigma > 0
// calibrated to the discount curve c up to the time horizon > 0. It returns
// an error for invalid parameters, event times outside [0, horizon] or if
// the curve is not a valid discount curve on the grid.
func New(c term.Curve, a, sigma, horizon float64, opts ...Option) (*Tree, error) {
	cfg := config{stepsPerYear: DefaultStepsPerYear, minSteps: DefaultMinSteps}
	for _, o := range opts {
		o(&cfg)
	}
	switch {
	case c == nil:
		return nil, fmt.Errorf("lattice: nil curve")
	case !(a >= 0) || math.IsInf(a, 0):
		return nil, fmt.Errorf("lattice: mean reversion must be non-negative and finite, got %v", a)
	case !(sigma > 0) || math.IsInf(sigma, 0):
		return nil, fmt.Errorf("lattice: volatility must be positive and finite, got %v", sigma)
	case !(horizon > 0) || math.IsInf(horizon, 0):
		return nil, fmt.Errorf("lattice: horizon must be positive and finite, got %v", horizon)
	case cfg.stepsPerYear < 1 || cfg.minSteps < 1:
		return nil, fmt.Errorf("lattice: steps per year (%d) and minimum steps (%d) must be at least 1", cfg.stepsPerYear, cfg.minSteps)
	}
	events := []float64{0, horizon}
	for _, t := range cfg.times {
		if !(t >= 0) || t > horizon+timeTol {
			return nil, fmt.Errorf("lattice: event time %v outside [0, %v]", t, horizon)
		}
		events = append(events, math.Min(t, horizon))
	}
	sort.Float64s(events)

	dtmax := 1 / float64(cfg.stepsPerYear)
	jmax := math.MaxInt32
	if a > 0 {
		if j := math.Ceil(0.184 / (a * dtmax)); j < math.MaxInt32 {
			jmax = int(math.Max(j, 1))
		}
	}
	steps := 0.0
	for i := 1; i < len(events); i++ {
		if d := events[i] - events[i-1]; d > timeTol {
			steps += math.Max(math.Ceil(d/dtmax-1e-9), float64(cfg.minSteps))
		}
	}
	if w := math.Min(steps, float64(jmax)); steps*(2*w+1) > maxNodeSteps {
		return nil, fmt.Errorf("lattice: %.3g steps with up to %.3g nodes each exceed the size limit; reduce the horizon or the steps per year", steps, 2*w+1)
	}
	grid := []float64{0}
	for _, e := range events[1:] {
		t0 := grid[len(grid)-1]
		if e-t0 <= timeTol {
			continue
		}
		n := int(math.Ceil((e-t0)/dtmax - 1e-9))
		if n < cfg.minSteps {
			n = cfg.minSteps
		}
		for k := 1; k < n; k++ {
			grid = append(grid, t0+(e-t0)*float64(k)/float64(n))
		}
		grid = append(grid, e)
	}

	tr := &Tree{a: a, sigma: sigma, dx: sigma * math.Sqrt(3*dtmax), dtmax: dtmax, jmax: jmax, t: grid}
	if err := tr.calibrate(c); err != nil {
		return nil, err
	}
	return tr, nil
}

// Times returns a copy of the time grid (years), starting at 0.
func (tr *Tree) Times() []float64 { return append([]float64(nil), tr.t...) }

// Steps returns the number of time steps of the lattice.
func (tr *Tree) Steps() int { return len(tr.t) - 1 }

// Horizon returns the last time of the lattice (years).
func (tr *Tree) Horizon() float64 { return tr.t[len(tr.t)-1] }

// moments returns the moments of x over a step of length dt: the factor u =
// 1-exp(-a dt) (the mean of x moves by -u*x) and the variance in units of
// dx² = 3 sigma² dtmax, in which sigma cancels (no underflow for tiny sigma).
func (tr *Tree) moments(dt float64) (u, v float64) {
	if tr.a == 0 {
		return 0, dt / (3 * tr.dtmax)
	}
	u = -math.Expm1(-tr.a * dt)
	return u, -math.Expm1(-2*tr.a*dt) / (2 * tr.a) / (3 * tr.dtmax)
}

// branch returns the middle target node k of j and the probabilities of
// moving to k+1, k, k-1.
func (tr *Tree) branch(j int, u, v float64) (k int, pu, pm, pd float64) {
	k = j
	switch {
	case j >= tr.jmax:
		k = j - 1
	case j <= -tr.jmax:
		k = j + 1
	}
	d := float64(j)*(1-u) - float64(k)
	pu = (v + d*d + d) / 2
	pd = (v + d*d - d) / 2
	pm = 1 - v - d*d
	return
}

// discounts writes exp(-j*dx*dt) for j = -w..w to g.
func (tr *Tree) discounts(w int, dt float64, g []float64) {
	q := math.Exp(-tr.dx * dt)
	g[0] = math.Exp(float64(w) * tr.dx * dt)
	for n := 1; n <= 2*w; n++ {
		g[n] = g[n-1] * q
	}
}

// calibrate fits alpha with forward induction on the Arrow-Debreu prices.
func (tr *Tree) calibrate(c term.Curve) error {
	n := len(tr.t) - 1
	tr.alpha = make([]float64, n)
	tr.width = make([]int, n+1)
	wmax := n
	if tr.jmax < wmax {
		wmax = tr.jmax
	}
	g := make([]float64, 2*wmax+1)
	q := make([]float64, 2*wmax+1)
	qn := make([]float64, 2*wmax+1)
	q[0] = 1
	for i := 0; i < n; i++ {
		w := tr.width[i]
		wn := w + 1
		if wn > tr.jmax {
			wn = tr.jmax
		}
		tr.width[i+1] = wn
		dt := tr.t[i+1] - tr.t[i]
		z := c.Discount(tr.t[i+1])
		if !(z > 0) || math.IsInf(z, 0) {
			return fmt.Errorf("lattice: invalid discount factor %v of the curve at t=%v", z, tr.t[i+1])
		}
		tr.discounts(w, dt, g)
		s := 0.0
		for m, qm := range q[:2*w+1] {
			s += qm * g[m]
		}
		tr.alpha[i] = (math.Log(s) - math.Log(z)) / dt
		df := math.Exp(-tr.alpha[i] * dt)
		for m := range qn[:2*wn+1] {
			qn[m] = 0
		}
		u, v := tr.moments(dt)
		for m, qm := range q[:2*w+1] {
			j := m - w
			k, pu, pm, pd := tr.branch(j, u, v)
			if !validProb(pu) || !validProb(pm) || !validProb(pd) {
				return fmt.Errorf("lattice: invalid branching probabilities at step %d, node %d (a=%v, sigma=%v): raise the steps per year (a*dt is too large)", i, j, tr.a, tr.sigma)
			}
			x := qm * g[m] * df
			qn[k+1+wn] += x * pu
			qn[k+wn] += x * pm
			qn[k-1+wn] += x * pd
		}
		q, qn = qn, q
	}
	return nil
}

// validProb returns true for a probability up to rounding (false for NaN).
func validProb(p float64) bool { return p >= -1e-12 && p <= 1+1e-12 }

// index returns the grid index of time t, or -1.
func (tr *Tree) index(t float64) int {
	i := sort.SearchFloat64s(tr.t, t-timeTol)
	if i < len(tr.t) && math.Abs(tr.t[i]-t) <= timeTol {
		return i
	}
	return -1
}

// Value returns the value at time 0 of the cash flows with the exercise
// rights, by backward induction, with the parallel discount spread s
// (decimal, e.g. an option-adjusted spread; the lattice itself stays
// calibrated to the curve without s).
//
// All times must be nodes of the time grid (pass them with WithTimes);
// otherwise an error is returned. A time 0 flow is added undiscounted. At
// most one exercise right per time is allowed.
//
// Coincidence convention: at a time t with a flow F and an exercise right
// with strike K, the exercise decision is taken on the value after the
// payment of F (ex-flow) and F is received in any case:
//
//	V(t) = F + min(C, K)  (call),   V(t) = F + max(C, K)  (put),
//
// where C is the discounted expected value of everything after t. For a bond
// this means a coupon paid on the call date goes to the holder, and K is the
// call price plus the accrued interest after that coupon (0 on a coupon
// date).
func (tr *Tree) Value(flows []Flow, ex []Exercise, s float64) (float64, error) {
	if math.IsNaN(s) || math.IsInf(s, 0) {
		return math.NaN(), fmt.Errorf("lattice: invalid spread %v", s)
	}
	n := len(tr.t) - 1
	f := make([]float64, n+1)
	for _, fl := range flows {
		i := tr.index(fl.T)
		if i < 0 || math.IsNaN(fl.Amount) || math.IsInf(fl.Amount, 0) {
			return math.NaN(), fmt.Errorf("lattice: invalid cash flow %+v (time is not a node of the grid or amount not finite)", fl)
		}
		f[i] += fl.Amount
	}
	type right struct {
		k    float64
		side Side
	}
	exs := make(map[int]right, len(ex))
	for _, e := range ex {
		i := tr.index(e.T)
		if i < 0 || !(e.Strike >= 0) || math.IsInf(e.Strike, 0) || (e.Side != Call && e.Side != Put) {
			return math.NaN(), fmt.Errorf("lattice: invalid exercise right %+v (time is not a node of the grid, strike negative or not finite, or unknown side)", e)
		}
		if _, dup := exs[i]; dup {
			return math.NaN(), fmt.Errorf("lattice: more than one exercise right at t=%v", e.T)
		}
		exs[i] = right{e.Strike, e.Side}
	}
	apply := func(i int, v []float64) {
		if r, ok := exs[i]; ok {
			for m := range v {
				if r.side == Call {
					v[m] = math.Min(v[m], r.k)
				} else {
					v[m] = math.Max(v[m], r.k)
				}
			}
		}
		if f[i] != 0 {
			for m := range v {
				v[m] += f[i]
			}
		}
	}

	wmax := tr.width[n]
	v := make([]float64, 2*wmax+1)
	vi := make([]float64, 2*wmax+1)
	g := make([]float64, 2*wmax+1)
	apply(n, v)
	for i := n - 1; i >= 0; i-- {
		w, wn := tr.width[i], tr.width[i+1]
		dt := tr.t[i+1] - tr.t[i]
		tr.discounts(w, dt, g)
		df := math.Exp(-(tr.alpha[i] + s) * dt)
		u, vr := tr.moments(dt)
		vi = vi[:2*w+1]
		for m := range vi {
			k, pu, pm, pd := tr.branch(m-w, u, vr)
			vi[m] = g[m] * df * (pu*v[k+1+wn] + pm*v[k+wn] + pd*v[k-1+wn])
		}
		apply(i, vi)
		v, vi = vi, v
	}
	return v[0], nil
}
