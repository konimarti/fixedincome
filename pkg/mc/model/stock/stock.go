// Package stock implements geometric Brownian motion for an equity (or
// any asset with a continuous dividend yield) under the risk-neutral
// measure,
//
//	dS/S = (r(t) - q) dt + σ dW,
//
// with a deterministic short rate r(t) given by the instantaneous forward
// rates of a discount curve, together with European, Asian and American
// option payoffs and closed-form references. r(t) is the risk-free
// (risk-neutral) rate, not an expected stock return. Rates, q and σ are
// continuously compounded decimals per year, times are in years.
package stock

import (
	"fmt"
	"math"

	"github.com/konimarti/fixedincome/pkg/instrument/option"
	"github.com/konimarti/fixedincome/pkg/mc"
	"github.com/konimarti/fixedincome/pkg/term"
)

// GBM is geometric Brownian motion with a deterministic rate curve.
type GBM struct {
	// S0 is the asset price at time 0 (> 0)
	S0 float64
	// Sigma is the volatility (decimal per square root of a year, >= 0)
	Sigma float64
	// Q is the continuous dividend yield (decimal per year)
	Q float64
	// Curve is the risk-free discount curve; the drift on a step
	// [t_i, t_{i+1}] is the forward rate ln(Z(t_i)/Z(t_{i+1}))/h - q
	Curve term.Curve
}

var _ mc.Model = (*GBM)(nil)

// New returns the GBM model with initial price s0, volatility sigma,
// dividend yield q and the risk-free curve c (e.g. term.Flat{Rate: r}).
func New(s0, sigma, q float64, c term.Curve) (*GBM, error) {
	m := &GBM{S0: s0, Sigma: sigma, Q: q, Curve: c}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return m, nil
}

// Validate checks the parameters.
func (m *GBM) Validate() error {
	if m.Curve == nil {
		return fmt.Errorf("stock: nil curve")
	}
	if !(m.S0 > 0) || math.IsInf(m.S0, 0) {
		return fmt.Errorf("stock: initial price must be positive and finite, got %v", m.S0)
	}
	if !(m.Sigma >= 0) || math.IsInf(m.Sigma, 0) {
		return fmt.Errorf("stock: volatility must be non-negative and finite, got %v", m.Sigma)
	}
	if math.IsNaN(m.Q) || math.IsInf(m.Q, 0) {
		return fmt.Errorf("stock: dividend yield is not finite: %v", m.Q)
	}
	return nil
}

type sim struct {
	s0         float64
	drift, vol []float64 // per step, log price
	df         []float64 // per node
}

// Discretize returns the exact simulator on g (one normal per step):
//
//	ln S_{i+1} = ln S_i + ln(Z(t_i)/Z(t_{i+1})) - (q + σ²/2) h_i + σ √h_i z_i,
//
// so that Df[i] S_i (with Df[i] = Z(t_i)) is a martingale on the grid
// without discretisation bias. State[0][i] = S(t_i).
func (m *GBM) Discretize(g *mc.Grid) (mc.Simulator, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	n := g.Steps()
	s := &sim{s0: m.S0, drift: make([]float64, n), vol: make([]float64, n), df: make([]float64, n+1)}
	term.DiscountGrid(m.Curve, g.Times, s.df)
	s.df[0] = 1
	for i, d := range s.df {
		if !(d > 0) || math.IsInf(d, 0) {
			return nil, fmt.Errorf("stock: curve discount factor at t=%v is not positive and finite", g.Times[i])
		}
	}
	for i := 0; i < n; i++ {
		h := g.Dt(i)
		s.drift[i] = math.Log(s.df[i]/s.df[i+1]) - (m.Q+0.5*m.Sigma*m.Sigma)*h
		s.vol[i] = m.Sigma * math.Sqrt(h)
	}
	return s, nil
}

func (s *sim) Factors() int { return 1 }
func (s *sim) Dim() int     { return len(s.drift) }

func (s *sim) Simulate(z []float64, p *mc.Path) {
	st := p.State[0]
	n := len(s.drift)
	_ = st[n]
	z = z[:n]
	copy(p.Df, s.df)
	x := s.s0
	st[0] = x
	for i := 0; i < n; i++ {
		x *= math.Exp(s.drift[i] + s.vol[i]*z[i])
		st[i+1] = x
	}
}

// Forward returns E[S(T)] = S0 e^{-qT}/Z(T).
func (m *GBM) Forward(T float64) float64 {
	return m.S0 * math.Exp(-m.Q*T) / m.Curve.Discount(T)
}

// European returns the Black-Scholes price of a European option with
// strike K and maturity T (rate r = Curve.ZeroRate(T)).
func (m *GBM) European(typ option.Type, K, T float64) (float64, error) {
	return option.European{Type: typ, S: m.S0, K: K, T: T, Vol: m.Sigma, Q: m.Q}.Price(m.Curve)
}

// TerminalControl returns the control variate X = Df[N] S(t_N) with known
// mean S0 e^{-q t_N} (for European and Asian options).
func (m *GBM) TerminalControl(g *mc.Grid) *mc.Control {
	n := g.Steps()
	return &mc.Control{
		X:    func(p *mc.Path) float64 { return p.Df[n] * p.State[0][n] },
		Mean: m.S0 * math.Exp(-m.Q*g.T()),
	}
}

// GeometricAsian returns the closed-form price of the discretely monitored
// geometric average option with payoff max(±(G - K), 0) at t_N, where
// G = (S(t_1)...S(t_N))^{1/N} over the grid nodes t_1..t_N (t_0 excluded).
// ln G is normal with mean ln S0 + (1/N) Σ_k [ln(1/Z(t_k)) - (q+σ²/2) t_k]
// and variance σ²/N² Σ_{j,k} min(t_j, t_k).
func (m *GBM) GeometricAsian(typ option.Type, K float64, g *mc.Grid) (float64, error) {
	if err := m.Validate(); err != nil {
		return math.NaN(), err
	}
	if err := typ.Validate(); err != nil {
		return math.NaN(), err
	}
	if !(K > 0) || math.IsInf(K, 0) {
		return math.NaN(), fmt.Errorf("stock: strike must be positive and finite, got %v", K)
	}
	if g == nil {
		return math.NaN(), fmt.Errorf("stock: nil grid")
	}
	if _, err := mc.NewGrid(g.Times); err != nil {
		return math.NaN(), err
	}
	n := g.Steps()
	nf := float64(n)
	mean, v := 0.0, 0.0
	for k := 1; k <= n; k++ {
		t := g.Times[k]
		mean += -math.Log(m.Curve.Discount(t)) - (m.Q+0.5*m.Sigma*m.Sigma)*t
		v += float64(2*(n-k)+1) * t
	}
	mean = math.Log(m.S0) + mean/nf
	v *= m.Sigma * m.Sigma / (nf * nf)
	df := m.Curve.Discount(g.T())
	fwd := math.Exp(mean + 0.5*v)
	if v == 0 {
		if typ == option.Call {
			return df * math.Max(fwd-K, 0), nil
		}
		return df * math.Max(K-fwd, 0), nil
	}
	sd := math.Sqrt(v)
	d2 := (mean - math.Log(K)) / sd
	d1 := d2 + sd
	if typ == option.Call {
		return df * (fwd*option.N(d1) - K*option.N(d2)), nil
	}
	return df * (K*option.N(-d2) - fwd*option.N(-d1)), nil
}

// EuropeanControl returns the discounted European payoff at t_N with
// strike K as a control variate with its Black-Scholes mean (e.g. for
// American options).
func (m *GBM) EuropeanControl(typ option.Type, K float64, g *mc.Grid) (*mc.Control, error) {
	if g == nil {
		return nil, fmt.Errorf("stock: nil grid")
	}
	if _, err := mc.NewGrid(g.Times); err != nil {
		return nil, err
	}
	mean, err := m.European(typ, K, g.T())
	if err != nil {
		return nil, err
	}
	e := European{Type: typ, Strike: K}
	return &mc.Control{X: e.Value, Mean: mean}, nil
}

// GeometricAsianControl returns the geometric Asian payoff as a control
// variate (with its closed-form mean) for the arithmetic Asian option.
func (m *GBM) GeometricAsianControl(typ option.Type, K float64, g *mc.Grid) (*mc.Control, error) {
	mean, err := m.GeometricAsian(typ, K, g)
	if err != nil {
		return nil, err
	}
	a := Asian{Type: typ, Strike: K, Geometric: true}
	return &mc.Control{X: a.Value, Mean: mean}, nil
}

func intrinsic(typ option.Type, s, k float64) float64 {
	if typ == option.Put {
		return math.Max(k-s, 0)
	}
	return math.Max(s-k, 0)
}

// European is the payoff of a European option exercised at the last grid
// node t_N (discounted with Df[N]).
type European struct {
	Type   option.Type
	Strike float64
}

// Value returns Df[N] max(±(S(t_N) - K), 0).
func (e European) Value(p *mc.Path) float64 {
	n := len(p.Df) - 1
	return p.Df[n] * intrinsic(e.Type, p.State[0][n], e.Strike)
}

// Asian is the payoff of a discretely monitored average price option paid
// at t_N. The average is taken over the grid nodes t_1..t_N (t_0 excluded),
// arithmetic or geometric.
type Asian struct {
	Type      option.Type
	Strike    float64
	Geometric bool
}

// Value returns Df[N] max(±(A - K), 0) for the average A.
func (a Asian) Value(p *mc.Path) float64 {
	s := p.State[0]
	n := len(s) - 1
	avg := 0.0
	if a.Geometric {
		for i := 1; i <= n; i++ {
			avg += math.Log(s[i])
		}
		avg = math.Exp(avg / float64(n))
	} else {
		for i := 1; i <= n; i++ {
			avg += s[i]
		}
		avg /= float64(n)
	}
	return p.Df[n] * intrinsic(a.Type, avg, a.Strike)
}

// American is an option that can be exercised at every grid node t_1..t_N
// (Bermudan on the grid; refine the grid to approximate continuous
// exercise). It implements mc.Exercisable with the monomial basis
// 1, x, ..., x^Degree in the moneyness x = S/K.
//
// If Model is set (recommended; GBM.American does it), the value E_i of
// the European option on the remaining life [t_i, t_N] (Black-Scholes with
// the forward rate Df[i]/Df[N] of the path and the dividend yield of the
// model) is added as a regression basis function (E_i/K) and used as lower
// bound of the continuation value (mc.ContinuationBounder): the option is
// only exercised if the intrinsic value is at least E_i. This makes the
// exercise rule never worse than holding to maturity (e.g. a call without
// dividends is never exercised early) and improves the regression. Model
// must be the model that simulates the paths.
type American struct {
	Type   option.Type
	Strike float64
	// Degree of the polynomial basis (0: 3)
	Degree int
	// Model provides the European value of the remaining option (optional)
	Model *GBM
}

var (
	_ mc.Exercisable         = American{}
	_ mc.ContinuationBounder = American{}
	_ mc.Validator           = American{}
)

// American returns the American (Bermudan on the grid) option with strike K
// and the European lower bound and basis function of this model.
func (m *GBM) American(typ option.Type, K float64) American {
	return American{Type: typ, Strike: K, Model: m}
}

// Validate checks the option type, the strike (> 0), the degree (0..10)
// and the model.
func (a American) Validate() error {
	if err := a.Type.Validate(); err != nil {
		return err
	}
	if !(a.Strike > 0) || math.IsInf(a.Strike, 0) {
		return fmt.Errorf("stock: strike must be positive and finite, got %v", a.Strike)
	}
	if a.Degree < 0 || a.Degree > 10 {
		return fmt.Errorf("stock: basis degree must be in 0..10, got %d", a.Degree)
	}
	if a.Model != nil {
		return a.Model.Validate()
	}
	return nil
}

// CanExercise returns true for all nodes.
func (a American) CanExercise(i int) bool { return true }

// Exercise returns the intrinsic value at node i.
func (a American) Exercise(p *mc.Path, i int) float64 {
	return intrinsic(a.Type, p.State[0][i], a.Strike)
}

func (a American) degree() int {
	if a.Degree <= 0 {
		return 3
	}
	return a.Degree
}

// NumBasis returns Degree+1, plus one with a Model.
func (a American) NumBasis() int {
	if a.Model != nil {
		return a.degree() + 2
	}
	return a.degree() + 1
}

// Basis writes 1, x, ..., x^Degree with x = S(t_i)/K and, with a Model,
// E_i/K.
func (a American) Basis(p *mc.Path, i int, out []float64) {
	x := p.State[0][i] / a.Strike
	v := 1.0
	n := a.degree() + 1
	for k := 0; k < n; k++ {
		out[k] = v
		v *= x
	}
	if a.Model != nil {
		out[n] = a.european(p, i) / a.Strike
	}
}

// ContinuationBound returns the European value of the remaining option
// (0 without Model).
func (a American) ContinuationBound(p *mc.Path, i int) float64 {
	if a.Model == nil {
		return 0
	}
	return a.european(p, i)
}

// european returns the Black-Scholes value at t_i of the European option
// maturing at t_N, with the discount factor Df[N]/Df[i] of the path.
func (a American) european(p *mc.Path, i int) float64 {
	n := len(p.Df) - 1
	s := p.State[0][i]
	tau := p.Grid.Times[n] - p.Grid.Times[i]
	if tau <= 0 {
		return intrinsic(a.Type, s, a.Strike)
	}
	disc := p.Df[n] / p.Df[i]
	fwd := s * math.Exp(-a.Model.Q*tau) / disc
	sd := a.Model.Sigma * math.Sqrt(tau)
	if sd == 0 {
		return disc * intrinsic(a.Type, fwd, a.Strike)
	}
	d1 := (math.Log(fwd/a.Strike) + 0.5*sd*sd) / sd
	d2 := d1 - sd
	if a.Type == option.Put {
		return disc * (a.Strike*option.N(-d2) - fwd*option.N(-d1))
	}
	return disc * (fwd*option.N(d1) - a.Strike*option.N(d2))
}
