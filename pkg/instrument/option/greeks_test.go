package option_test

import (
	"math"
	"testing"

	"github.com/konimarti/fixedincome/pkg/instrument/option"
	"github.com/konimarti/fixedincome/pkg/term"
)

// centralDiff approximates the first derivative of f at x with a central
// finite difference
func centralDiff(f func(float64) float64, x, h float64) float64 {
	return (f(x+h) - f(x-h)) / (2.0 * h)
}

// TestEuropean_GreeksWithDividends checks Delta, Gamma, Vega and Rho against
// central finite differences of PresentValue for a non-zero dividend yield
func TestEuropean_GreeksWithDividends(t *testing.T) {
	ts := term.Flat{R: 2.0, Spread: 0.0}
	base := option.European{
		Type: option.Call,
		S:    110.0,
		K:    100.0,
		T:    2.0,
		Q:    3.0,
		Vola: 0.3,
	}

	for _, typ := range []int{option.Call, option.Put} {
		opt := base
		opt.Type = typ

		pvS := func(s float64) float64 {
			o := opt
			o.S = s
			return o.PresentValue(&ts)
		}
		pvVola := func(v float64) float64 {
			o := opt
			o.Vola = v
			return o.PresentValue(&ts)
		}
		// Rho is defined per unit of the (decimal) interest rate
		pvRate := func(r float64) float64 {
			tsr := term.Flat{R: r * 100.0, Spread: 0.0}
			return opt.PresentValue(&tsr)
		}

		h := 1e-3
		fdDelta := centralDiff(pvS, opt.S, h)
		fdGamma := (pvS(opt.S+h) - 2.0*pvS(opt.S) + pvS(opt.S-h)) / (h * h)
		fdVega := centralDiff(pvVola, opt.Vola, 1e-5)
		fdRho := centralDiff(pvRate, 0.02, 1e-6)

		tests := []struct {
			Name     string
			Got      float64
			Expected float64
			Tol      float64
		}{
			{"delta", opt.Delta(&ts), fdDelta, 1e-6},
			{"gamma", opt.Gamma(&ts), fdGamma, 1e-5},
			{"vega", opt.Vega(&ts), fdVega, 1e-5},
			{"rho", opt.Rho(&ts), fdRho, 1e-4},
		}
		for _, test := range tests {
			if math.Abs(test.Got-test.Expected) > test.Tol {
				t.Errorf("type %d: %s is not correct; got %v, expected (finite difference) %v",
					typ, test.Name, test.Got, test.Expected)
			}
		}
	}
}

// TestEuropean_PutCallParity checks C - P = S exp(-qT) - K Z(T) with q != 0
func TestEuropean_PutCallParity(t *testing.T) {
	ts := term.Flat{R: 2.0, Spread: 0.0}
	call := option.European{Type: option.Call, S: 110.0, K: 100.0, T: 2.0, Q: 3.0, Vola: 0.3}
	put := call
	put.Type = option.Put

	lhs := call.PresentValue(&ts) - put.PresentValue(&ts)
	rhs := call.S*math.Exp(-0.03*call.T) - call.K*math.Exp(-0.02*call.T)
	if math.Abs(lhs-rhs) > 1e-10 {
		t.Errorf("put-call parity violated; got C-P=%v, expected %v", lhs, rhs)
	}

	// put-call parity for delta: Delta_C - Delta_P = exp(-qT)
	dlhs := call.Delta(&ts) - put.Delta(&ts)
	drhs := math.Exp(-0.03 * call.T)
	if math.Abs(dlhs-drhs) > 1e-10 {
		t.Errorf("put-call parity for delta violated; got %v, expected %v", dlhs, drhs)
	}
}

// TestEuropean_InvalidType checks that an unknown option type is not silently
// priced at zero
func TestEuropean_InvalidType(t *testing.T) {
	ts := term.Flat{R: 2.0, Spread: 0.0}
	opt := option.European{Type: 7, S: 110.0, K: 100.0, T: 2.0, Q: 0.0, Vola: 0.3}
	for name, fn := range map[string]func(){
		"PresentValue": func() { opt.PresentValue(&ts) },
		"Delta":        func() { opt.Delta(&ts) },
		"Rho":          func() { opt.Rho(&ts) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: expected panic for invalid option type", name)
				}
			}()
			fn()
		}()
	}
}
