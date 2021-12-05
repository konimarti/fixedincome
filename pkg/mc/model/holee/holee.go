package holee

import (
	"math"
	"math/rand"
	"time"

	"github.com/konimarti/bonds/pkg/term"
)

// HoLee implements the Ho-Lee interest rate model
type HoLee struct {
	// R0 is the initial rate (known today)
	R0 float64
	// Sigma is the standard deviation of the short term interest rate
	Sigma float64
	// T is the maturity (up to which to calculate the interes rates)
	T float64
	// N represents number of steps
	N int
	// Theta are the parameters of the Ho-Lee model
	Theta []float64
	// Rng is the random number generator (NormFloat64)
	Rng *rand.Rand
	// Payoff returns the discounted payoff for the given simulated rates
	Payoff func([]float64) float64
}

// New creates a new Ho-Lee model
func New(ts term.Structure, sigma, t float64, n int, payoff func([]float64) float64) (*HoLee, error) {
	hl := &HoLee{
		R0:     ts.Rate(t / float64(n)),
		Sigma:  sigma,
		T:      t,
		N:      n,
		Theta:  make([]float64, n),
		Rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
		Payoff: payoff,
	}
	err := Calibrate(hl, ts)
	return hl, err
}

// Calibrate calculates the parameters of the Ho-Lee model (theta's) to match the current yield curve
func Calibrate(hl *HoLee, ts term.Structure) error {
	n := hl.N
	dt := hl.T / float64(n)

	r := make([]float64, n+2)
	f := make([]float64, n+1)

	// calculate current rates, forward rates and thetas on the grid
	for i := 0; i < n+2; i += 1 {
		r[i] = ts.Rate(float64(i+1)*dt) / 100.0
	}
	for i := 0; i < n+1; i += 1 {
		// f[i] = r[i] + float64(i+1)*(r[i+1]-r[i])
		f[i] = -math.Log(ts.Z(float64(i+2)*dt)/ts.Z(float64(i+1)*dt)) / dt
	}
	for i := 0; i < n; i += 1 {
		hl.Theta[i] = (f[i+1]-f[i])/dt + math.Pow(hl.Sigma, 2.0)*float64(i+1)*dt
	}
	return nil
}

// Measurement implements the model interface for the Monte Carlo engine
func (hl *HoLee) Measurement() float64 {
	n := hl.N
	dt := hl.T / float64(n)
	rates := make([]float64, n)

	// simulate interest rates
	rates[0] = hl.R0 / 100.0
	for i := 0; i < (n - 1); i += 1 {
		rates[i+1] = rates[i] + hl.Theta[i]*dt + hl.Sigma*math.Sqrt(dt)*hl.Rng.NormFloat64()
	}
	return hl.Payoff(rates)
}
