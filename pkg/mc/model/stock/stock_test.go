package stock_test

import (
	"math"
	"math/rand"
	"testing"

	"github.com/konimarti/bonds/pkg/instrument/option"
	"github.com/konimarti/bonds/pkg/mc"
	"github.com/konimarti/bonds/pkg/mc/model/stock"
	"github.com/konimarti/bonds/pkg/term"
)

func TestStock_EuropeanCall(t *testing.T) {

	// define term structure
	term := term.Flat{
		2.0,
		0.0,
	}

	// define parameters for ho lee model
	S := 100.0
	sigma := 0.3
	T := 2.0
	N := 2 * 255
	K := 120.0
	r := term.Rate(T) / 100.0

	max := func(a, b float64) float64 {
		if a > b {
			return a
		}
		return b
	}

	// create stock model to simulate stock prices
	model := stock.New(r, S, sigma, T, N, func(stockPrices []float64) float64 {
		return term.Z(T) * max(stockPrices[N-1]-K, 0.00)
	})

	// overwriting default rng to make monte carlo more deterministic for testing
	model.Rng = rand.New(rand.NewSource(99))

	// calculate reference price with Black Scholes
	refCall := option.European{
		option.Call, S, K, T, 0.0, sigma,
	}
	refCallValue := refCall.PresentValue(&term)

	// perform monte carlo simulation with ho lee model
	engine := mc.New(model, 1e4)
	err := engine.Run()
	if err != nil {
		t.Error(err)
	}
	europeanCall, err := engine.Estimate()
	if err != nil {
		t.Error(err)
	}
	if math.Abs(europeanCall-refCallValue) > 0.05 {
		t.Errorf("stock model failed to price the European call option; got: %v, expected: %v", europeanCall, refCallValue)
	}
}
