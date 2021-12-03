package rate_test

import (
	"math"
	"testing"

	"github.com/konimarti/bonds/pkg/rate"
)

var (
	annualRate           = 12.0
	monthlyEffectiveRate = math.Pow(1.01, 12)
	ccRate               = 11.332869
)

func TestContinuous(t *testing.T) {
	if math.Abs(rate.Continuous(annualRate, 1)-ccRate) > 1e-6 {
		t.Errorf("conversion from annual to cc rate failed")
	}
}

func TestAnnual(t *testing.T) {
	if math.Abs(rate.Annual(ccRate, 1)-annualRate) > 1e-6 {
		t.Errorf("conversion from cc to annual rate failed")
	}
}

func TestEffectiveAnnual(t *testing.T) {
	n := 12 // monthly compounding
	if math.Abs(rate.EffectiveAnnual(annualRate, n)-monthlyEffectiveRate) > 1e-6 {
		t.Errorf("conversion from cc to annual rate failed")
	}
}
