package term_test

import (
	"math"
	"testing"

	"github.com/konimarti/bonds/pkg/term"
)

func TestFlat(t *testing.T) {

	rate := 1.25
	spread := 10.0

	f := term.Flat{rate, spread}

	if math.Abs(f.Rate(0.0)-f.Rate(math.Pi)) > 0.00001 {
		t.Errorf("yield curve is not flat")
	}
	if math.Abs(f.Rate(math.Pi)-(rate+spread*0.01)) > 0.00001 {
		t.Errorf("rate is not correctly calculated")
	}
	if math.Abs(f.Z(math.Pi)-math.Exp(-(rate+spread*0.01)*0.01*math.Pi)) > 0.00001 {
		t.Errorf("discount factor is not correctly calculated")
	}
}
