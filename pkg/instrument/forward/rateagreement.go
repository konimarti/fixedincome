package forward

import "github.com/konimarti/bonds/pkg/term"

// RateAgreement (FRA) is a constract between two counterparties,
// according to which one counterparty agrees to pay the forward rate
// f_n(0,T_1,T_2) on a given notional amount N during a given future period of
// time from T_1 to T_2, while the other counterparty agrees
// to pay according to the future market floating rate r_n(T_1,T_2).
type RateAgreement struct {
	// N is the notional amount
	N float64

	// M is the number of securities in the long position to T_2
	// determined at t=0 in order to ensure that the FRA is 0 at initiation
	// M = Z(0,T_1) / Z(0,T_2)
	M float64

	// T1 is the beginning time for paying the forward rate
	T1 float64

	// T2 is the ending time for paying the forward rate
	T2 float64
}

// PresentValue calculated the value of the FRA at time t
// Value of FRA = N * [ M * Z(t,T_2) - Z(t,T_1) ]
func (fra *RateAgreement) PresentValue(ts term.Structure) float64 {
	return fra.N * (fra.M*ts.Z(fra.T2) - ts.Z(fra.T1))
}
