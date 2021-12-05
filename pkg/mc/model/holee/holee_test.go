package holee_test

import (
	"math"
	"math/rand"
	"testing"

	"github.com/konimarti/bonds/pkg/mc"
	"github.com/konimarti/bonds/pkg/mc/model/holee"
	"github.com/konimarti/bonds/pkg/term"
)

func TestHoLee(t *testing.T) {

	// define term structure for model calibration
	term := term.NelsonSiegelSvensson{
		-0.43381,
		-0.308942,
		4.83643,
		-4.10991,
		4.65211,
		3.33637,
		0.0,
	}

	// define parameters for ho lee model
	sigma := 0.001
	T := 5.0
	N := 5 * 255

	// create ho lee interest rate model with payoff function for zero bonds
	model := holee.New(&term, sigma, T, N, func(rates []float64) float64 {
		dt := T / float64(N)
		rate := 0.0
		for i := 0; i < (N - 1); i += 1 {
			rate += rates[i] * dt
		}
		return math.Exp(-rate) * 100.0
	})

	// overwriting default rng to make monte carlo more deterministic for testing
	model.Rng = rand.New(rand.NewSource(99))

	// perform monte carlo simulation with ho lee model
	engine := mc.New(model, 1e4)
	err := engine.Run()
	if err != nil {
		t.Error(err)
	}
	zeroBondEstimate, err := engine.Estimate()
	if err != nil {
		t.Error(err)
	}
	if math.Abs(zeroBondEstimate-term.Z(T)*100.0) > 0.01 {
		t.Errorf("ho lee model failed to calculate zero bond; got: %v, expected: %v", zeroBondEstimate, term.Z(T)*100.0)
	}

}
