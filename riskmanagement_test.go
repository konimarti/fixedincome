package bonds_test

import (
	"math"
	"testing"
	"time"

	"github.com/konimarti/bonds"
	"github.com/konimarti/bonds/pkg/instrument/bond"
	"github.com/konimarti/bonds/pkg/maturity"
	"github.com/konimarti/bonds/pkg/term"
)

func TestInterestSensitivity(t *testing.T) {
	// define zero bond
	bond := bond.Straight{
		Schedule: maturity.Schedule{
			Settlement: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
			Maturity:   time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC),
			Frequency:  1,
			Basis:      "30E360",
		},
		Coupon:     0.0,
		Redemption: 100.0,
	}
	// define flat yield curve
	ts := term.Flat{2.0, 0.0}

	// calculate sensitivity
	pvbp := bonds.PVBP(&bond, &ts)
	// fmt.Println("duration=", bond.Duration(&ts))
	// fmt.Println("convexity=", bond.Convexity(&ts))

	ts2 := ts
	pvbpRef := bond.PresentValue(ts2.SetSpread(1.0)) - bond.PresentValue(&ts)

	if math.Abs(pvbp-pvbpRef) > 0.0001 {
		t.Errorf("pvbp calculation failed; got: %v, expected: %v", pvbp, pvbpRef)
	}
}
