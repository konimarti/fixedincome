package term_test

import (
	"math"
	"testing"

	"github.com/konimarti/bonds/pkg/term"
)

func TestSpline(t *testing.T) {

	// use NSS term structure as reference
	refTerm := term.NelsonSiegelSvensson{
		-0.266372,
		-0.471343,
		5.68789,
		-5.12324,
		5.74881,
		4.14426,
		0.0, // spread
	}

	maturities := []float64{0.25, 0.5, 1.0, 2.0, 3.0, 5.0, 7.0, 10.0, 15.0, 20.0}
	var refRate, refZ []float64
	for _, t := range maturities {
		refRate = append(refRate, refTerm.Rate(t))
		refZ = append(refZ, refTerm.Z(t))
	}

	// create spline term structure
	spread := 0.0
	spline := term.NewSpline(maturities, refZ, spread)

	// test spline approximation: rates
	sum := 0.0
	for i, t := range maturities {
		sum += math.Pow(spline.Rate(t)-refRate[i], 2.0)
	}

	if math.Abs(sum) > 0.000001 {
		t.Errorf("splines do not accurately interpolte rates of yield curve; got: %v, expected: %v", sum, 0.0)
	}

	// test spline approximation: rates
	sum = 0.0
	for i, t := range maturities {
		sum += math.Pow(spline.Z(t)-refZ[i], 2.0)
	}

	if math.Abs(sum) > 0.000001 {
		t.Errorf("splines do not accurately interpolte discount factors Z of yield curve; got: %v, expected: %v", sum, 0.0)
	}
}
