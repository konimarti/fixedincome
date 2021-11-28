package option

import (
	"math"

	"github.com/konimarti/bonds/pkg/term"
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

func (e *European) PresentValue(ts term.Structure) float64 {
	var value float64
	d1 := D1(e.S, e.K, e.T, e.Q, e.Vola, ts)
	d2 := D2(d1, e.T, e.Vola)
	z := math.Exp(-term.ToCC(ts.Rate(e.T)) / 100.0 * e.T)
	if e.Type == Call {
		value = e.S*math.Exp(-e.Q/100.0*e.T)*N(d1) - e.K*z*N(d2)
	} else if e.Type == Put {
		value = -e.S*math.Exp(-e.Q/100.0*e.T)*N(-d1) + e.K*z*N(-d2)
	}
	return value
}

func (e *European) SetVola(newVola float64) {
	e.Vola = newVola
}

func D1(S, K, T, Q, Vola float64, ts term.Structure) float64 {
	return (math.Log(S/K) + (term.ToCC(ts.Rate(T))/100.0-Q/100.0+math.Pow(Vola, 2.0)/2.0)*T) / (Vola * math.Sqrt(T))
}

func D2(d1, T, Vola float64) float64 {
	return d1 - Vola*math.Sqrt(T)
}

func N(x float64) float64 {
	if x < 0 {
		return 1.0 - N(-x)
	}
	return 0.5 * math.Erfc(-x/math.Sqrt2)
}
