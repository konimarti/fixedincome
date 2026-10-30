package stock_test

import (
	"math"
	"math/rand"
	"testing"

	"github.com/konimarti/fixedincome/pkg/instrument/option"
	"github.com/konimarti/fixedincome/pkg/mc"
	"github.com/konimarti/fixedincome/pkg/mc/model/stock"
	"github.com/konimarti/fixedincome/pkg/term"
)

func TestStock_EuropeanCall(t *testing.T) {

	// define term structure
	term := term.Flat{Rate: 0.02}

	// define parameters for ho lee model
	S := 110.0
	sigma := 0.3
	T := 2.0
	N := 2 * 255
	K := 100.0
	r := term.ZeroRate(T)

	max := func(a, b float64) float64 {
		if a > b {
			return a
		}
		return b
	}

	// create stock model to simulate stock prices
	model := stock.New(r, S, sigma, T, N, func(stockPrices []float64) float64 {
		return term.Discount(T) * max(stockPrices[N-1]-K, 0.00)
	})

	// overwriting default rng to make monte carlo more deterministic for testing
	model.Rng = rand.New(rand.NewSource(99))

	// calculate reference price with Black Scholes
	refCall := option.European{
		Type: option.Call, S: S, K: K, T: T, Q: 0.0, Vol: sigma,
	}
	refCallValue, err := refCall.Price(term)
	if err != nil {
		t.Fatal(err)
	}

	// perform monte carlo simulation with ho lee model
	engine := mc.New(model, 1e5)
	err = engine.Run()
	if err != nil {
		t.Error(err)
	}
	europeanCall, err := engine.Estimate()
	if err != nil {
		t.Error(err)
	}
	stderr, err := engine.StdError()
	if err != nil {
		t.Error(err)
	}
	// accept deviations within 4 standard errors of the Monte Carlo estimate
	if math.Abs(europeanCall-refCallValue) > 4.0*stderr {
		t.Errorf("stock model failed to price the European call option; got: %v, expected: %v, standard error: %v", europeanCall, refCallValue, stderr)
	}
}
