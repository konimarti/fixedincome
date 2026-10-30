// Package holee implements the Ho-Lee short rate model
//
//	dr = θ(t) dt + σ dW
//
// under the risk-neutral measure, with θ(t) = f'(0,t) + σ² t chosen such
// that the model reproduces the initial discount curve exactly. It is the
// Hull-White model with zero mean reversion (see package hullwhite for the
// simulation scheme). Rates and σ are decimals (σ = 0.01 is a normal
// volatility of 100bp per square root of a year), times are in years.
package holee

import (
	"github.com/konimarti/fixedincome/pkg/mc"
	"github.com/konimarti/fixedincome/pkg/mc/model/hullwhite"
	"github.com/konimarti/fixedincome/pkg/term"
)

// HoLee is the Ho-Lee model fitted to an initial curve. Create it with New.
type HoLee struct {
	hw hullwhite.HullWhite
}

var _ mc.Model = (*HoLee)(nil)

// New returns the Ho-Lee model with volatility sigma >= 0 (decimal per
// square root of a year) calibrated to the curve c. The calibration is
// exact and implicit: the short rate is r(t) = σW(t) + φ(t) with
// φ(t) = f(0,t) + σ²t²/2, and the discount integral uses
// ∫_0^t φ = -ln Z(0,t) + σ²t³/6 (exact on every grid node).
func New(c term.Curve, sigma float64) (*HoLee, error) {
	hw, err := hullwhite.New(c, 0, sigma)
	if err != nil {
		return nil, err
	}
	return &HoLee{hw: *hw}, nil
}

// Curve returns the initial curve.
func (m *HoLee) Curve() term.Curve { return m.hw.Curve }

// Sigma returns the volatility (decimal per square root of a year).
func (m *HoLee) Sigma() float64 { return m.hw.Sigma }

// R0 returns the initial short rate r(0) = f(0,0) (decimal), set by the
// calibration.
func (m *HoLee) R0() float64 { return m.hw.R0() }

// Theta returns θ(t) = f'(0,t) + σ²t (decimal per year; f' by central
// differences).
func (m *HoLee) Theta(t float64) float64 { return m.hw.Theta(t) }

// Phi returns E[r(t)] = f(0,t) + σ²t²/2.
func (m *HoLee) Phi(t float64) float64 { return m.hw.Phi(t) }

// Z returns the model price at time t of the zero-coupon bond with face
// value 1 maturing at T >= t given r(t) = r:
//
//	Z(r;t,T) = Z(0,T)/Z(0,t) exp((T-t) f(0,t) - σ²/2 t (T-t)² - (T-t) r).
func (m *HoLee) Z(r, t, T float64) float64 { return m.hw.Z(r, t, T) }

// Discretize returns the exact simulator on g (State[0] = short rate, Df =
// pathwise discount factor; E[Df[i]] = Z(0,t_i) for every grid, Df[i] =
// Z(0,t_i) for σ = 0).
func (m *HoLee) Discretize(g *mc.Grid) (mc.Simulator, error) {
	return m.hw.Discretize(g)
}
