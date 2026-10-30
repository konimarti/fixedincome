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

// TestEuropean_GreeksWithDividends checks all Greeks against central finite
// differences of Price for a non-zero dividend yield
func TestEuropean_GreeksWithDividends(t *testing.T) {
	ts := term.Flat{Rate: 0.02}
	base := option.European{Type: option.Call, S: 110.0, K: 100.0, T: 2.0, Q: 0.03, Vol: 0.3}

	for _, typ := range []option.Type{option.Call, option.Put} {
		opt := base
		opt.Type = typ
		price := func(o option.European, c term.Curve) float64 {
			p, err := o.Price(c)
			if err != nil {
				t.Fatal(err)
			}
			return p
		}
		pvS := func(s float64) float64 { o := opt; o.S = s; return price(o, ts) }
		pvVol := func(v float64) float64 { o := opt; o.Vol = v; return price(o, ts) }
		pvRate := func(r float64) float64 { return price(opt, term.Flat{Rate: r}) }
		pvQ := func(q float64) float64 { o := opt; o.Q = q; return price(o, ts) }
		pvT := func(T float64) float64 { o := opt; o.T = T; return price(o, ts) }

		h := 1e-3
		g, err := opt.Greeks(ts)
		if err != nil {
			t.Fatal(err)
		}
		tests := []struct {
			Name     string
			Got      float64
			Expected float64
			Tol      float64
		}{
			{"delta", g.Delta, centralDiff(pvS, opt.S, h), 1e-6},
			{"gamma", g.Gamma, (pvS(opt.S+h) - 2.0*pvS(opt.S) + pvS(opt.S-h)) / (h * h), 1e-5},
			{"vega", g.Vega, centralDiff(pvVol, opt.Vol, 1e-5), 1e-5},
			{"rho", g.Rho, centralDiff(pvRate, 0.02, 1e-6), 1e-4},
			{"phi", g.Phi, centralDiff(pvQ, 0.03, 1e-6), 1e-4},
			{"theta", g.Theta, -centralDiff(pvT, opt.T, 1e-5), 1e-4},
		}
		for _, test := range tests {
			if math.Abs(test.Got-test.Expected) > test.Tol {
				t.Errorf("type %v: %s is not correct; got %v, expected (finite difference) %v",
					typ, test.Name, test.Got, test.Expected)
			}
		}
	}
}

// TestEuropean_PutCallParity checks C - P = S exp(-qT) - K Z(T) with q != 0
func TestEuropean_PutCallParity(t *testing.T) {
	ts := term.Flat{Rate: 0.02}
	call := option.European{Type: option.Call, S: 110.0, K: 100.0, T: 2.0, Q: 0.03, Vol: 0.3}
	put := call
	put.Type = option.Put

	c, _ := call.Price(ts)
	p, _ := put.Price(ts)
	rhs := call.S*math.Exp(-0.03*call.T) - call.K*math.Exp(-0.02*call.T)
	if math.Abs(c-p-rhs) > 1e-10 {
		t.Errorf("put-call parity violated; got C-P=%v, expected %v", c-p, rhs)
	}

	// put-call parity for delta: Delta_C - Delta_P = exp(-qT)
	gc, _ := call.Greeks(ts)
	gp, _ := put.Greeks(ts)
	if math.Abs(gc.Delta-gp.Delta-math.Exp(-0.03*call.T)) > 1e-10 {
		t.Errorf("put-call parity for delta violated; got %v, expected %v", gc.Delta-gp.Delta, math.Exp(-0.03*call.T))
	}
}

// TestEuropean_Invalid checks that invalid options return errors instead of
// being silently priced at zero
func TestEuropean_Invalid(t *testing.T) {
	ts := term.Flat{Rate: 0.02}
	invalid := []option.European{
		{Type: 7, S: 110.0, K: 100.0, T: 2.0, Vol: 0.3},
		{Type: 0, S: 110.0, K: 100.0, T: 2.0, Vol: 0.3},
		{Type: option.Call, S: -1, K: 100.0, T: 2.0, Vol: 0.3},
		{Type: option.Call, S: 110, K: 0, T: 2.0, Vol: 0.3},
		{Type: option.Call, S: 110, K: 100, T: -1, Vol: 0.3},
		{Type: option.Call, S: 110, K: 100, T: 2, Vol: 0},
		{Type: option.Call, S: 110, K: 100, T: 2, Vol: 0.3, Q: math.NaN()},
	}
	for i, opt := range invalid {
		if _, err := opt.Price(ts); err == nil {
			t.Errorf("option %d: expected error from Price", i)
		}
		if _, err := opt.Greeks(ts); err == nil {
			t.Errorf("option %d: expected error from Greeks", i)
		}
	}
	if _, err := option.ImpliedVol(10, invalid[0], ts); err == nil {
		t.Errorf("expected error from ImpliedVol for invalid type")
	}
}

// TestEuropean_Expiry checks intrinsic values and finite limit Greeks at T=0
func TestEuropean_Expiry(t *testing.T) {
	ts := term.Flat{Rate: 0.02}
	for _, test := range []struct {
		Type  option.Type
		S     float64
		Price float64
		Delta float64
	}{
		{option.Call, 110, 10, 1},
		{option.Call, 90, 0, 0},
		{option.Call, 100, 0, 0.5},
		{option.Put, 90, 10, -1},
		{option.Put, 110, 0, 0},
	} {
		opt := option.European{Type: test.Type, S: test.S, K: 100, T: 0, Vol: 0.3}
		p, err := opt.Price(ts)
		if err != nil || p != test.Price {
			t.Errorf("%v S=%v: price %v, %v, expected %v", test.Type, test.S, p, err, test.Price)
		}
		g, err := opt.Greeks(ts)
		if err != nil || g != (option.Greeks{Delta: test.Delta}) {
			t.Errorf("%v S=%v: greeks %+v, %v", test.Type, test.S, g, err)
		}
	}
	if _, err := option.ImpliedVol(10, option.European{Type: option.Call, S: 110, K: 100, T: 0, Vol: 0.3}, ts); err == nil {
		t.Errorf("expected error for implied vol at expiry")
	}
}
