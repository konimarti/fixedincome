package forward

import (
	"github.com/konimarti/bonds/pkg/term"
)

type Forward struct {
	// K is the forwad price at initiation (F_0,T) at time 0
	K float64
	// F is the current forward price (F_t,T) at time t
	F float64
	// M is the remaining maturity of the forward contract (M=T-t)
	M float64
}

// PresentValue returns the value of the forward contract
func (f *Forward) PresentValue(ts term.Structure) float64 {
	return (f.K - f.F) * ts.Z(f.M)
}

//
// Calculate Forward Prices (F_t,T)
//

// Fx calculates the forward rate for the currency pair (two term structure)
// If currentFx is CHF/EUR, then tsLong should be CHF rats and tsShort should be EUR rates
func Fx(currentFx, t float64, tsLong, tsShort term.Structure) (float64, error) {
	return currentFx * tsShort.Z(t) / tsLong.Z(t), nil
}

// ZeroBond calculates the forward price for buying a zero-bond at time t with maturity m
func ZeroBond(t, m float64, ts term.Structure) (float64, error) {
	return ts.Z(m) / ts.Z(t), nil
}

// Stock calculates the forward price for a stock with no dividends
func Stock(currentPrice, t float64, ts term.Structure) (float64, error) {
	return currentPrice / ts.Z(t), nil
}

// func StockDividendYield(stockPrice, dividendYield, maturity float64, ts term.Structure) (float64, error) {
// 	return stockPrice / ts.SetSpread(-dividendYield).Z(maturity, 1), nil
// }
//
// func StockDividendDate(stockPrice, dividendAmount, dividendMaturity, maturity float64, ts term.Structure) (float64, error) {
// 	if dividendMaturity > maturity {
// 		return 0.0, fmt.Errorf("dividend date is after maturity of forward")
// 	}
// 	return (stockPrice - dividendAmount*ts.Z(dividendMaturity, 1)) / ts.Z(maturity, 1), nil
// }
