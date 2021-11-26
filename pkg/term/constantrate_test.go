package term_test

import (
	"math"
	"testing"

	"github.com/konimarti/bonds/pkg/term"
)

func TestConstantRate(t *testing.T) {
	rate := 1.25

	c := term.ConstantRate{rate, 0.0}

	coupon := 1.0
	dcf := 0.0
	for i := 0; i < 20; i++ {
		if c.Rate(float64(i)) != rate {
			t.Errorf("rates don't match for maturity %d", i)
		}
		dcf += coupon * c.Z(float64(i), 1)
		coupon = coupon * (1 + rate/100.0)
	}

	if math.Abs(dcf-20.0) > 0.00001 {
		t.Errorf("discount factors are not correct")
	}
}
