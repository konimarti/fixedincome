package term_test

import (
	"math"
	"sort"
	"sync"
	"testing"

	"github.com/konimarti/fixedincome/pkg/term"
)

func TestSpline(t *testing.T) {
	// use NSS term structure as reference
	refTerm := snb20210331

	maturities := []float64{20.0, 0.25, 0.5, 1.0, 3.0, 2.0, 5.0, 7.0, 10.0, 15.0}
	sort.Float64s(maturities)
	var refRate, refZ []float64
	for _, t := range maturities {
		refRate = append(refRate, refTerm.ZeroRate(t))
		refZ = append(refZ, refTerm.Discount(t))
	}

	spline, err := term.NewSpline(maturities, refZ)
	if err != nil {
		t.Fatal(err)
	}

	// spline interpolates the knots
	for i, x := range maturities {
		if math.Abs(spline.ZeroRate(x)-refRate[i]) > 1e-14 {
			t.Errorf("spline does not interpolate the rate at %v; got: %v, expected: %v", x, spline.ZeroRate(x), refRate[i])
		}
		if math.Abs(spline.Discount(x)-refZ[i]) > 1e-14 {
			t.Errorf("spline does not interpolate Z at %v", x)
		}
	}

	// approximation between knots
	sum := 0.0
	for x := 0.25; x <= 20; x += 0.125 {
		sum += math.Pow(spline.ZeroRate(x)-refTerm.ZeroRate(x), 2.0)
	}
	// root mean squared error below 0.3bp
	if sum > 159*math.Pow(0.3e-4, 2) {
		t.Errorf("spline does not accurately interpolate rates of yield curve; squared error %v", sum)
	}
}

// TestSpline_Flat checks that a flat curve is reproduced exactly (the
// log-discount factors are linear), including the short end below the first
// knot and the extrapolation beyond the last knot
func TestSpline_Flat(t *testing.T) {
	flat := term.Flat{Rate: 0.02}
	for _, m := range [][]float64{
		{0.5, 1.0, 2.0, 5.0, 10.0},
		{0.0, 0.5, 1.0, 2.0, 5.0, 10.0},
		{3.0},
	} {
		z := make([]float64, len(m))
		for i, x := range m {
			z[i] = flat.Discount(x)
		}
		spline, err := term.NewSpline(m, z)
		if err != nil {
			t.Fatal(err)
		}
		for _, spread := range []float64{0.0, 0.005} {
			c := term.WithSpread(spline, spread)
			expectedRate := 0.02 + spread
			for _, x := range []float64{-1.0, 0.0, 1e-9, 0.1, 0.25, 0.5, 7.0, 30.0} {
				if r := c.ZeroRate(x); math.Abs(r-expectedRate) > 1e-12 {
					t.Errorf("knots %v spread %v: ZeroRate(%v); got: %v, expected: %v", m, spread, x, r, expectedRate)
				}
				if f := term.InstForward(c, x); math.Abs(f-expectedRate) > 1e-12 {
					t.Errorf("knots %v spread %v: InstForward(%v); got: %v", m, spread, x, f)
				}
				if x >= 0 {
					if got, expected := c.Discount(x), math.Exp(-expectedRate*x); math.Abs(got-expected) > 1e-12 {
						t.Errorf("knots %v spread %v: Discount(%v); got: %v, expected: %v", m, spread, x, got, expected)
					}
				}
			}
		}
	}
}

// TestSpline_SpreadSign checks that a positive spread lowers the discount
// factors, Z_s(t) = Z(t) * exp(-s*t), in the same way as for the NSS model
func TestSpline_SpreadSign(t *testing.T) {
	ref := snb20210331

	var m, z []float64
	for i := 1; i <= 600; i++ {
		x := float64(i) * 0.05
		m = append(m, x)
		z = append(z, ref.Discount(x))
	}

	spread := 0.01
	spline, err := term.NewSpline(m, z)
	if err != nil {
		t.Fatal(err)
	}
	s := term.WithSpread(spline, spread)
	r := term.WithSpread(ref, spread)

	for _, x := range []float64{0.5, 1.0, 5.0, 10.0, 20.0} {
		expected := z[int(math.Round(x/0.05))-1] * math.Exp(-spread*x)
		if math.Abs(s.Discount(x)-expected) > 1e-12 {
			t.Errorf("spline Discount(%v) with spread; got: %v, expected: %v", x, s.Discount(x), expected)
		}
		if math.Abs(s.ZeroRate(x)-r.ZeroRate(x)) > 1e-10 {
			t.Errorf("spline ZeroRate(%v) with spread; got: %v, expected: %v", x, s.ZeroRate(x), r.ZeroRate(x))
		}
	}
}

// TestSpline_Extrapolation checks the flat instantaneous forward beyond the
// last knot and continuity of the forward at the last knot
func TestSpline_Extrapolation(t *testing.T) {
	ref := snb20210331
	m := []float64{0.5, 1, 2, 5, 10}
	z := make([]float64, len(m))
	for i, x := range m {
		z[i] = ref.Discount(x)
	}
	s, err := term.NewSpline(m, z)
	if err != nil {
		t.Fatal(err)
	}
	fn := s.InstForward(10)
	if math.Abs(s.InstForward(10-1e-9)-fn) > 1e-7 {
		t.Errorf("forward not continuous at last knot")
	}
	for _, x := range []float64{10.5, 20, 50} {
		if s.InstForward(x) != fn {
			t.Errorf("forward not flat beyond last knot at %v", x)
		}
		expected := z[len(z)-1] * math.Exp(-fn*(x-10))
		if math.Abs(s.Discount(x)-expected) > 1e-14 {
			t.Errorf("Discount(%v) = %v, expected %v", x, s.Discount(x), expected)
		}
	}
	// analytic forward agrees with finite differences
	for _, x := range []float64{0.1, 0.5, 1.5, 4, 9.9} {
		if d := math.Abs(s.InstForward(x) - term.InstForward(numericOnly{s}, x)); d > 1e-8 {
			t.Errorf("InstForward(%v) differs from finite difference by %v", x, d)
		}
	}
}

// TestSpline_RateAtZero checks that the spot rate at t<=0 is finite and close
// to the short end of the curve
func TestSpline_RateAtZero(t *testing.T) {
	flat := term.Flat{Rate: 0.02}
	m := []float64{0.0, 0.5, 1.0, 2.0, 5.0, 10.0}
	z := make([]float64, len(m))
	for i, x := range m {
		z[i] = flat.Discount(x)
	}
	spline, err := term.NewSpline(m, z)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range []float64{0.0, -1.0} {
		r := spline.ZeroRate(x)
		if math.IsNaN(r) || math.IsInf(r, 0) {
			t.Errorf("ZeroRate(%v) is not finite: %v", x, r)
		}
		if math.Abs(r-0.02) > 1e-12 {
			t.Errorf("ZeroRate(%v); got: %v, expected: %v", x, r, 0.02)
		}
	}
}

// TestSpline_Validation checks that invalid inputs are rejected
func TestSpline_Validation(t *testing.T) {
	testData := []struct {
		Name string
		M    []float64
		Z    []float64
	}{
		{"unequal lengths", []float64{1.0, 2.0, 3.0}, []float64{0.99, 0.98}},
		{"empty", nil, nil},
		{"duplicate maturities", []float64{1.0, 2.0, 2.0}, []float64{0.99, 0.98, 0.97}},
		{"unsorted maturities", []float64{2.0, 1.0, 3.0}, []float64{0.98, 0.99, 0.97}},
		{"negative maturity", []float64{-1.0, 1.0}, []float64{1.01, 0.99}},
		{"anchor not 1", []float64{0.0, 1.0}, []float64{0.99, 0.98}},
		{"non-positive discount factor", []float64{1.0, 2.0, 3.0}, []float64{0.99, 0.0, 0.97}},
		{"NaN discount factor", []float64{1.0, 2.0, 3.0}, []float64{0.99, math.NaN(), 0.97}},
		{"Inf discount factor", []float64{1.0, 2.0}, []float64{0.99, math.Inf(1)}},
		{"NaN maturity", []float64{1.0, math.NaN()}, []float64{0.99, 0.98}},
	}
	for _, test := range testData {
		if _, err := term.NewSpline(test.M, test.Z); err == nil {
			t.Errorf("%s: expected error", test.Name)
		}
	}
	// a single knot is valid (flat forward through the anchor)
	s, err := term.NewSpline([]float64{2.0}, []float64{math.Exp(-0.04)})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(s.ZeroRate(0.5)-0.02) > 1e-15 {
		t.Errorf("single knot: got rate %v", s.ZeroRate(0.5))
	}
}

func TestSpline_Accessors(t *testing.T) {
	m := []float64{0.5, 1, 2}
	z := []float64{0.99, 0.98, 0.96}
	s, err := term.NewSpline(m, z)
	if err != nil {
		t.Fatal(err)
	}
	knots, dfs := s.Knots(), s.DiscountFactors()
	for i := range m {
		if knots[i] != m[i] || dfs[i] != z[i] {
			t.Errorf("accessors: got %v %v, expected %v %v", knots, dfs, m, z)
		}
	}
	knots[0] = 99 // copies
	m[1] = 99     // input is copied
	if s.Knots()[0] != 0.5 || s.Knots()[1] != 1 {
		t.Errorf("spline is not immutable")
	}
}

// TestSpline_Concurrent evaluates one spline concurrently (run with -race)
func TestSpline_Concurrent(t *testing.T) {
	s, err := term.NewSpline([]float64{0.5, 1, 2, 5, 10}, []float64{0.995, 0.99, 0.98, 0.95, 0.9})
	if err != nil {
		t.Fatal(err)
	}
	ref := s.Discount(3.3)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < 1000; k++ {
				if s.Discount(3.3) != ref {
					t.Errorf("concurrent evaluation differs")
					return
				}
				_ = term.WithSpread(s, 0.01).Discount(float64(k % 12))
			}
		}()
	}
	wg.Wait()
}

func BenchmarkSplineDiscount(b *testing.B) {
	var m, z []float64
	for i := 1; i <= 40; i++ {
		x := float64(i) * 0.75
		m = append(m, x)
		z = append(z, snb20210331.Discount(x))
	}
	s, err := term.NewSpline(m, z)
	if err != nil {
		b.Fatal(err)
	}
	var c term.Curve = s
	sum := 0.0
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sum += c.Discount(float64(i%30) + 0.5)
	}
	_ = sum
}
