package bonds

import "math"

// Nelson-Siegel-Svensson implementation for spot rates
// Source: https://data.snb.ch/en/topics/ziredev#!/doc/explanations_ziredev#interest_rates_meth_par_siegel
type NelsonSiegelSvensson struct {
	B0 float64 `json:"b0"`
	B1 float64 `json:"b1"`
	B2 float64 `json:"b2"`
	B3 float64 `json:"b3"`
	T1 float64 `json:"t1"`
	T2 float64 `json:"t2"`
}

// RateContinuous returns the continuous compounded spot rate for maturity m in years in percent
func (n *NelsonSiegelSvensson) RateContinuous(m float64) float64 {
	rate_cc := n.B0
	rate_cc += n.B1 * ((1.0 - math.Exp(-m/n.T1)) * n.T1 / m)
	rate_cc += n.B2 * (((1.0 - math.Exp(-m/n.T1)) * n.T1 / m) - math.Exp(-m/n.T1))
	rate_cc += n.B3 * (((1.0 - math.Exp(-m/n.T2)) * n.T2 / m) - math.Exp(-m/n.T2))
	// rate_cc += n.B1 * ((1.0 - math.Exp(-m/n.T1)) / (m / n.T1))
	// rate_cc += n.B2 * (((1.0 - math.Exp(-m/n.T1)) / (m / n.T1)) - math.Exp(-m/n.T1))
	// rate_cc += n.B3 * (((1.0 - math.Exp(-m/n.T2)) / (m / n.T2)) - math.Exp(-m/n.T2))
	return rate_cc
}

// Rate returns the annually compounded spot rate for maturity m in years in percent
func (n *NelsonSiegelSvensson) Rate(m float64) float64 {
	return (math.Exp(n.RateContinuous(m)/100.0) - 1.0) * 100.0
}

// DContinuous return the discount factor to discount the cash flows for the given maturity m in years
func (n *NelsonSiegelSvensson) DContinuous(m float64) float64 {
	return math.Exp(-n.RateContinuous(m) / 100.0 * m)
}

// D return the discount factor to discount the cash flows for the given maturity m in years, zspread is the zero volatility spread in bps
func (n *NelsonSiegelSvensson) D(m float64, zspread float64, interestfrequency int) float64 {
	frequency := 1.0
	if interestfrequency > 0 {
		frequency = float64(interestfrequency)
	}
	return math.Pow(1.0+(n.Rate(m)/100.0+zspread/1e4)/frequency, -m*frequency)
}

// Forward rate
func (n *NelsonSiegelSvensson) Forward(m, t float64) float64 {
	rateM := n.Rate(m)
	rateMt := n.Rate(m + t)
	forward := math.Pow(math.Pow(1.0+rateMt, m+t)/math.Pow(1.0+rateM, m), 1.0/t)
	return forward
}
