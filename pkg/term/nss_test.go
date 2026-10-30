package term_test

import (
	"math"
	"testing"

	"github.com/konimarti/fixedincome/pkg/term"
)

// snb20210331 are the NSS parameters (betas in decimal) as of 2021-03-31 from
// https://data.snb.ch/en/topics/ziredev#!/cube/rendopar
var snb20210331 = term.NSS{
	B0: -0.00266372, B1: -0.00471343, B2: 0.0568789, B3: -0.0512324,
	Tau1: 5.74881, Tau2: 4.14426,
}

func TestNSS(t *testing.T) {
	n := snb20210331

	// data for 2020-03 from https://data.snb.ch/en/topics/ziredev#!/cube/rendoblim
	data := []struct {
		M             float64
		RateInPercent float64 // annually compounded
	}{
		{1, -0.782},
		{2, -0.776},
		{3, -0.736},
		{4, -0.677},
		{5, -0.606},
		{6, -0.532},
		{7, -0.459},
		{8, -0.389},
		{9, -0.325},
		{10, -0.267},
		{15, -0.073},
		{20, -0.001},
		{30, -0.007},
	}

	for _, test := range data {
		got := n.ZeroRate(test.M)
		expected := math.Log(1.0 + test.RateInPercent/100.0)
		if math.Abs(got-expected) > 0.00001 {
			t.Errorf("got %f, but wanted %f, failed for maturity %f", got, expected, test.M)
		}
		if z := n.Discount(test.M); math.Abs(z-math.Exp(-got*test.M)) > 1e-15 {
			t.Errorf("discount factor inconsistent with zero rate at %v", test.M)
		}
	}
}

func TestNSS_ShortEnd(t *testing.T) {
	n := snb20210331
	limit := n.B0 + n.B1
	for _, x := range []float64{-1, 0, 1e-12, 1e-9, 1e-7} {
		if r := n.ZeroRate(x); math.Abs(r-limit) > 1e-9 {
			t.Errorf("ZeroRate(%v) = %v, expected limit %v", x, r, limit)
		}
		if f := n.InstForward(x); math.Abs(f-limit) > 1e-9 {
			t.Errorf("InstForward(%v) = %v, expected limit %v", x, f, limit)
		}
	}
	if n.Discount(0) != 1 {
		t.Errorf("Discount(0) = %v", n.Discount(0))
	}
	// continuity across the series/Expm1 switch (x = 1e-6): the change over
	// dt is bounded by the slope of the zero rate (< 0.01 per year)
	a, b := n.ZeroRate(5.74881*0.99e-6), n.ZeroRate(5.74881*1.01e-6)
	if math.Abs(a-b) > 0.01*5.74881*0.02e-6+1e-15 {
		t.Errorf("zero rate jumps at series switch: %v vs %v", a, b)
	}
}

// TestNSS_InstForward compares the analytic forward with finite differences
func TestNSS_InstForward(t *testing.T) {
	n := snb20210331
	numeric := term.WithSpread(numericOnly{n}, 0)
	for _, x := range []float64{0, 0.1, 1, 5, 10, 30} {
		got := n.InstForward(x)
		expected := term.InstForward(numeric, x)
		if math.Abs(got-expected) > 1e-8 {
			t.Errorf("InstForward(%v) = %v, finite difference %v", x, got, expected)
		}
	}
	// forward between t1 and t2 equals the average instantaneous forward
	f := term.Forward(n, 2, 3)
	expected := (n.ZeroRate(3)*3 - n.ZeroRate(2)*2)
	if math.Abs(f-expected) > 1e-14 {
		t.Errorf("Forward(2,3) = %v, expected %v", f, expected)
	}
	if math.Abs(term.Forward(n, 3, 2)-f) > 1e-15 {
		t.Errorf("Forward is not symmetric")
	}
}

// numericOnly hides the analytic InstForward of a curve
type numericOnly struct{ c term.Curve }

func (n numericOnly) Discount(t float64) float64 { return n.c.Discount(t) }
func (n numericOnly) ZeroRate(t float64) float64 { return n.c.ZeroRate(t) }

func TestNSS_Validate(t *testing.T) {
	if _, err := term.NewNSS(0.01, 0, 0, 0, 0, 1); err == nil {
		t.Errorf("expected error for tau1 = 0")
	}
	if _, err := term.NewNSS(0.01, 0, 0, 0, 1, -1); err == nil {
		t.Errorf("expected error for tau2 < 0")
	}
	if _, err := term.NewNSS(math.NaN(), 0, 0, 0, 1, 1); err == nil {
		t.Errorf("expected error for NaN")
	}
	if _, err := term.NewNSS(0.01, 0, 0, 0, 1, 1); err != nil {
		t.Errorf("unexpected error %v", err)
	}
}

func TestWithSpread(t *testing.T) {
	n := snb20210331
	c := term.WithSpread(term.WithSpread(n, 0.003), 0.002)
	base, s := term.Spread(c)
	if _, ok := base.(term.NSS); !ok || math.Abs(s-0.005) > 1e-16 {
		t.Fatalf("nested spreads do not collapse: %T, %v", base, s)
	}
	for _, x := range []float64{0.5, 5, 20} {
		if math.Abs(c.Discount(x)-n.Discount(x)*math.Exp(-0.005*x)) > 1e-15 {
			t.Errorf("Discount(%v) with spread", x)
		}
		if math.Abs(c.ZeroRate(x)-n.ZeroRate(x)-0.005) > 1e-15 {
			t.Errorf("ZeroRate(%v) with spread", x)
		}
		if math.Abs(term.InstForward(c, x)-n.InstForward(x)-0.005) > 1e-15 {
			t.Errorf("InstForward(%v) with spread", x)
		}
	}
	if b := term.WithSpread(c, -0.005); b != term.Curve(n) {
		t.Errorf("zero total spread does not return the base curve")
	}
	if b, s := term.Spread(n); b != term.Curve(n) || s != 0 {
		t.Errorf("Spread of a plain curve")
	}
}

func BenchmarkNSSDiscount(b *testing.B) {
	var c term.Curve = snb20210331
	sum := 0.0
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sum += c.Discount(float64(i%30) + 0.5)
	}
	_ = sum
}
