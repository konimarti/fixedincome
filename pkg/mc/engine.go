// Package mc implements a parallel, seedable Monte Carlo engine for
// path-dependent valuation with stochastic models (short-rate models,
// geometric Brownian motion).
//
// A Model is discretised on a Grid (nodes t_0 = 0 < ... < t_N = T, N+1
// nodes) into a Simulator, which turns a vector of standard normal variates
// into a Path: the state variables on all nodes and the pathwise discount
// factors Df[i] = exp(-∫_0^{t_i} r ds). A Payoff values a path (discounted
// to time 0). Price estimates the expected payoff:
//
//	m, _ := holee.New(curve, 0.01)
//	g, _ := mc.NewUniformGrid(5, 60)
//	res, _ := mc.Price(m, mc.PayoffFunc(func(p *mc.Path) float64 {
//		return 100 * p.Df[len(p.Df)-1]
//	}), g, mc.Options{Paths: 100000, Seed: 1})
//	lo, hi := res.CI95()
//
// Conventions: times in years, rates and volatilities as decimals,
// continuously compounded.
//
// Reproducibility: the samples are split into blocks of BlockSize samples;
// block b uses its own random stream derived from (Options.Seed, b). The
// blocks are distributed over the workers and the block statistics are
// merged in block order, so the result for a given Seed is bitwise
// identical for every number of workers.
//
// Memory: the block statistics are merged as soon as all earlier blocks are
// done (ordered streaming merge with a reorder window of 2×Workers blocks),
// so the memory use is O(Workers × payoff components) and independent of
// the number of paths; no samples are stored (except the training paths of
// PriceAmerican).
//
// Statistics: means and (co)variances are accumulated with Welford's
// algorithm per block and merged with Chan's formula; the standard error
// uses the unbiased sample variance (n-1). With antithetic variates the
// independent sample is the average of a path and its antithetic path.
//
// Control variates: for a control X with known mean μ the estimator is
// Ȳ - β(X̄ - μ). β = Cov(Y,X)/Var(X) is estimated on a pilot sample, the
// first P = min(256, samples/10) samples, which are excluded from the
// estimate. With β independent of the remaining samples the estimator is
// unbiased and its standard error (sample variance of Y - βX, n-1) is
// honest also for small samples; at least MinControlSamples samples are
// required.
//
// Panics in Simulate, payoffs, controls or exercise functions are recovered
// and returned as errors (ErrPanic).
package mc

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
)

// BlockSize is the number of independent samples per random stream.
const BlockSize = 1024

// MinControlSamples is the minimum number of independent samples with a
// control variate.
const MinControlSamples = 30

// maxPilot is the maximum pilot sample size for the control variate
// coefficient.
const maxPilot = 256

// streamTraining is the first stream number used for the training paths of
// PriceAmerican (independent from the pricing streams 0, 1, ...).
const streamTraining = 1 << 40

// Control is a control variate: a pathwise quantity X with a known
// expectation Mean (e.g. the discounted terminal stock price with mean
// S0*exp(-qT), or a discounted zero bond with mean Z(0,T)).
type Control struct {
	// X returns the (discounted) control value of a path; it must be safe
	// for concurrent use
	X func(p *Path) float64
	// Mean is the exact expectation of X
	Mean float64
}

// Options configures a simulation.
type Options struct {
	// Paths is the number of simulated paths (> 0; even with Antithetic)
	Paths int
	// Workers is the number of goroutines (0: runtime.GOMAXPROCS(0)). The
	// result does not depend on Workers.
	Workers int
	// Seed selects the random streams; the same Seed gives the same result
	Seed uint64
	// Antithetic simulates each normal vector z together with -z and uses
	// the pair average as one sample (Paths/2 samples)
	Antithetic bool
	// Control is an optional control variate (see the package
	// documentation: pilot sample for β, at least MinControlSamples
	// samples)
	Control *Control
	// TrainingPaths is the number of regression paths for PriceAmerican
	// (0: Paths); ignored by Price
	TrainingPaths int
}

// Result is a Monte Carlo estimate.
type Result struct {
	// Mean is the estimate
	Mean float64
	// StdErr is the standard error of Mean
	StdErr float64
	// N is the number of independent samples in the estimate (pairs with
	// antithetic variates; without the control variate pilot samples)
	N int
	// Paths is the number of simulated paths (including pilot samples)
	Paths int
	// Beta is the control variate coefficient estimated on the pilot
	// sample (0 without control variate)
	Beta float64
}

// CI95 returns the 95% confidence interval Mean ± 1.96 StdErr.
func (r Result) CI95() (lo, hi float64) {
	const z = 1.959963984540054
	return r.Mean - z*r.StdErr, r.Mean + z*r.StdErr
}

var (
	// ErrNonFinite is returned if a payoff or control returned NaN or ±Inf.
	ErrNonFinite = errors.New("mc: non-finite payoff or control value")
	// ErrPanic wraps a panic recovered from a simulator, payoff, control
	// or exercise function.
	ErrPanic = errors.New("mc: panic in simulation")
)

// moments are the running moments of (Y, X) for one payoff component.
type moments struct {
	n             float64
	my, mx        float64
	cyy, cxx, cxy float64
	nonFinite     bool
}

func (m *moments) add(y, x float64) {
	if math.IsNaN(y) || math.IsInf(y, 0) || math.IsNaN(x) || math.IsInf(x, 0) {
		m.nonFinite = true
		return
	}
	m.n++
	dy := y - m.my
	m.my += dy / m.n
	dx := x - m.mx
	m.mx += dx / m.n
	ry := y - m.my
	m.cyy += dy * ry
	m.cxx += dx * (x - m.mx)
	m.cxy += dx * ry
}

func (m *moments) merge(o *moments) {
	m.nonFinite = m.nonFinite || o.nonFinite
	if o.n == 0 {
		return
	}
	if m.n == 0 {
		nf := m.nonFinite
		*m = *o
		m.nonFinite = nf
		return
	}
	n := m.n + o.n
	dy := o.my - m.my
	dx := o.mx - m.mx
	f := m.n * o.n / n
	m.my += dy * o.n / n
	m.mx += dx * o.n / n
	m.cyy += o.cyy + dy*dy*f
	m.cxx += o.cxx + dx*dx*f
	m.cxy += o.cxy + dx*dy*f
	m.n = n
}

// result returns the estimate; pilot holds the pilot moments for the
// control variate coefficient (nil without control variate).
func (m *moments) result(cv *Control, pilot *moments, paths int) (Result, error) {
	if m.nonFinite || (pilot != nil && pilot.nonFinite) {
		return Result{Mean: math.NaN(), StdErr: math.NaN(), Paths: paths}, ErrNonFinite
	}
	n := m.n
	res := Result{N: int(n), Paths: paths}
	if cv == nil {
		res.Mean = m.my
		res.StdErr = math.Sqrt(m.cyy / (n - 1) / n)
		return res, nil
	}
	if pilot.cxx > 0 {
		res.Beta = pilot.cxy / pilot.cxx
	}
	b := res.Beta
	res.Mean = m.my - b*(m.mx-cv.Mean)
	v := m.cyy - 2*b*m.cxy + b*b*m.cxx // Σ (Y-βX - mean)²
	if v < 0 {
		v = 0
	}
	res.StdErr = math.Sqrt(v / (n - 1) / n)
	return res, nil
}

// evalFunc writes the discounted payoff values of path p to out, using a
// per-worker scratch buffer.
type evalFunc func(scratch []float64, p *Path, out []float64)

// job is a validated simulation.
type job struct {
	sim        Simulator
	g          *Grid
	k          int // payoff components
	nscratch   int
	eval       evalFunc
	o          Options
	streamBase uint64

	samples, nblocks, npilot int
	next                     int64 // next block to claim
	stop                     int32 // set after an error
	errMu                    sync.Mutex
	err                      error
	pilot                    []moments
	merge                    merger
}

// pilotSize returns the number of pilot samples for the control variate.
func pilotSize(samples int) int {
	p := samples / 10
	if p > maxPilot {
		p = maxPilot
	}
	return p
}

// checkOptions validates o and returns the number of independent samples.
func checkOptions(o *Options) (samples int, err error) {
	if o.Paths <= 0 {
		return 0, fmt.Errorf("mc: number of paths must be positive, got %d", o.Paths)
	}
	if o.Workers < 0 {
		return 0, fmt.Errorf("mc: number of workers must not be negative, got %d", o.Workers)
	}
	if o.TrainingPaths < 0 {
		return 0, fmt.Errorf("mc: number of training paths must not be negative, got %d", o.TrainingPaths)
	}
	samples = o.Paths
	if o.Antithetic {
		if o.Paths%2 != 0 {
			return 0, fmt.Errorf("mc: number of paths must be even with antithetic variates, got %d", o.Paths)
		}
		samples = o.Paths / 2
	}
	min := 2
	if o.Control != nil {
		if o.Control.X == nil {
			return 0, fmt.Errorf("mc: control variate without function X")
		}
		if math.IsNaN(o.Control.Mean) || math.IsInf(o.Control.Mean, 0) {
			return 0, fmt.Errorf("mc: control variate mean is not finite: %v", o.Control.Mean)
		}
		min = MinControlSamples
	}
	if samples < min {
		return 0, fmt.Errorf("mc: at least %d independent samples are needed, got %d", min, samples)
	}
	return samples, nil
}

func workers(o Options, blocks int) int {
	w := o.Workers
	if w == 0 {
		w = runtime.GOMAXPROCS(0)
	}
	if w > blocks {
		w = blocks
	}
	return w
}

func discretize(m Model, g *Grid) (sim Simulator, err error) {
	if m == nil {
		return nil, fmt.Errorf("mc: nil model")
	}
	if err := g.validate(); err != nil {
		return nil, err
	}
	err = safely(func() error {
		var e error
		sim, e = m.Discretize(g)
		return e
	})
	if err != nil {
		return nil, err
	}
	if sim.Factors() < 0 || sim.Dim() < 0 {
		return nil, fmt.Errorf("mc: invalid simulator dimensions (factors %d, dim %d)", sim.Factors(), sim.Dim())
	}
	return sim, nil
}

// safely runs f and converts a panic into an error wrapping ErrPanic.
func safely(f func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", ErrPanic, r)
		}
	}()
	return f()
}

func (j *job) fail(err error) {
	j.errMu.Lock()
	if j.err == nil {
		j.err = err
	}
	j.errMu.Unlock()
	atomic.StoreInt32(&j.stop, 1)
}

// merger merges the block statistics in block order. A block b may only
// be started when b < next + window, so at most window blocks are pending
// and the number of buffers is bounded by window + workers.
type merger struct {
	mu     sync.Mutex
	cond   *sync.Cond
	k      int
	next   int
	window int
	slots  [][]moments
	total  []moments
	free   [][]moments
}

func (m *merger) init(k, window int) {
	m.cond = sync.NewCond(&m.mu)
	m.k = k
	m.window = window
	m.slots = make([][]moments, window)
	m.total = make([]moments, k)
}

// acquire waits until block b may start and returns a zeroed buffer.
func (m *merger) acquire(b int) []moments {
	m.mu.Lock()
	defer m.mu.Unlock()
	for b >= m.next+m.window {
		m.cond.Wait()
	}
	var buf []moments
	if n := len(m.free); n > 0 {
		buf = m.free[n-1]
		m.free = m.free[:n-1]
		for i := range buf {
			buf[i] = moments{}
		}
	} else {
		buf = make([]moments, m.k)
	}
	return buf
}

// deposit stores the statistics of block b and merges all consecutive
// finished blocks.
func (m *merger) deposit(b int, buf []moments) {
	m.mu.Lock()
	m.slots[b%m.window] = buf
	for {
		s := m.next % m.window
		ready := m.slots[s]
		if ready == nil {
			break
		}
		for c := range m.total {
			m.total[c].merge(&ready[c])
		}
		m.slots[s] = nil
		m.free = append(m.free, ready)
		m.next++
	}
	m.cond.Broadcast()
	m.mu.Unlock()
}

// run executes the simulation and returns one result per payoff component.
func (j *job) run() ([]Result, error) {
	samples, err := checkOptions(&j.o)
	if err != nil {
		return nil, err
	}
	j.samples = samples
	j.nblocks = (samples + BlockSize - 1) / BlockSize
	if j.o.Control != nil {
		j.npilot = pilotSize(samples)
		j.pilot = make([]moments, j.k)
	}
	nw := workers(j.o, j.nblocks)
	j.merge.init(j.k, 2*nw)
	var wg sync.WaitGroup
	wg.Add(nw)
	for w := 0; w < nw; w++ {
		go func() {
			defer wg.Done()
			j.work()
		}()
	}
	wg.Wait()
	if j.err != nil {
		return nil, j.err
	}

	res := make([]Result, j.k)
	for c := range j.merge.total {
		var pilot *moments
		if j.pilot != nil {
			pilot = &j.pilot[c]
		}
		if res[c], err = j.merge.total[c].result(j.o.Control, pilot, j.o.Paths); err != nil {
			return res, err
		}
	}
	return res, nil
}

// work processes blocks until none are left. All buffers are allocated
// once per worker (block statistics buffers are recycled by the merger),
// so the steady state does not allocate.
func (j *job) work() {
	cur := -1
	var local []moments
	defer func() {
		if r := recover(); r != nil {
			j.fail(fmt.Errorf("%w: %v", ErrPanic, r))
			if cur >= 0 {
				// keep the merge going so that no worker waits forever
				j.merge.deposit(cur, local)
			}
		}
	}()
	src := &xoshiro{}
	rng := rand.New(src)
	z := make([]float64, j.sim.Dim())
	p1 := NewPath(j.g, j.sim.Factors())
	var p2 *Path
	y1 := make([]float64, j.k)
	var y2 []float64
	if j.o.Antithetic {
		p2 = NewPath(j.g, j.sim.Factors())
		y2 = make([]float64, j.k)
	}
	scratch := make([]float64, j.nscratch)
	cv := j.o.Control
	for atomic.LoadInt32(&j.stop) == 0 {
		b := int(atomic.AddInt64(&j.next, 1) - 1)
		if b >= j.nblocks {
			return
		}
		local = j.merge.acquire(b)
		cur = b
		src.reseed(j.o.Seed, j.streamBase+uint64(b))
		end := (b + 1) * BlockSize
		if end > j.samples {
			end = j.samples
		}
		for s := b * BlockSize; s < end; s++ {
			for i := range z {
				z[i] = rng.NormFloat64()
			}
			j.sim.Simulate(z, p1)
			j.eval(scratch, p1, y1)
			var x float64
			if cv != nil {
				x = cv.X(p1)
			}
			if j.o.Antithetic {
				for i := range z {
					z[i] = -z[i]
				}
				j.sim.Simulate(z, p2)
				j.eval(scratch, p2, y2)
				for c := range y1 {
					y1[c] = 0.5 * (y1[c] + y2[c])
				}
				if cv != nil {
					x = 0.5 * (x + cv.X(p2))
				}
			}
			acc := local
			if s < j.npilot {
				acc = j.pilot // only block 0 contains pilot samples
			}
			for c := range acc {
				acc[c].add(y1[c], x)
			}
		}
		cur = -1
		j.merge.deposit(b, local)
	}
}

// Price returns the Monte Carlo estimate of E[pay.Value(path)] for the
// model m discretised on the grid g.
func Price(m Model, pay Payoff, g *Grid, o Options) (Result, error) {
	if pay == nil {
		return Result{}, fmt.Errorf("mc: nil payoff")
	}
	sim, err := discretize(m, g)
	if err != nil {
		return Result{}, err
	}
	j := &job{sim: sim, g: g, k: 1, o: o, eval: func(_ []float64, p *Path, out []float64) {
		out[0] = pay.Value(p)
	}}
	res, err := j.run()
	if res == nil {
		return Result{}, err
	}
	return res[0], err
}

// PriceMulti estimates the expectations of all components of pay on the
// same paths (one Result per component). A control variate is applied to
// every component with its own coefficient.
func PriceMulti(m Model, pay MultiPayoff, g *Grid, o Options) ([]Result, error) {
	if pay == nil {
		return nil, fmt.Errorf("mc: nil payoff")
	}
	switch zb := pay.(type) {
	case ZeroBonds:
		if zb.Grid != g {
			return nil, fmt.Errorf("mc: ZeroBonds.Grid is not the simulation grid")
		}
	case *ZeroBonds:
		if zb == nil || zb.Grid != g {
			return nil, fmt.Errorf("mc: ZeroBonds.Grid is not the simulation grid")
		}
	}
	if pay.Len() < 1 {
		return nil, fmt.Errorf("mc: payoff has no components")
	}
	sim, err := discretize(m, g)
	if err != nil {
		return nil, err
	}
	j := &job{sim: sim, g: g, k: pay.Len(), o: o, eval: func(_ []float64, p *Path, out []float64) {
		pay.Values(p, out)
	}}
	return j.run()
}
