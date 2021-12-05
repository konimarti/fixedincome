package forward

import (
	"github.com/konimarti/bonds/pkg/term"
)

// Contract (forward contract) is a contract between two counterparties
// in which one counterparty agrees to purchase and the other agrees to sell
// a given security at a given future time, and at a given price,
// called the forward price.
type Contract struct {
	// K is the delivery price agreed upon at initiation
	K float64
	// F is the current forward price (F_t,T) at time t
	F float64
	// T is the remaining maturity of the forward contract (M=T-t)
	T float64
}

// PresentValue returns the value of the forward contract
func (f *Contract) PresentValue(ts term.Structure) float64 {
	return (f.F - f.K) * ts.Z(f.T)
}

//
// Calculate Forward Prices (F_t,T) at t=0
//

// ZeroBondPrice calculates the forward price for buying a zero-bond at time t with maturity m
func ZeroBondPrice(t, m float64, ts term.Structure) (float64, error) {
	return ts.Z(m) / ts.Z(t), nil
}

// // Fx calculates the forward rate for the currency pair (two term structure)
// // If currentFx is CHF/EUR, then tsLong should be CHF rats and tsShort should be EUR rates
// func Fx(currentFx, t float64, tsLong, tsShort term.Structure) (float64, error) {
// 	return currentFx * tsShort.Z(t) / tsLong.Z(t), nil
// }
//

//
// // StockPrice calculates the forward price for a stock with no dividends
// func StockPrice(currentPrice, t float64, ts term.Structure) (float64, error) {
// 	return currentPrice / ts.Z(t), nil
// }
