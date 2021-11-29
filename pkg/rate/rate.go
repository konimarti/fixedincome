package rate

import "math"

// Continuous converts an annual rate to a continuously compounded rate
// Exepects an annual rate i in percent (e.g. 3.0)
func Continuous(i float64) float64 {
	return math.Log(1+i/100.0) * 100.0
}

// Annual converts a continuously compounded rate to an annual rate
// Exepects an continuously compounded rate r in percent (e.g. 3.0)
func Annual(r float64) float64 {
	return (math.Exp(r/100.0) - 1.0) * 100.0
}

// EffectiveAnnual converts an annual interest rate into an effective rate due to compounding
// Exepects an annual rate in percent (e.g. 3.0) and compounding frequency n
func EffectiveAnnual(i float64, n int) float64 {
	return math.Pow(1+(i/100.0)/float64(n), float64(n))
}
