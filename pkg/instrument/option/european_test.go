package option_test

import (
	"errors"
	"math"
	"testing"

	"github.com/konimarti/fixedincome/pkg/instrument/option"
	"github.com/konimarti/fixedincome/pkg/term"
)

var (
	testOption = option.European{
		Type: option.Call,
		S:    110.0,
		K:    100.0,
		T:    2.0,
		Q:    0.0,
		Vol:  0.3,
	}
	ts = term.Flat{Rate: 0.02}
)

func TestN(t *testing.T) {
	tests := []struct {
		Value    float64
		Expected float64
	}{
		{0.5311, 0.7023},
		{-0.5311, 0.2977},
		{0.1068, 0.5425},
		{-0.1068, 0.4575},
	}
	for _, test := range tests {
		got := option.N(test.Value)
		if math.Abs(got-test.Expected) > 0.0001 {
			t.Errorf("N function failed; Got %v, Expected %v", got, test.Expected)
		}
	}
}

func TestD1(t *testing.T) {
	value := option.D1(testOption.S, testOption.K, testOption.T, 0.02, testOption.Q, testOption.Vol)
	expected := 0.5311
	if math.Abs(value-expected) > 0.0001 {
		t.Errorf("D1 function failed; Got %v, Expected %v", value, expected)
	}
}

func TestD2(t *testing.T) {
	d1 := option.D1(testOption.S, testOption.K, testOption.T, 0.02, testOption.Q, testOption.Vol)
	value := option.D2(d1, testOption.T, testOption.Vol)
	expected := 0.1068
	if math.Abs(value-expected) > 0.0001 {
		t.Errorf("D2 function failed; Got %v, Expected %v", value, expected)
	}
}

func TestEuropean_Price(t *testing.T) {
	for _, test := range []struct {
		Type     option.Type
		Expected float64
	}{
		{option.Call, 25.1291},
		{option.Put, 11.2080},
	} {
		opt := testOption
		opt.Type = test.Type
		value, err := opt.Price(ts)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(value-test.Expected) > 0.0001 {
			t.Errorf("pricing of European %v failed; Got %v, Expected %v", test.Type, value, test.Expected)
		}
	}
}

func TestEuropean_Greeks(t *testing.T) {
	testData := []struct {
		Type          option.Type
		ExpectedDelta float64
		ExpectedGamma float64
		ExpectedRho   float64
		ExpectedVega  float64
	}{
		{option.Call, 0.7023, 0.0074, 104.2505, 53.8985},
		{option.Put, -0.2977, 0.0074, -87.9074, 53.8985},
	}

	tolerance := 0.0001
	for _, test := range testData {
		opt := testOption
		opt.Type = test.Type
		g, err := opt.Greeks(ts)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(g.Delta-test.ExpectedDelta) > tolerance {
			t.Errorf("delta is not correct; Got %v, Expected %v", g.Delta, test.ExpectedDelta)
		}
		if math.Abs(g.Gamma-test.ExpectedGamma) > tolerance {
			t.Errorf("gamma is not correct; Got %v, Expected %v", g.Gamma, test.ExpectedGamma)
		}
		if math.Abs(g.Rho-test.ExpectedRho) > tolerance {
			t.Errorf("rho is not correct; Got %v, Expected %v", g.Rho, test.ExpectedRho)
		}
		if math.Abs(g.Vega-test.ExpectedVega) > tolerance {
			t.Errorf("vega is not correct; Got %v, Expected %v", g.Vega, test.ExpectedVega)
		}
	}
}

func TestImpliedVol(t *testing.T) {
	expected := 0.3
	for _, test := range []struct {
		Type  option.Type
		Price float64
	}{
		{option.Call, 25.1291},
		{option.Put, 11.2080},
	} {
		opt := testOption
		opt.Type = test.Type
		opt.Vol = math.Pi
		vol, err := option.ImpliedVol(test.Price, opt, ts)
		if err != nil || math.Abs(vol-expected) > 0.0001 {
			t.Errorf("type %v: got %v (%v), expected %v", test.Type, vol, err, expected)
		}
		if opt.Vol != math.Pi {
			t.Errorf("ImpliedVol modified the option")
		}
	}
}

// TestImpliedVol_Exact checks that the implied volatility reprices the
// option and that prices outside the attainable range return an error
func TestImpliedVol_Exact(t *testing.T) {
	opt := testOption
	price, _ := opt.Price(ts)
	opt.Vol = 0.9
	vol, err := option.ImpliedVol(price, opt, ts)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(vol-0.3) > 1e-9 {
		t.Errorf("wrong implied vol; got %v, expected %v", vol, 0.3)
	}
	// price below the lower arbitrage bound S - K*Z(T): no solution
	if _, err := option.ImpliedVol(1.0, opt, ts); !errors.Is(err, option.ErrNoImpliedVol) {
		t.Errorf("expected ErrNoImpliedVol for price below arbitrage bound, got %v", err)
	}
	// price above the underlying: no solution
	if _, err := option.ImpliedVol(200.0, opt, ts); !errors.Is(err, option.ErrNoImpliedVol) {
		t.Errorf("expected ErrNoImpliedVol for price above the underlying, got %v", err)
	}
	// deep in the money at the discounted intrinsic value: no vega
	deep := option.European{Type: option.Call, S: 200, K: 100, T: 1, Vol: 0.2}
	intrinsic := 200 - 100*ts.Discount(1)
	if _, err := option.ImpliedVol(intrinsic, deep, ts); !errors.Is(err, option.ErrNoVega) {
		t.Errorf("expected ErrNoVega, got %v", err)
	}
	// very low volatility is still found (lower bound 1e-6) for an option
	// struck at the forward (where the price is sensitive to the vol)
	low := testOption
	low.K = low.S * math.Exp(0.02*low.T)
	low.Vol = 0.001
	p, _ := low.Price(ts)
	if vol, err := option.ImpliedVol(p, low, ts); err != nil || math.Abs(vol-0.001) > 1e-6 {
		t.Errorf("low vol: got %v, %v", vol, err)
	}
}

func TestParseType(t *testing.T) {
	for in, expected := range map[string]option.Type{"call": option.Call, "CALL": option.Call, " c ": option.Call, "Put": option.Put, "p": option.Put} {
		if got, err := option.ParseType(in); err != nil || got != expected {
			t.Errorf("ParseType(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := option.ParseType("straddle"); err == nil {
		t.Errorf("expected error")
	}
	if option.Call.String() != "call" || option.Put.String() != "put" {
		t.Errorf("String()")
	}
}
