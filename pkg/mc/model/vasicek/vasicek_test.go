package vasicek_test

import (
	"math"
	"math/rand"
	"testing"

	"github.com/konimarti/fixedincome/pkg/mc"
	"github.com/konimarti/fixedincome/pkg/mc/model/vasicek"
	"github.com/konimarti/fixedincome/pkg/term"
)

func TestVasicek(t *testing.T) {

	// define term structure for model calibration
	term := term.NSS{
		B0:   -0.0043381,
		B1:   -0.00308942,
		B2:   0.0483643,
		B3:   -0.0410991,
		Tau1: 4.65211,
		Tau2: 3.33637,
	}

	// define parameters for ho lee model
	sigma := 0.05000
	T := 5.0
	N := 5 * 252

	// create Vasicek interest rate model with payoff function for zero bonds
	model, err := vasicek.New(term, sigma, T, N, func(rates []float64) float64 {
		dt := T / float64(N)
		rate := 0.0
		for i := 0; i < (N - 1); i += 1 {
			rate += rates[i] * dt
		}
		return math.Exp(-rate) * 100.0
	})
	if err != nil {
		t.Error(err)
	}

	// overwriting default rng to make monte carlo more deterministic for testing
	model.Rng = rand.New(rand.NewSource(99))

	// run monte carlo simulation
	engine := mc.New(model, 1e4)
	err = engine.Run()
	if err != nil {
		t.Error(err)
	}
	zeroBondEstimate, err := engine.Estimate()
	if err != nil {
		t.Error(err)
	}
	stderr, err := engine.StdError()
	if err != nil {
		t.Error(err)
	}
	// compare the Monte Carlo estimate with the analytic zero bond price of
	// the calibrated Vasicek model; accept deviations within 4 standard
	// errors. Note: the calibrated model does not reproduce the term
	// structure exactly (calibration bias of about 2 standard errors w.r.t.
	// term.Discount(T), to be addressed in WP4).
	expected := model.Z(model.R0, 0.0, T) * 100.0
	if math.Abs(zeroBondEstimate-expected) > 4.0*stderr {
		t.Errorf("vasicek model failed to calculate zero bond; got: %v, expected: %v, standard error: %v (term structure: %v)", zeroBondEstimate, expected, stderr, term.Discount(T)*100.0)
	}

}
