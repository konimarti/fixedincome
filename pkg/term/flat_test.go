package term_test

import (
	"math"
	"testing"

	"github.com/konimarti/fixedincome/pkg/term"
)

func TestFlat(t *testing.T) {
	rate := 0.0125
	spread := 0.0010 // 10bp

	f := term.WithSpread(term.Flat{Rate: rate}, spread)

	if math.Abs(f.ZeroRate(0.0)-f.ZeroRate(math.Pi)) > 1e-15 {
		t.Errorf("yield curve is not flat")
	}
	if math.Abs(f.ZeroRate(math.Pi)-(rate+spread)) > 1e-15 {
		t.Errorf("rate is not correctly calculated")
	}
	if math.Abs(f.Discount(math.Pi)-math.Exp(-(rate+spread)*math.Pi)) > 1e-15 {
		t.Errorf("discount factor is not correctly calculated")
	}
	if term.InstForward(f, 3.0) != rate+spread {
		t.Errorf("instantaneous forward is not correctly calculated")
	}
}
