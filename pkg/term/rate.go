package term

import "math"

// ToCC converts an annual rate to a continuously compounded rate
// Rates are given in percent (e.g. 3.0%)
func ToCC(r float64) float64 {
	return math.Log(1+r/100.0) * 100.0
}

// ToAnnaul converts a continuously compounded rate to an annual rate
// Rates are given in percent (e.g. 3.0%)
func ToAnnual(rcc float64) float64 {
	return (math.Exp(rcc/100.0) - 1.0) * 100.0
}
