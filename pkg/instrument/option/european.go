package option

import (
	"math"

	"github.com/konimarti/fixedincome/pkg/term"
)

const (
	Call int = iota
	Put
)

// European is the implementation of plain vanilla European option
type European struct {
	// Type is the type of the option (call=0, put=1)
	Type int
	// S is the price of the underlying asset
	S float64
	// K is the strike price
	K float64
	// T is remaining maturity in years
	T float64
	// Q is the dividend yield in percent
	Q float64
	// Vola is the volatility of the underlying asset
	Vola float64
}

// Presentvalues implements the Black-Scholes pricing for European call and put options
func (e *European) PresentValue(ts term.Structure) float64 {
	var value float64
	d1 := D1(e.S, e.K, e.T, e.Q, e.Vola, ts)
	d2 := D2(d1, e.T, e.Vola)
	z := ts.Z(e.T)
	if e.Type == Call {
		value = e.S*math.Exp(-e.Q/100.0*e.T)*N(d1) - e.K*z*N(d2)
	} else if e.Type == Put {
		value = -e.S*math.Exp(-e.Q/100.0*e.T)*N(-d1) + e.K*z*N(-d2)
	}
	return value
}

// SetVola sets the volatility (needed for the calculation of the implied volatility)
func (e *European) SetVola(newVola float64) {
	e.Vola = newVola
}

// implement the 'Greeks'

// Delta
func (e *European) Delta(ts term.Structure) float64 {
	d1 := D1(e.S, e.K, e.T, e.Q, e.Vola, ts)
	sign := 1.0
	if e.Type == Put {
		sign = -1.0
	}
	return sign * math.Exp(-e.Q*e.T) * N(sign*d1)
}

// Gamma
func (e *European) Gamma(ts term.Structure) float64 {
	d1 := D1(e.S, e.K, e.T, e.Q, e.Vola, ts)
	return math.Exp(-e.Q*e.T) * Napostroph(d1) / (e.S * e.Vola * math.Sqrt(e.T))
}

// Rho
func (e *European) Rho(ts term.Structure) float64 {
	d1 := D1(e.S, e.K, e.T, e.Q, e.Vola, ts)
	d2 := D2(d1, e.T, e.Vola)
	sign := 1.0
	if e.Type == Put {
		sign = -1.0
	}
	return sign * e.K * e.T * math.Exp(-(ts.Rate(e.T)/100.0)*e.T) * N(sign*d2)
}

// Vega
func (e *European) Vega(ts term.Structure) float64 {
	d1 := D1(e.S, e.K, e.T, e.Q, e.Vola, ts)
	return math.Exp(-e.Q*e.T) * e.S * math.Sqrt(e.T) * Napostroph(d1)
}

// helper function for Black Scholes formula

// D1
func D1(S, K, T, Q, Vola float64, ts term.Structure) float64 {
	return (math.Log(S/K) + (ts.Rate(T)/100.0-Q/100.0+math.Pow(Vola, 2.0)/2.0)*T) / (Vola * math.Sqrt(T))
}

// D2
func D2(d1, T, Vola float64) float64 {
	return d1 - Vola*math.Sqrt(T)
}

// N
func N(x float64) float64 {
	if x < 0 {
		return 1.0 - N(-x)
	}
	return 0.5 * math.Erfc(-x/math.Sqrt2)
}

// Napostroph
func Napostroph(x float64) float64 {
	return math.Exp(-(x*x)/2.0) / (math.SqrtPi * math.Sqrt2)
}
