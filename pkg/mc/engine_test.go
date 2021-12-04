package mc_test

import (
	"math"
	"math/rand"
	"testing"

	"github.com/konimarti/bonds/pkg/mc"
)

type pi struct {
	Rng *rand.Rand
}

func NewPi() pi {
	rng := rand.New(rand.NewSource(99))
	return pi{rng}
}

func (p pi) Measurement() float64 {
	x, y := p.Rng.Float64(), p.Rng.Float64()
	if x*x+y*y <= 1.0 {
		return 1.0
	}
	return 0.0
}

func TestMonteCarloEngine_PI(t *testing.T) {
	engine := mc.New(
		NewPi(),
		1e6,
	)
	err := engine.Run()
	if err != nil {
		t.Error(err)
	}
	average, err := engine.Estimate()
	if err != nil {
		t.Error(err)
	}
	if math.Abs(average-math.Pi/4.0) > 0.001 {
		t.Errorf("monte carlo estimate failed; got: %v, expected: %v", average, math.Pi/4.0)
	}
	stderror, err := engine.StdError()
	if err != nil {
		t.Error(err)
	}
	if math.Abs(stderror-0.0004) > 0.0001 {
		t.Errorf("monte carlo stderror failed; got: %v, expected: %v", stderror, 0.0004)
	}
	lower, upper, err := engine.CI()
	if err != nil {
		t.Error(err)
	}
	if math.Abs((lower-average)/stderror-(-1.96)) > 0.001 {
		t.Errorf("monte carlo lower CI failed; got: %v, expected: %v", lower, -1.96)
	}
	if math.Abs((upper-average)/stderror-1.96) > 0.001 {
		t.Errorf("monte carlo upper CI failed; got: %v, expected: %v", upper, 1.96)
	}

}
