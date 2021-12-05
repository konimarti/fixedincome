package vasicek_test

import (
	"math"
	"math/rand"
	"testing"

	"github.com/konimarti/bonds/pkg/mc"
	"github.com/konimarti/bonds/pkg/mc/model/vasicek"
	"github.com/konimarti/bonds/pkg/term"
)

func TestVasicek(t *testing.T) {

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
	sigma := 0.05000
	T := 5.0
	N := 5 * 252

	// create Vasicek interest rate model with payoff function for zero bonds
	model, err := vasicek.New(&term, sigma, T, N, func(rates []float64) float64 {
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
	if math.Abs(zeroBondEstimate-term.Z(T)*100.0) > 0.5 {
		t.Errorf("vasicek model failed to calculate zero bond; got: %v, expected: %v", zeroBondEstimate, term.Z(T)*100.0)
	}

}
