// Package vasicek implements the Vasicek equilibrium short rate model
//
//	dr = γ (r̄ - r) dt + σ dW
//
// under the risk-neutral measure. Rates, r̄ and σ are decimals (σ in
// absolute rate units per square root of a year), γ is the mean reversion
// speed per year (γ = 0 is allowed: Brownian motion without drift), times
// are in years.
//
// The Vasicek model has constant parameters and is not fitted to the
// initial curve: its bond prices Z(r0;0,T) only approximate a given curve
// (see Calibrate and the reported RMSE). Use package hullwhite for an
// arbitrage-free model that reproduces the curve exactly.
package vasicek

import (
	"fmt"
	"math"

	"github.com/konimarti/fixedincome/pkg/mc"
	"github.com/konimarti/fixedincome/pkg/mc/model/internal/ou"
	"github.com/konimarti/fixedincome/pkg/term"
	"gonum.org/v1/gonum/optimize"
)

// Vasicek is the Vasicek model.
type Vasicek struct {
	// R0 is the initial short rate (continuously compounded, decimal)
	R0 float64
	// Rbar is the long-run mean of the short rate (decimal)
	Rbar float64
	// Gamma is the speed of mean reversion (per year, >= 0)
	Gamma float64
	// Sigma is the volatility of the short rate (decimal per square root of
	// a year, >= 0)
	Sigma float64
}

var _ mc.Model = (*Vasicek)(nil)

// New returns the Vasicek model with the given parameters.
func New(r0, rbar, gamma, sigma float64) (*Vasicek, error) {
	v := &Vasicek{R0: r0, Rbar: rbar, Gamma: gamma, Sigma: sigma}
	if err := v.Validate(); err != nil {
		return nil, err
	}
	return v, nil
}

// Validate checks the parameters.
func (v *Vasicek) Validate() error {
	for _, p := range []struct {
		name string
		x    float64
	}{{"R0", v.R0}, {"Rbar", v.Rbar}, {"Gamma", v.Gamma}, {"Sigma", v.Sigma}} {
		if math.IsNaN(p.x) || math.IsInf(p.x, 0) {
			return fmt.Errorf("vasicek: %s is not finite: %v", p.name, p.x)
		}
	}
	if v.Gamma < 0 {
		return fmt.Errorf("vasicek: mean reversion speed must be non-negative, got %v", v.Gamma)
	}
	if v.Sigma < 0 {
		return fmt.Errorf("vasicek: volatility must be non-negative, got %v", v.Sigma)
	}
	return nil
}

// Mean returns E[r(t)] = r̄ + (r0 - r̄) e^{-γt}.
func (v *Vasicek) Mean(t float64) float64 {
	return v.Rbar + (v.R0-v.Rbar)*math.Exp(-v.Gamma*t)
}

// intMean returns ∫_0^t E[r(s)] ds = r̄ t + (r0 - r̄) B(γ,t).
func (v *Vasicek) intMean(t float64) float64 {
	return v.Rbar*t + (v.R0-v.Rbar)*ou.B(v.Gamma, t)
}

// Z returns the price at time t of the zero-coupon bond with face value 1
// maturing at T >= t given the short rate r(t) = r:
//
//	Z(r;t,T) = exp(-r̄ τ - (r - r̄) B(γ,τ) + V(τ)/2),   τ = T - t,
//
// with B(γ,τ) = (1-e^{-γτ})/γ and V(τ) = Var(∫_t^T r), which is the
// formula of Veronesi (2010, Eq. 15.28) written without the 1/γ
// singularity (γ -> 0 gives exp(-rτ + σ²τ³/6)).
func (v *Vasicek) Z(r, t, T float64) float64 {
	tau := T - t
	return math.Exp(-v.Rbar*tau - (r-v.Rbar)*ou.B(v.Gamma, tau) +
		0.5*ou.VarIntegral(v.Gamma, v.Sigma, tau))
}

// ZeroRate returns the continuously compounded model zero rate
// -ln Z(R0;0,T)/T for maturity T > 0 (R0 for T = 0).
func (v *Vasicek) ZeroRate(T float64) float64 {
	if T <= 0 {
		return v.R0
	}
	return -math.Log(v.Z(v.R0, 0, T)) / T
}

// Discretize returns the exact simulator on g: the OU transition of r and
// the exact joint Gaussian transition of ∫r (no Euler discretisation, 2
// normals per step). State[0][i] = r(t_i), Df[i] = exp(-∫_0^{t_i} r ds)
// with E[Df[i]] = Z(R0;0,t_i) for every grid.
func (v *Vasicek) Discretize(g *mc.Grid) (mc.Simulator, error) {
	if err := v.Validate(); err != nil {
		return nil, err
	}
	return ou.NewSim(g, v.Gamma, v.Sigma, v.Mean, v.intMean), nil
}

// Calibration reports the fit of Calibrate.
type Calibration struct {
	// RMSE is the root mean squared error of the model zero rates against
	// the curve zero rates at the calibration maturities (decimal)
	RMSE float64
	// Evaluations is the number of objective function evaluations
	Evaluations int
	// AtBound reports that γ is at MinGamma or MaxGamma (relative
	// tolerance 1e-6)
	AtBound bool
}

// Bounds of the mean reversion speed in Calibrate (per year).
const (
	MinGamma = 1e-4
	MaxGamma = 50.0
)

// Calibrate fits r̄ and γ of the Vasicek model with the given volatility
// sigma to the zero rates of the curve c at the maturities (years, > 0) by
// least squares (Nelder-Mead). R0 is set to the short rate of the curve,
// c.ZeroRate(0).
//
// Parametrisation: the optimiser works on (κ, u) with the drift level
// κ = γ r̄ and γ = MinGamma + (MaxGamma-MinGamma)/(1 + e^{-u}), so γ stays
// in (MinGamma, MaxGamma) and the objective stays well-conditioned for
// γ -> 0, where the model tends to a Brownian motion with drift κ (the
// bond price has no 1/γ singularity, see Z). If the best fit is at
// γ = MinGamma (Calibration.AtBound), the curve is better described by a
// model without mean reversion and r̄ = κ/γ can be very large.
//
// The returned Calibration reports the RMSE, which is in general not zero:
// the Vasicek model cannot reproduce an arbitrary curve (use hullwhite).
func Calibrate(c term.Curve, sigma float64, maturities []float64) (*Vasicek, Calibration, error) {
	if c == nil {
		return nil, Calibration{}, fmt.Errorf("vasicek: nil curve")
	}
	if len(maturities) == 0 {
		return nil, Calibration{}, fmt.Errorf("vasicek: no calibration maturities")
	}
	target := make([]float64, len(maturities))
	for i, t := range maturities {
		if !(t > 0) || math.IsInf(t, 0) {
			return nil, Calibration{}, fmt.Errorf("vasicek: calibration maturity must be positive and finite, got %v", t)
		}
		target[i] = c.ZeroRate(t)
		if math.IsNaN(target[i]) || math.IsInf(target[i], 0) {
			return nil, Calibration{}, fmt.Errorf("vasicek: curve zero rate at %v is not finite", t)
		}
	}
	v := &Vasicek{R0: c.ZeroRate(0), Sigma: sigma}
	if err := v.Validate(); err != nil {
		return nil, Calibration{}, err
	}
	gamma := func(u float64) float64 {
		return MinGamma + (MaxGamma-MinGamma)/(1+math.Exp(-u))
	}
	evals := 0
	obj := func(x []float64) float64 {
		evals++
		g := gamma(x[1])
		m := Vasicek{R0: v.R0, Rbar: x[0] / g, Gamma: g, Sigma: sigma}
		sum := 0.0
		for i, t := range maturities {
			d := m.ZeroRate(t) - target[i]
			sum += d * d
		}
		return sum / float64(len(maturities))
	}
	// start: long rate as mean, γ = 0.5
	u0 := -math.Log((MaxGamma-MinGamma)/(0.5-MinGamma) - 1)
	x0 := []float64{0.5 * target[len(target)-1], u0}
	res, err := optimize.Minimize(optimize.Problem{Func: obj}, x0,
		&optimize.Settings{MajorIterations: 5000, Converger: &optimize.FunctionConverge{Absolute: 1e-20, Relative: 1e-12, Iterations: 200}},
		&optimize.NelderMead{})
	if err != nil {
		return nil, Calibration{}, fmt.Errorf("vasicek: calibration failed: %w", err)
	}
	v.Gamma = gamma(res.X[1])
	v.Rbar = res.X[0] / v.Gamma
	cal := Calibration{RMSE: math.Sqrt(obj(res.X)), Evaluations: evals,
		AtBound: v.Gamma < MinGamma*(1+1e-6) || v.Gamma > MaxGamma*(1-1e-6)}
	if err := v.Validate(); err != nil {
		return nil, cal, err
	}
	return v, cal, nil
}
