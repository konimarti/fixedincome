package mc

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
)

// Exercisable is an option with early exercise (American or Bermudan) on
// the grid nodes, priced by PriceAmerican with the Longstaff-Schwartz
// least-squares method. Its methods must be safe for concurrent use.
type Exercisable interface {
	// CanExercise reports whether exercise is allowed at node i (1 <= i <=
	// N; exercise at t_0 is not considered)
	CanExercise(i int) bool
	// Exercise returns the undiscounted cash flow from exercising at node
	// i; values <= 0 mean that exercise is not worthwhile (out of the
	// money) and such paths are excluded from the regression
	Exercise(p *Path, i int) float64
	// NumBasis returns the number of regression basis functions
	// (including the constant)
	NumBasis() int
	// Basis writes the basis functions evaluated on the state at node i to
	// out (len(out) == NumBasis())
	Basis(p *Path, i int, out []float64)
}

// ContinuationBounder is optionally implemented by an Exercisable that
// knows a lower bound of the continuation value, e.g. the value of the
// European option on the remaining life (holding to maturity is always
// possible). ContinuationBound returns this bound at node i, undiscounted
// (in units of time t_i money, like Exercise).
type ContinuationBounder interface {
	ContinuationBound(p *Path, i int) float64
}

// Validator is optionally implemented by an Exercisable; PriceAmerican
// returns the error of Validate.
type Validator interface {
	Validate() error
}

// AmericanResult is the result of PriceAmerican.
type AmericanResult struct {
	// Result is the estimate on independent pricing paths with the exercise
	// rule estimated on the training paths. It is a low-biased estimator
	// (the rule is sub-optimal) with an honest standard error.
	Result
	// InSample is the classical Longstaff-Schwartz estimate on the
	// training paths (no standard error; the bias of the in-sample
	// estimate has no definite sign)
	InSample float64
	// InSampleStdErr is the standard error of InSample (sample standard
	// deviation of the realised cash flows, n-1; no control variate)
	InSampleStdErr float64
	// TrainingPaths is the number of regression paths
	TrainingPaths int
}

// rule is the estimated exercise rule.
type rule struct {
	a     Exercisable
	lb    ContinuationBounder // nil: no bound
	beta  [][]float64         // per node, nil: no regression
	nodes int
}

// exercise reports whether to exercise with exercise value h > 0 at node
// i < N: h must be at least the regression estimate of the continuation
// value and at least the known lower bound of the continuation value.
func (r *rule) exercise(p *Path, i int, h float64, phi []float64) bool {
	if r.beta[i] == nil {
		return false
	}
	if r.lb != nil && h < r.lb.ContinuationBound(p, i) {
		return false
	}
	r.a.Basis(p, i, phi)
	return h >= dot(r.beta[i], phi)
}

// PriceAmerican prices an option with early exercise with the
// Longstaff-Schwartz method (Longstaff and Schwartz 2001):
//
//  1. TrainingPaths paths (independent random streams) are simulated and
//     stored. Going backwards from t_N, at every exercise node the
//     discounted realised cash flows of the in-the-money paths are
//     regressed on the basis functions (least squares, normal equations
//     with a tiny ridge term). Nodes with fewer in-the-money paths than
//     basis functions are not exercised.
//  2. The exercise rule (regression coefficients) is applied to Options.Paths
//     fresh paths with the regular engine (workers, antithetic variates and
//     control variates are supported) to obtain Result.
//
// Exercise rule: a path is exercised at node i if the exercise value h > 0
// is at least the estimated continuation value and, if the option
// implements ContinuationBounder, at least the known lower bound of the
// continuation value. With the European value of the remaining option as
// bound, every exercise decision is at least as good as holding the
// option to maturity, so the out-of-sample estimate is never biased below
// the European price (the regression alone can exercise a no-dividend call
// early when the continuation value is underestimated).
//
// Cash flows are discounted with the pathwise discount factors Df, so the
// method also works for stochastic rates. Exercise at t_0 is not
// considered; compare with the intrinsic value if needed. The training
// paths are stored in memory (TrainingPaths × (N+1) × (Factors+1)
// float64), are simulated without antithetic variates, and the regression
// runs on a single goroutine (deterministic summation order). If the
// option implements Validator, Validate is called first.
func PriceAmerican(m Model, a Exercisable, g *Grid, o Options) (AmericanResult, error) {
	if a == nil {
		return AmericanResult{}, fmt.Errorf("mc: nil exercisable option")
	}
	if v, ok := a.(Validator); ok {
		if err := v.Validate(); err != nil {
			return AmericanResult{}, err
		}
	}
	if _, err := checkOptions(&o); err != nil {
		return AmericanResult{}, err
	}
	sim, err := discretize(m, g)
	if err != nil {
		return AmericanResult{}, err
	}
	k := a.NumBasis()
	if k < 1 {
		return AmericanResult{}, fmt.Errorf("mc: at least one basis function is needed, got %d", k)
	}
	ntrain := o.TrainingPaths
	if ntrain == 0 {
		ntrain = o.Paths
	}
	if ntrain < 2*k {
		return AmericanResult{}, fmt.Errorf("mc: at least %d training paths are needed, got %d", 2*k, ntrain)
	}

	n := g.Steps()
	r := &rule{a: a, nodes: n}
	r.lb, _ = a.(ContinuationBounder)
	var inSample, inSampleSE float64
	err = safely(func() error {
		paths, err := simulateMany(sim, g, ntrain, o)
		if err != nil {
			return err
		}
		inSample, inSampleSE = regress(paths, r)
		return nil
	})
	if err != nil {
		return AmericanResult{}, err
	}

	eval := func(phi []float64, p *Path, out []float64) {
		for i := 1; i <= n; i++ {
			if !a.CanExercise(i) {
				continue
			}
			h := a.Exercise(p, i)
			if !(h > 0) {
				continue
			}
			if i == n || r.exercise(p, i, h, phi) {
				out[0] = p.Df[i] * h
				return
			}
		}
		out[0] = 0
	}
	j := &job{sim: sim, g: g, k: 1, nscratch: k, eval: eval, o: o}
	res, err := j.run()
	out := AmericanResult{InSample: inSample, InSampleStdErr: inSampleSE, TrainingPaths: ntrain}
	if res != nil {
		out.Result = res[0]
	}
	return out, err
}

// simulateMany simulates and stores n paths with the training streams.
func simulateMany(sim Simulator, g *Grid, n int, o Options) ([]Path, error) {
	f := sim.Factors()
	nodes := len(g.Times)
	slab := make([]float64, n*nodes*(f+1))
	paths := make([]Path, n)
	states := make([][]float64, n*f)
	for j := range paths {
		buf := slab[j*nodes*(f+1) : (j+1)*nodes*(f+1)]
		paths[j] = Path{Grid: g, Df: buf[:nodes:nodes], State: states[j*f : (j+1)*f : (j+1)*f]}
		for c := 0; c < f; c++ {
			paths[j].State[c] = buf[(c+1)*nodes : (c+2)*nodes : (c+2)*nodes]
		}
	}
	nblocks := (n + BlockSize - 1) / BlockSize
	nw := workers(o, nblocks)
	var next int64
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error
	wg.Add(nw)
	for w := 0; w < nw; w++ {
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("%w: %v", ErrPanic, r)
					}
					errMu.Unlock()
				}
			}()
			src := &xoshiro{}
			rng := rand.New(src)
			z := make([]float64, sim.Dim())
			for {
				b := int(atomic.AddInt64(&next, 1) - 1)
				if b >= nblocks {
					return
				}
				src.reseed(o.Seed, streamTraining+uint64(b))
				end := (b + 1) * BlockSize
				if end > n {
					end = n
				}
				for s := b * BlockSize; s < end; s++ {
					for i := range z {
						z[i] = rng.NormFloat64()
					}
					sim.Simulate(z, &paths[s])
				}
			}
		}()
	}
	wg.Wait()
	return paths, firstErr
}

// regress runs the backward induction on the training paths, stores the
// regression coefficients per node in r.beta (nil: no exercise) and
// returns the in-sample estimate and its standard error.
func regress(paths []Path, r *rule) (float64, float64) {
	a, n := r.a, r.nodes
	k := a.NumBasis()
	v := make([]float64, len(paths)) // realised cash flow discounted to 0
	if a.CanExercise(n) {
		for j := range paths {
			if h := a.Exercise(&paths[j], n); h > 0 {
				v[j] = paths[j].Df[n] * h
			}
		}
	}
	r.beta = make([][]float64, n+1)
	phi := make([]float64, k)
	ata := make([]float64, k*k)
	atb := make([]float64, k)
	for i := n - 1; i >= 1; i-- {
		if !a.CanExercise(i) {
			continue
		}
		for c := range ata {
			ata[c] = 0
		}
		for c := range atb {
			atb[c] = 0
		}
		itm := 0
		for j := range paths {
			p := &paths[j]
			if !(a.Exercise(p, i) > 0) {
				continue
			}
			itm++
			a.Basis(p, i, phi)
			y := v[j] / p.Df[i]
			for row := 0; row < k; row++ {
				atb[row] += phi[row] * y
				for c := 0; c <= row; c++ {
					ata[row*k+c] += phi[row] * phi[c]
				}
			}
		}
		if itm < k {
			continue
		}
		b, ok := solveSPD(ata, atb, k)
		if !ok {
			continue
		}
		r.beta[i] = b
		for j := range paths {
			p := &paths[j]
			h := a.Exercise(p, i)
			if h > 0 && r.exercise(p, i, h, phi) {
				v[j] = p.Df[i] * h
			}
		}
	}
	var m moments
	for _, x := range v {
		m.add(x, 0)
	}
	return m.my, math.Sqrt(m.cyy / (m.n - 1) / m.n)
}

func dot(a, b []float64) float64 {
	s := 0.0
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

// solveSPD solves the normal equations A x = b (lower triangle of the k×k
// matrix a, row-major) by a Cholesky decomposition with a relative ridge
// term of 1e-12 times the mean diagonal. It returns false if the matrix is
// not positive definite.
func solveSPD(a, b []float64, k int) ([]float64, bool) {
	l := make([]float64, k*k)
	tr := 0.0
	for i := 0; i < k; i++ {
		tr += a[i*k+i]
	}
	ridge := 1e-12 * tr / float64(k)
	for i := 0; i < k; i++ {
		for j := 0; j <= i; j++ {
			s := a[i*k+j]
			if i == j {
				s += ridge
			}
			for m := 0; m < j; m++ {
				s -= l[i*k+m] * l[j*k+m]
			}
			if i == j {
				if !(s > 0) {
					return nil, false
				}
				l[i*k+i] = math.Sqrt(s)
			} else {
				l[i*k+j] = s / l[j*k+j]
			}
		}
	}
	x := make([]float64, k)
	for i := 0; i < k; i++ {
		s := b[i]
		for m := 0; m < i; m++ {
			s -= l[i*k+m] * x[m]
		}
		x[i] = s / l[i*k+i]
	}
	for i := k - 1; i >= 0; i-- {
		s := x[i]
		for m := i + 1; m < k; m++ {
			s -= l[m*k+i] * x[m]
		}
		x[i] = s / l[i*k+i]
	}
	for _, xi := range x {
		if math.IsNaN(xi) || math.IsInf(xi, 0) {
			return nil, false
		}
	}
	return x, true
}
