package term

import "math"

// NelsonSiegelSvensson represents a spot-rate term structure
// Source: https://data.snb.ch/en/topics/ziredev#!/doc/explanations_ziredev#interest_rates_meth_par_siegel
type NelsonSiegelSvensson struct {
	B0 float64 `json:"b0"`
	B1 float64 `json:"b1"`
	B2 float64 `json:"b2"`
	B3 float64 `json:"b3"`
	T1 float64 `json:"t1"`
	T2 float64 `json:"t2"`
}

// r returns the continuous compounded spot rate in percent for a maturity of m years
func (nss *NelsonSiegelSvensson) r(m float64) float64 {
	cc := nss.B0
	cc += nss.B1 * ((1.0 - math.Exp(-m/nss.T1)) * nss.T1 / m)
	cc += nss.B2 * (((1.0 - math.Exp(-m/nss.T1)) * nss.T1 / m) - math.Exp(-m/nss.T1))
	cc += nss.B3 * (((1.0 - math.Exp(-m/nss.T2)) * nss.T2 / m) - math.Exp(-m/nss.T2))
	// cc += nss.B1 * ((1.0 - math.Exp(-m/nss.T1)) / (m / nss.T1))
	// cc += nss.B2 * (((1.0 - math.Exp(-m/nss.T1)) / (m / nss.T1)) - math.Exp(-m/nss.T1))
	// cc += nss.B3 * (((1.0 - math.Exp(-m/nss.T2)) / (m / nss.T2)) - math.Exp(-m/nss.T2))
	return cc
}

// z return the discount factor for a maturity of m years
func (nss *NelsonSiegelSvensson) z(m float64) float64 {
	return math.Exp(-nss.r(m) * 0.01 * m)
}

// Rate returns the annually compounded spot rate in percents for a maturity of m years
func (nss *NelsonSiegelSvensson) Rate(m float64) float64 {
	return (math.Exp(nss.r(m)*0.01) - 1.0) * 100.0
}

// Z return the discount factor to discount the cash flows for a maturity of m years.
// Spread is the static (zero-volatility) annual spread in bps
// coumpounding is the compounding frequency (if 0, set to 1 by default)
func (nss *NelsonSiegelSvensson) Z(m float64, spread float64, compounding int) float64 {
	n := 1.0
	if compounding > 0 {
		n = float64(compounding)
	}
	return math.Pow(1.0+(nss.Rate(m)*0.01+spread*0.0001)/n, -m*n)
}

// F is the forward discount factor at time T1=m years
// func (n *NelsonSiegelSvensson) F(m, t float64) float64 {
// 	return z(m+t) / z(m)
// }
