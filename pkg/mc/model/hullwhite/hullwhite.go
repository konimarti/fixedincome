// Package hullwhite implements the one-factor Hull-White (extended
// Vasicek) short rate model
//
//	dr = (θ(t) - a r) dt + σ dW
//
// under the risk-neutral measure, with θ(t) chosen such that the model
// reproduces the initial discount curve exactly. Rates and σ are decimals
// (σ in absolute rate units per square root of a year, e.g. 0.01 = 100bp),
// a is the mean reversion speed per year (a = 0 is the Ho-Lee model),
// times are in years.
//
// The model is written as r(t) = x(t) + φ(t) with the zero-mean OU process
// dx = -a x dt + σ dW, x(0) = 0, and
//
//	φ(t) = f(0,t) + σ²/2 B(a,t)²,   B(a,t) = (1-e^{-at})/a,
//	∫_0^t φ ds = -ln Z(0,t) + V(t)/2, V(t) = Var(∫_0^t x ds),
//
// where Z(0,t) is the initial curve and f(0,t) its instantaneous forward
// rate. Simulation is exact (see Discretize): E[Df(t_i)] = Z(0,t_i) holds
// on every grid without discretisation bias.
package hullwhite

import (
	"fmt"
	"math"

	"github.com/konimarti/fixedincome/pkg/mc"
	"github.com/konimarti/fixedincome/pkg/mc/model/internal/ou"
	"github.com/konimarti/fixedincome/pkg/term"
)

// HullWhite is the Hull-White model fitted to an initial curve. Create it
// with New; the value must not be modified afterwards.
type HullWhite struct {
	// Curve is the initial discount curve the model reproduces
	Curve term.Curve
	// A is the mean reversion speed (per year, >= 0)
	A float64
	// Sigma is the short rate volatility (decimal per square root of a
	// year, >= 0)
	Sigma float64
}

var _ mc.Model = (*HullWhite)(nil)

// New returns the Hull-White model with mean reversion a >= 0 and
// volatility sigma >= 0 fitted to the curve c.
func New(c term.Curve, a, sigma float64) (*HullWhite, error) {
	m := &HullWhite{Curve: c, A: a, Sigma: sigma}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return m, nil
}

// Validate checks the parameters.
func (m *HullWhite) Validate() error {
	if m.Curve == nil {
		return fmt.Errorf("hullwhite: nil curve")
	}
	if !(m.A >= 0) || math.IsInf(m.A, 0) {
		return fmt.Errorf("hullwhite: mean reversion must be non-negative and finite, got %v", m.A)
	}
	if !(m.Sigma >= 0) || math.IsInf(m.Sigma, 0) {
		return fmt.Errorf("hullwhite: volatility must be non-negative and finite, got %v", m.Sigma)
	}
	return nil
}

// R0 returns the initial short rate r(0) = f(0,0) (decimal).
func (m *HullWhite) R0() float64 { return term.InstForward(m.Curve, 0) }

// Phi returns the deterministic shift φ(t) = E[r(t)] of the short rate.
func (m *HullWhite) Phi(t float64) float64 {
	b := ou.B(m.A, t)
	return term.InstForward(m.Curve, t) + 0.5*m.Sigma*m.Sigma*b*b
}

// IntPhi returns ∫_0^t φ ds = -ln Z(0,t) + V(t)/2.
func (m *HullWhite) IntPhi(t float64) float64 {
	if t == 0 {
		return 0
	}
	return -math.Log(m.Curve.Discount(t)) + 0.5*ou.VarIntegral(m.A, m.Sigma, t)
}

// Theta returns the drift function θ(t) = φ'(t) + a φ(t); the derivative of
// the forward rate is approximated by central differences.
func (m *HullWhite) Theta(t float64) float64 {
	h := 1e-4 * math.Max(1, t)
	var df float64
	if t >= h {
		df = (term.InstForward(m.Curve, t+h) - term.InstForward(m.Curve, t-h)) / (2 * h)
	} else {
		df = (term.InstForward(m.Curve, t+h) - term.InstForward(m.Curve, t)) / h
	}
	b := ou.B(m.A, t)
	dphi := df + m.Sigma*m.Sigma*b*math.Exp(-m.A*t)
	return dphi + m.A*m.Phi(t)
}

// Z returns the model price at time t of the zero-coupon bond with face
// value 1 maturing at T >= t, given the short rate r(t) = r:
//
//	Z(r;t,T) = Z(0,T)/Z(0,t) exp(B f(0,t) - σ²/2 B(2a,t) B² - B r),
//
// with B = B(a,T-t) (Brigo and Mercurio 2006, Eq. 3.39). Z(R0;0,T) =
// Z(0,T).
func (m *HullWhite) Z(r, t, T float64) float64 {
	b := ou.B(m.A, T-t)
	f := term.InstForward(m.Curve, t)
	return m.Curve.Discount(T) / m.Curve.Discount(t) *
		math.Exp(b*f-0.5*m.Sigma*m.Sigma*ou.B(2*m.A, t)*b*b-b*r)
}

// Discretize returns the exact simulator on g. State[0][i] is the short
// rate r(t_i) and Df[i] = exp(-∫_0^{t_i} r ds), simulated jointly with the
// exact Gaussian transition of (x, ∫x) (2 normals per step). The
// deterministic part of the discount integral is IntPhi(t_i), so
// E[Df[i]] = Z(0,t_i) exactly for every grid, and Df[i] = Z(0,t_i) for
// σ = 0.
func (m *HullWhite) Discretize(g *mc.Grid) (mc.Simulator, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	for _, t := range g.Times[1:] {
		if v := m.IntPhi(t); math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("hullwhite: curve discount factor at t=%v is not positive and finite", t)
		}
	}
	return ou.NewSim(g, m.A, m.Sigma, m.Phi, m.IntPhi), nil
}
