// Package ou implements the exact simulation of Gaussian one-factor short
// rate models r(t) = x(t) + φ(t), where x is the zero-mean
// Ornstein-Uhlenbeck process dx = -a x dt + σ dW, x(0) = 0 (a >= 0; a = 0 is
// a Brownian motion), and φ is deterministic.
//
// Scheme: the pair (x(t_i), Y(t_i) = ∫_0^{t_i} x ds) is a Gaussian Markov
// process and is simulated with its exact transition (Glasserman 2004,
// Sec. 3.3):
//
//	x_{i+1} = e^{-ah} x_i + ε1,            Var ε1 = σ² B(2a,h)
//	Y_{i+1} = Y_i + B(a,h) x_i + ε2,        Var ε2 = V(h), Cov(ε1,ε2) = σ² B(a,h)²/2
//
// with B(a,h) = (1-e^{-ah})/a and V(h) = Var(∫_0^h x) (see VarIntegral). The
// deterministic part enters through Φ(t_i) = ∫_0^{t_i} φ ds, which the models
// supply exactly on the nodes, so the pathwise discount factor
// Df_i = exp(-Φ(t_i) - Y_i) has the exact distribution for every grid: there
// is no time discretisation bias in E[Df_i].
package ou

import (
	"math"

	"github.com/konimarti/fixedincome/pkg/mc"
)

// B returns (1-e^{-a t})/a, and t for a = 0 (no singularity for a -> 0).
func B(a, t float64) float64 {
	x := a * t
	if math.Abs(x) < 1e-300 {
		return t
	}
	return -math.Expm1(-x) / a
}

// g returns x - 2(1-e^{-x}) + (1-e^{-2x})/2, using its Taylor series
// sum_{n>=3} (-1)^n (2-2^{n-1}) x^n/n! for small x (no cancellation).
func g(x float64) float64 {
	if x > 0.5 {
		return x + 2*math.Expm1(-x) - 0.5*math.Expm1(-2*x)
	}
	sum := 0.0
	term := x * x / 2 // x^n/n! for n = 2
	pow2 := 2.0       // 2^{n-1} for n = 2
	sign := 1.0
	for n := 3; n < 40; n++ {
		term *= x / float64(n)
		pow2 *= 2
		sign = -sign
		c := sign * (2 - pow2) * term
		sum += c
		if math.Abs(c) <= 1e-17*math.Abs(sum) {
			break
		}
	}
	return sum
}

// VarIntegral returns the variance V(t) = Var(∫_0^t x ds) of the integrated
// OU process with x(0) = 0: σ²/a³ (at - 2(1-e^{-at}) + (1-e^{-2at})/2),
// which tends to σ² t³/3 for a -> 0 (evaluated without cancellation).
func VarIntegral(a, sigma, t float64) float64 {
	x := a * t
	if x < 1e-8 {
		return sigma * sigma * t * t * t * (1.0/3 - x/4)
	}
	return sigma * sigma / (a * a * a) * g(x)
}

// Sim is an exact simulator on a grid; it implements mc.Simulator with one
// factor (the short rate) and 2 normals per step.
type Sim struct {
	decay, bx, sx, c1, c2 []float64 // per step
	phi, Phi              []float64 // per node
}

// NewSim discretises the model on g. phi and Phi return φ(t) and
// Φ(t) = ∫_0^t φ ds; Phi(0) must be 0.
func NewSim(g *mc.Grid, a, sigma float64, phi, Phi func(t float64) float64) *Sim {
	n := g.Steps()
	s := &Sim{
		decay: make([]float64, n), bx: make([]float64, n), sx: make([]float64, n),
		c1: make([]float64, n), c2: make([]float64, n),
		phi: make([]float64, n+1), Phi: make([]float64, n+1),
	}
	for i := 0; i <= n; i++ {
		t := g.Times[i]
		s.phi[i] = phi(t)
		s.Phi[i] = Phi(t)
	}
	for i := 0; i < n; i++ {
		h := g.Dt(i)
		s.decay[i] = math.Exp(-a * h)
		s.bx[i] = B(a, h)
		vx := sigma * sigma * B(2*a, h)
		vy := VarIntegral(a, sigma, h)
		cov := 0.5 * sigma * sigma * s.bx[i] * s.bx[i]
		if vx > 0 {
			s.sx[i] = math.Sqrt(vx)
			s.c1[i] = cov / s.sx[i]
		}
		if r := vy - s.c1[i]*s.c1[i]; r > 0 {
			s.c2[i] = math.Sqrt(r)
		}
	}
	return s
}

// Factors returns 1 (State[0] is the short rate r(t_i), decimal).
func (s *Sim) Factors() int { return 1 }

// Dim returns 2N.
func (s *Sim) Dim() int { return 2 * len(s.decay) }

// Simulate fills the short rates and discount factors of p.
func (s *Sim) Simulate(z []float64, p *mc.Path) {
	r, df := p.State[0], p.Df
	n := len(s.decay)
	_, _ = r[n], df[n]
	z = z[:2*n]
	x, y := 0.0, 0.0
	r[0] = s.phi[0]
	df[0] = math.Exp(-s.Phi[0])
	for i := 0; i < n; i++ {
		z1, z2 := z[2*i], z[2*i+1]
		y += s.bx[i]*x + s.c1[i]*z1 + s.c2[i]*z2
		x = s.decay[i]*x + s.sx[i]*z1
		r[i+1] = x + s.phi[i+1]
		df[i+1] = math.Exp(-s.Phi[i+1] - y)
	}
}
