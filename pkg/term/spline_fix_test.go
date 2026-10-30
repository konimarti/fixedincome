package term_test

import (
	"math"
	"testing"

	"github.com/konimarti/fixedincome/pkg/term"
)

// TestSpline_SpreadSign checks that a positive spread lowers the discount
// factors, Z_s(t) = Z(t) * exp(-s*t), in the same way as for the NSS model
func TestSpline_SpreadSign(t *testing.T) {
	ref := &term.NelsonSiegelSvensson{B0: -0.266372, B1: -0.471343, B2: 5.68789, B3: -5.12324, T1: 5.74881, T2: 4.14426}

	var m, z []float64
	for x := 0.05; x <= 30.0; x += 0.05 {
		m = append(m, x)
		z = append(z, ref.Z(x))
	}

	spread := 100.0 // bps
	spline := term.NewSpline(m, z, spread)
	ref.SetSpread(spread)

	for _, x := range []float64{0.5, 1.0, 5.0, 10.0, 20.0} {
		expected := z[int(math.Round(x/0.05))-1] * math.Exp(-spread*0.0001*x)
		if math.Abs(spline.Z(x)-expected) > 1e-6 {
			t.Errorf("spline Z(%v) with spread; got: %v, expected: %v", x, spline.Z(x), expected)
		}
		if math.Abs(spline.Rate(x)-ref.Rate(x)) > 1e-4 {
			t.Errorf("spline Rate(%v) with spread; got: %v, expected: %v", x, spline.Rate(x), ref.Rate(x))
		}
	}
}

// TestSpline_RateAtZero checks that the spot rate at t<=0 is finite and close
// to the short end of the curve
func TestSpline_RateAtZero(t *testing.T) {
	flat := term.Flat{R: 2.0}
	m := []float64{0.0, 0.5, 1.0, 2.0, 5.0, 10.0}
	z := make([]float64, len(m))
	for i, x := range m {
		z[i] = flat.Z(x)
	}
	spline := term.NewSpline(m, z, 0.0)
	for _, x := range []float64{0.0, -1.0} {
		r := spline.Rate(x)
		if math.IsNaN(r) || math.IsInf(r, 0) {
			t.Errorf("Rate(%v) is not finite: %v", x, r)
		}
		if math.Abs(r-2.0) > 0.01 {
			t.Errorf("Rate(%v); got: %v, expected: %v", x, r, 2.0)
		}
	}
}

// TestSpline_InitValidation checks that invalid inputs are rejected by Init
// and that NewSpline panics for them
func TestSpline_InitValidation(t *testing.T) {
	testData := []struct {
		Name string
		M    []float64
		Z    []float64
	}{
		{"unequal lengths", []float64{1.0, 2.0, 3.0}, []float64{0.99, 0.98}},
		{"single point", []float64{1.0}, []float64{0.99}},
		{"empty", nil, nil},
		{"duplicate maturities", []float64{1.0, 2.0, 2.0}, []float64{0.99, 0.98, 0.97}},
		{"non-positive discount factor", []float64{1.0, 2.0, 3.0}, []float64{0.99, 0.0, 0.97}},
		{"NaN discount factor", []float64{1.0, 2.0, 3.0}, []float64{0.99, math.NaN(), 0.97}},
	}
	for _, test := range testData {
		s := term.Spline{Maturities: test.M, DiscountFactors: test.Z}
		if err := s.Init(); err == nil {
			t.Errorf("%s: expected error from Init", test.Name)
		}
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: expected NewSpline to panic", test.Name)
				}
			}()
			term.NewSpline(test.M, test.Z, 0.0)
		}()
	}

	// valid unsorted input
	s := term.Spline{Maturities: []float64{2.0, 1.0, 3.0}, DiscountFactors: []float64{0.98, 0.99, 0.97}}
	if err := s.Init(); err != nil {
		t.Errorf("valid input: unexpected error %v", err)
	}
}

// TestSpline_ShortEndExtrapolation checks the flat zero-rate extrapolation
// below the first knot when the spline has no knot at t=0
func TestSpline_ShortEndExtrapolation(t *testing.T) {
	flat := term.Flat{R: 2.0}
	m := []float64{0.5, 1.0, 2.0, 5.0, 10.0}
	z := make([]float64, len(m))
	for i, x := range m {
		z[i] = flat.Z(x)
	}
	for _, spread := range []float64{0.0, 50.0} {
		spline := term.NewSpline(m, z, spread)
		expectedRate := 2.0 + spread*0.01
		for _, x := range []float64{-1.0, 0.0, 1e-9, 0.1, 0.25, 0.5} {
			r := spline.Rate(x)
			if math.Abs(r-expectedRate) > 1e-9 {
				t.Errorf("spread %v: Rate(%v); got: %v, expected: %v", spread, x, r, expectedRate)
			}
			if x > 0 {
				expectedZ := math.Exp(-expectedRate * 0.01 * x)
				if got := spline.Z(x); math.Abs(got-expectedZ) > 1e-12 {
					t.Errorf("spread %v: Z(%v); got: %v, expected: %v", spread, x, got, expectedZ)
				}
			}
		}
	}
}
