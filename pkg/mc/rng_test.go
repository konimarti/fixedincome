package mc

import (
	"math"
	"testing"
)

// TestStreams checks that the per-block streams of a seed behave like
// independent N(0,1) sources: the standardised block sums are N(0,1) and
// neighbouring streams are uncorrelated.
func TestStreams(t *testing.T) {
	const seeds, streams, draws = 200, 16, 512
	var s1, s2, cross float64
	n := 0
	prev := make([]float64, draws)
	cur := make([]float64, draws)
	for seed := uint64(1); seed <= seeds; seed++ {
		for b := uint64(0); b < streams; b++ {
			rng := NewRand(seed, b)
			sum := 0.0
			for i := range cur {
				cur[i] = rng.NormFloat64()
				sum += cur[i]
			}
			z := sum / math.Sqrt(draws)
			s1 += z
			s2 += z * z
			n++
			if b > 0 {
				c := 0.0
				for i := range cur {
					c += cur[i] * prev[i]
				}
				cross += c / math.Sqrt(draws)
			}
			cur, prev = prev, cur
		}
	}
	mean := s1 / float64(n)
	v := s2/float64(n) - mean*mean
	// n = 3200 samples: SE of the mean 0.018, of the variance ~0.025
	if math.Abs(mean) > 4/math.Sqrt(float64(n)) || math.Abs(v-1) > 0.1 {
		t.Errorf("block sums: mean %v, variance %v", mean, v)
	}
	nc := float64(seeds * (streams - 1))
	if c := cross / nc; math.Abs(c) > 4/math.Sqrt(nc) {
		t.Errorf("cross-stream correlation statistic %v", c)
	}
	// same (seed, stream) gives the same sequence
	a, b := NewRand(5, 3), NewRand(5, 3)
	for i := 0; i < 10; i++ {
		if a.Uint64() != b.Uint64() {
			t.Fatal("not reproducible")
		}
	}
}
