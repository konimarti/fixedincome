package mc

import (
	"fmt"
	"math"
)

// Grid is the simulation time grid t_0 = 0 < t_1 < ... < t_N = T in years.
// A grid with N steps has N+1 nodes. Construct grids with NewUniformGrid or
// NewGrid (which validate the times); a Grid must not be modified after it
// is used in a simulation.
type Grid struct {
	// Times are the node times t_0 = 0, ..., t_N = T (years)
	Times []float64
}

// NewUniformGrid returns the grid with n >= 1 equal steps from 0 to T > 0:
// t_i = T*i/n, so that t_N = T exactly.
func NewUniformGrid(T float64, n int) (*Grid, error) {
	if !(T > 0) || math.IsInf(T, 0) {
		return nil, fmt.Errorf("mc: horizon T must be positive and finite, got %v", T)
	}
	if n < 1 {
		return nil, fmt.Errorf("mc: number of time steps must be at least 1, got %d", n)
	}
	ts := make([]float64, n+1)
	for i := range ts {
		ts[i] = T * float64(i) / float64(n)
	}
	ts[n] = T
	return &Grid{Times: ts}, nil
}

// NewGrid returns a grid with the given node times (copied). The first time
// must be 0, the times must be finite and strictly increasing, and there
// must be at least two nodes (one step).
func NewGrid(times []float64) (*Grid, error) {
	if len(times) < 2 {
		return nil, fmt.Errorf("mc: a grid needs at least 2 nodes, got %d", len(times))
	}
	if times[0] != 0 {
		return nil, fmt.Errorf("mc: the first grid time must be 0, got %v", times[0])
	}
	for i := 1; i < len(times); i++ {
		if math.IsNaN(times[i]) || math.IsInf(times[i], 0) || !(times[i] > times[i-1]) {
			return nil, fmt.Errorf("mc: grid times must be finite and strictly increasing (t[%d]=%v, t[%d]=%v)", i-1, times[i-1], i, times[i])
		}
	}
	return &Grid{Times: append([]float64(nil), times...)}, nil
}

// Steps returns the number of time steps N (number of nodes minus one).
func (g *Grid) Steps() int { return len(g.Times) - 1 }

// T returns the horizon t_N in years.
func (g *Grid) T() float64 { return g.Times[len(g.Times)-1] }

// Dt returns the length t_{i+1} - t_i of step i (0 <= i < N) in years.
func (g *Grid) Dt(i int) float64 { return g.Times[i+1] - g.Times[i] }

// Index returns the index of the node at time t, allowing a relative
// tolerance of 1e-9 years, and false if t is not a grid node.
func (g *Grid) Index(t float64) (int, bool) {
	tol := 1e-9 * math.Max(1, math.Abs(t))
	lo, hi := 0, len(g.Times)-1
	for lo < hi {
		mid := (lo + hi) / 2
		if g.Times[mid] < t-tol {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if math.Abs(g.Times[lo]-t) <= tol {
		return lo, true
	}
	return -1, false
}

// validate checks a grid that may have been built as a struct literal.
func (g *Grid) validate() error {
	if g == nil {
		return fmt.Errorf("mc: nil grid")
	}
	_, err := NewGrid(g.Times)
	return err
}

// Path is one simulated scenario on a grid.
type Path struct {
	// Grid is the time grid of the path
	Grid *Grid
	// State[k][i] is the k-th state variable at node i (e.g. the short rate
	// r(t_i) as a decimal or the stock price S(t_i)); see the model
	// documentation for the meaning of the factors
	State [][]float64
	// Df[i] = exp(-∫_0^{t_i} r(s) ds) is the pathwise (stochastic or
	// deterministic) discount factor to time 0; Df[0] = 1
	Df []float64
}

// NewPath allocates a path for the grid with the given number of state
// variables.
func NewPath(g *Grid, factors int) *Path {
	n := len(g.Times)
	buf := make([]float64, (factors+1)*n)
	p := &Path{Grid: g, State: make([][]float64, factors), Df: buf[:n:n]}
	for k := range p.State {
		p.State[k] = buf[(k+1)*n : (k+2)*n : (k+2)*n]
	}
	return p
}

// Simulator generates paths of a model discretised on a fixed grid. The
// methods must be safe for concurrent use (Simulate must not modify the
// simulator).
type Simulator interface {
	// Factors returns the number of state variables of a path
	Factors() int
	// Dim returns the number of independent standard normal variates
	// needed for one path
	Dim() int
	// Simulate fills p.State and p.Df from the standard normal variates z
	// (len(z) == Dim()). Negating z yields the antithetic path.
	Simulate(z []float64, p *Path)
}

// Model is a stochastic model that can be discretised on a time grid.
// Discretize precomputes all grid-dependent coefficients (calibration to
// the initial curve on the grid, transition moments) once per simulation.
type Model interface {
	Discretize(g *Grid) (Simulator, error)
}

// Payoff values one path. Value returns the payoff discounted to time 0
// (typically using p.Df). Value is called concurrently from several
// goroutines (with different paths) and must not modify shared state.
type Payoff interface {
	Value(p *Path) float64
}

// PayoffFunc adapts a function to the Payoff interface.
type PayoffFunc func(p *Path) float64

// Value calls f(p).
func (f PayoffFunc) Value(p *Path) float64 { return f(p) }

// MultiPayoff values several quantities on the same paths (e.g. the zero
// bond prices for all grid nodes). Values writes Len() discounted values to
// out; like Payoff.Value it must be safe for concurrent use.
type MultiPayoff interface {
	Len() int
	Values(p *Path, out []float64)
}

// ZeroBonds is the MultiPayoff of the zero-coupon bonds with face value 1
// maturing at the grid nodes: out[i] = p.Df[i] (Len = number of nodes).
// Its Monte Carlo means are the model discount curve on the grid.
type ZeroBonds struct {
	Grid *Grid
}

// Len returns the number of grid nodes.
func (z ZeroBonds) Len() int { return len(z.Grid.Times) }

// Values copies the pathwise discount factors.
func (z ZeroBonds) Values(p *Path, out []float64) { copy(out, p.Df) }
