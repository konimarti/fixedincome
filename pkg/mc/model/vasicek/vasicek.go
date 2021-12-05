package vasicek

import (
	"math"
	"math/rand"
	"time"

	"github.com/konimarti/bonds/pkg/term"
	"gonum.org/v1/gonum/optimize"
)

// Vasicek implements the basic Vasicek interest rate model
type Vasicek struct {
	// R0
	R0 float64
	// Rbar
	Rbar float64
	// Gamma
	Gamma float64
	// Sigma is the standard deviation of the first difference of the short term interest rate
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

// New creates a new Vasicek model
func New(ts term.Structure, sigma, t float64, n int, payoff func([]float64) float64) (*Vasicek, error) {
	v := &Vasicek{
		R0:     ts.Rate(t/float64(n)) / 100.0,
		Rbar:   ts.Rate(t) / 100.0,
		Gamma:  1.0,
		Sigma:  sigma,
		T:      t,
		N:      n,
		Rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
		Payoff: payoff,
	}
	err := Calibrate(v, ts)
	return v, err
}

// Calibrate calculates the parameters of the Vasicek model
func Calibrate(v *Vasicek, ts term.Structure) error {

	// initial estimate
	x := []float64{
		// rbar
		v.Rbar,
		// gamma
		v.Gamma,
	}

	// least-squares function to optimize Vasicek parameters
	fun := func(x []float64) float64 {
		v.Rbar, v.Gamma = x[0], x[1]
		sum := 0.0
		dt := v.T / float64(v.N)
		for t := dt; t < v.T*1.5; t += dt {
			sum += math.Pow(v.Z(v.R0, 0.0, t)-ts.Z(t), 2.0)
		}
		return sum
	}

	// setup problem
	p := optimize.Problem{
		Func: fun,
	}

	result, err := optimize.Minimize(p, x, nil, nil)
	if err != nil {
		return err
	}
	if err = result.Status.Err(); err != nil {
		return err
	}
	// fmt.Printf("result.Status: %v\n", result.Status)
	// fmt.Printf("result.X: %0.4g\n", result.X)
	// fmt.Printf("result.F: %0.4g\n", result.F)
	// fmt.Printf("result.Stats.FuncEvaluations: %d\n", result.Stats.FuncEvaluations)

	// copy results to Vasicek model
	v.Rbar, v.Gamma = result.X[0], result.X[1]

	// fmt.Println(v.Z(v.R0, 0.0, 4.0), ts.Z(4.0))
	// fmt.Println(v.Z(v.R0, 0.0, 5.0), ts.Z(5.0))
	// fmt.Println(v.Z(v.R0, 0.0, 6.0), ts.Z(6.0))

	return nil
}

// Z returns the Vasicek discount factor Z(r;t,T)
// Source: P. Veronesi, Fixed Income Securities, p. 539, Eq. 15.28
func (v *Vasicek) Z(r, t, T float64) float64 {
	B := 1.0 / v.Gamma * (1 - math.Exp(-v.Gamma*(T-t)))
	A := (B-(T-t))*(v.Rbar-math.Pow(v.Sigma, 2.0)/(2.0*math.Pow(v.Gamma, 2.0))) - math.Pow(v.Sigma*B, 2.0)/(4.0*v.Gamma)
	return math.Exp(A - B*r)
}

// Measurement implements the model interface for the Monte Carlo engine
func (v *Vasicek) Measurement() float64 {
	n := v.N
	dt := v.T / float64(n)
	rates := make([]float64, n)

	// simulate interest rates
	rates[0] = v.R0
	for i := 0; i < (n - 1); i += 1 {
		rates[i+1] = rates[i] + v.Gamma*(v.Rbar-rates[i])*dt + v.Sigma*math.Sqrt(dt)*v.Rng.NormFloat64()
	}
	return v.Payoff(rates)
}
