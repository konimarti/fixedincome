package holee_test

import (
	"math"
	"math/rand"
	"testing"

	"github.com/konimarti/fixedincome/pkg/mc"
	"github.com/konimarti/fixedincome/pkg/mc/model/holee"
	"github.com/konimarti/fixedincome/pkg/term"
)

func TestHoLee(t *testing.T) {

	// define term structure for model calibration
	term := term.NelsonSiegelSvensson{
		B0:     -0.43381,
		B1:     -0.308942,
		B2:     4.83643,
		B3:     -4.10991,
		T1:     4.65211,
		T2:     3.33637,
		Spread: 0.0,
	}

	// define parameters for ho lee model
	sigma := 0.001
	T := 5.0
	N := 5 * 255

	// create ho lee interest rate model with payoff function for zero bonds
	model, err := holee.New(&term, sigma, T, N, func(rates []float64) float64 {
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

	// perform monte carlo simulation with ho lee model
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
	// accept deviations within 4 standard errors of the Monte Carlo estimate
	if math.Abs(zeroBondEstimate-term.Z(T)*100.0) > 4.0*stderr {
		t.Errorf("ho lee model failed to calculate zero bond; got: %v, expected: %v, standard error: %v", zeroBondEstimate, term.Z(T)*100.0, stderr)
	}

}
