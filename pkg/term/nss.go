package term

import "math"

// NelsonSiegelSvensson represents a spot-rate term structure
// Source: https://data.snb.ch/en/topics/ziredev#!/doc/explanations_ziredev#interest_rates_meth_par_siegel
type NelsonSiegelSvensson struct {
	B0     float64 `json:"b0"`
	B1     float64 `json:"b1"`
	B2     float64 `json:"b2"`
	B3     float64 `json:"b3"`
	T1     float64 `json:"t1"`
	T2     float64 `json:"t2"`
	Spread float64 `json:"spread"`
}

// SetSpread sets the constant spread that is added to the continuously
// compounded rate over all maturities
func (nss *NelsonSiegelSvensson) SetSpread(s float64) Structure {
	nss.Spread = s
	return nss
}

// Rate returns the continuous compounded spot rate (in %) for a term maturity
// of m years R_cc(0, m)
func (nss *NelsonSiegelSvensson) Rate(m float64) float64 {
	if m == 0.0 {
		m = 1e-7
	}
	cc := nss.B0
	cc += nss.B1 * ((1.0 - math.Exp(-m/nss.T1)) * nss.T1 / m)
	cc += nss.B2 * (((1.0 - math.Exp(-m/nss.T1)) * nss.T1 / m) - math.Exp(-m/nss.T1))
	cc += nss.B3 * (((1.0 - math.Exp(-m/nss.T2)) * nss.T2 / m) - math.Exp(-m/nss.T2))
	// cc += nss.B1 * ((1.0 - math.Exp(-m/nss.T1)) / (m / nss.T1))
	// cc += nss.B2 * (((1.0 - math.Exp(-m/nss.T1)) / (m / nss.T1)) - math.Exp(-m/nss.T1))
	// cc += nss.B3 * (((1.0 - math.Exp(-m/nss.T2)) / (m / nss.T2)) - math.Exp(-m/nss.T2))
	return cc + nss.Spread*0.01
}

// Z return the discount factor for a term maturity of m years Z(0, m)
func (nss *NelsonSiegelSvensson) Z(m float64) float64 {
	return math.Exp(-nss.Rate(m) * 0.01 * m)
}

// F is the forward discount factor F(0, m, m+t) for a zero-bond with maturity
// t at the future time t
// func (n *NelsonSiegelSvensson) F(m, t float64) float64 {
// 	return n.Z(m+t) / n.Z(m)
// }
