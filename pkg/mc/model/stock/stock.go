package stock

import (
	"math"
	"math/rand"
	"time"
)

type Stock struct {
	// R is annual return until maturity
	R float64
	// S0 is the stock value at the time 0
	S0 float64
	// Sigma is the standard deviation of the stock
	Sigma float64
	// T is the maturity (up to which to calculate the interes rates)
	T float64
	// N represents number of steps
	N int
	// Rng is the random number generator (NormFloat64)
	Rng *rand.Rand
	// Payoff returns the discounted payoff for the given simulated rates
	Payoff func([]float64) float64
}

func New(r, s0, sigma, t float64, n int, payoff func([]float64) float64) *Stock {
	s := &Stock{
		R:      r,
		S0:     s0,
		Sigma:  sigma,
		T:      t,
		N:      n,
		Rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
		Payoff: payoff,
	}
	return s
}

func (s *Stock) Measurement() float64 {
	n := s.N
	dt := s.T / float64(n)
	stockValues := make([]float64, n)

	// simulate interest rates
	stockValues[0] = s.S0
	for i := 0; i < (n - 1); i += 1 {
		stockValues[i+1] = stockValues[i] * math.Exp((s.R-math.Pow(s.Sigma, 2.0)/2.0)*dt+s.Sigma*math.Sqrt(dt)*s.Rng.NormFloat64())
	}
	return s.Payoff(stockValues)
}
