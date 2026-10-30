package main

import (
	"math"
	"testing"
	"time"
)

func TestParseDate(t *testing.T) {
	expected := time.Date(2021, 12, 2, 0, 0, 0, 0, time.UTC)
	for _, value := range []string{"2021-12-02", "02.12.2021", " 02.12.2021 "} {
		d, err := parseDate(value)
		if err != nil || !d.Equal(expected) {
			t.Errorf("parseDate(%q) = %v, %v; expected %v", value, d, err, expected)
		}
	}
	for _, value := range []string{"2021-13-02", "maturity", "", "2.12.21"} {
		if _, err := parseDate(value); err == nil {
			t.Errorf("parseDate(%q): expected error", value)
		}
	}
}

func TestIsHeader(t *testing.T) {
	testData := []struct {
		Row      []string
		Expected bool
	}{
		{[]string{"maturity", "coupon", "price", "id"}, true},
		{[]string{"02.12.2021", "0", "100.007", "IQT713061070"}, false},
		// typo in the date of a data row: not a header (must be reported)
		{[]string{"31.02.2021", "0", "100.007", "IQT713061070"}, false},
		{[]string{"2021-12-02", "coupon", "price"}, false},
		{[]string{"maturity"}, false},
	}
	for _, test := range testData {
		if got := isHeader(test.Row); got != test.Expected {
			t.Errorf("isHeader(%v) = %v, expected %v", test.Row, got, test.Expected)
		}
	}
}

func TestSplineKnots(t *testing.T) {
	testData := [][]float64{
		{0.5, 1.5},           // e.g. two bonds
		{0.5, 1.0, 1.5},      // few maturities
		{0.25, 0.5, 0.75, 1}, // single bond with quarterly coupons
		{0.1},
	}
	for _, maturities := range testData {
		for _, nbonds := range []int{1, 2, 3, 10} {
			knots := splineKnots(maturities, nbonds)
			if len(knots) == 0 {
				t.Errorf("no knots for %v", maturities)
			}
			for i, k := range knots {
				if k <= 0.0 || (i > 0 && !(k > knots[i-1])) {
					t.Errorf("maturities %v, %d bonds: knots not positive and strictly increasing: %v", maturities, nbonds, knots)
					break
				}
			}
			if knots[0] != maturities[0] || knots[len(knots)-1] != maturities[len(maturities)-1] {
				t.Errorf("knots %v do not span maturities %v", knots, maturities)
			}
			// a spline must be constructible from at least two knots
			if len(knots) >= 2 {
				z := make([]float64, len(knots))
				for i, k := range knots {
					z[i] = math.Exp(-0.01 * k)
				}
				if _, err := newSpline(knots, z); err != nil {
					t.Errorf("knots %v: %v", knots, err)
				}
			}
		}
	}
}

func TestNewSpline_InvalidTrialPoint(t *testing.T) {
	// the optimizer can propose non-positive discount factors: no panic
	if _, err := newSpline([]float64{0.5, 1.0, 2.0}, []float64{0.99, -0.1, 0.97}); err == nil {
		t.Errorf("expected error for negative discount factor")
	}
	if _, err := newSpline([]float64{0.5, 0.5, 2.0}, []float64{0.99, 0.98, 0.97}); err == nil {
		t.Errorf("expected error for duplicate knots")
	}
}
