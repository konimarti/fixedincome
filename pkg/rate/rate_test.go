package rate_test

import (
	"math"
	"testing"

	"github.com/konimarti/fixedincome/pkg/rate"
)

var (
	annualRate           = 0.12
	monthlyEffectiveRate = math.Pow(1.01, 12) - 1.0
	ccRate               = 0.11332869
)

func TestToContinuous(t *testing.T) {
	got, err := rate.ToContinuous(annualRate, 1)
	if err != nil || math.Abs(got-ccRate) > 1e-8 {
		t.Errorf("conversion from annual to cc rate failed; got %v, %v", got, err)
	}
	// monthly compounding: 12 * ln(1.01)
	got, err = rate.ToContinuous(annualRate, 12)
	if err != nil || math.Abs(got-12*math.Log(1.01)) > 1e-14 {
		t.Errorf("monthly: got %v, %v", got, err)
	}
}

func TestFromContinuous(t *testing.T) {
	got, err := rate.FromContinuous(ccRate, 1)
	if err != nil || math.Abs(got-annualRate) > 1e-8 {
		t.Errorf("conversion from cc to annual rate failed; got %v, %v", got, err)
	}
	for _, n := range []int{1, 2, 4, 12, 360} {
		for _, r := range []float64{-0.05, 0.0, 0.01, 0.3} {
			rc, err := rate.ToContinuous(r, n)
			if err != nil {
				t.Fatal(err)
			}
			back, err := rate.FromContinuous(rc, n)
			if err != nil || math.Abs(back-r) > 1e-14 {
				t.Errorf("round trip n=%d r=%v: got %v, %v", n, r, back, err)
			}
		}
	}
}

func TestInvalid(t *testing.T) {
	if _, err := rate.ToContinuous(0.02, 0); err == nil {
		t.Errorf("expected error for n=0")
	}
	if _, err := rate.FromContinuous(0.02, -1); err == nil {
		t.Errorf("expected error for n<0")
	}
	if _, err := rate.ToContinuous(-1.0, 1); err == nil {
		t.Errorf("expected error for 1+r/n <= 0")
	}
	if _, err := rate.FromSimple(-2.0, 0.5); err == nil {
		t.Errorf("expected error for 1+rs*tau <= 0")
	}
}

func TestEffectiveAnnual(t *testing.T) {
	got, err := rate.EffectiveAnnual(annualRate, 12)
	if err != nil || math.Abs(got-monthlyEffectiveRate) > 1e-12 {
		t.Errorf("effective annual rate failed; got %v, %v, expected %v", got, err, monthlyEffectiveRate)
	}
}

func TestSimple(t *testing.T) {
	rc := 0.03
	rs := rate.ToSimple(rc, 0.25)
	if math.Abs(rs-(math.Exp(0.03*0.25)-1)/0.25) > 1e-15 {
		t.Errorf("ToSimple: got %v", rs)
	}
	back, err := rate.FromSimple(rs, 0.25)
	if err != nil || math.Abs(back-rc) > 1e-15 {
		t.Errorf("FromSimple: got %v, %v", back, err)
	}
	if rate.ToSimple(rc, 0) != rc {
		t.Errorf("ToSimple limit tau=0")
	}
}
