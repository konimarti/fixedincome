package forward_test

import (
	"testing"
)

func TestFxForwadPrice(t *testing.T) {
	// termEur := term.NelsonSiegelSvensson{
	// 	0.294049,
	// 	-1.213049,
	// 	1.098431,
	// 	-2.523094,
	// 	1.847069,
	// 	2.154085,
	// 	0.0,
	// }
	// termChf := term.NelsonSiegelSvensson{
	// 	-0.352323,
	// 	-0.392947,
	// 	5.34703,
	// 	-3.93181,
	// 	4.8696,
	// 	3.87489,
	// 	0.0,
	// }
	// currentFx := 1.0457 // Eur/CHF
	//
	// // calculate maturities for next 12 months
	// today := time.Date(2021, 11, 26, 0, 0, 0, 0, time.UTC)
	// maturities := []float64{}
	// for i := 1; i <= 12; i++ {
	// 	maturities = append(maturities, maturity.DifferenceInYears(today, today.AddDate(0, i, 0)))
	// }
	//
	// // calculate forwad fx rates
	// for i, m := range maturities {
	// 	fwdFxPrice, err := forward.Fx(currentFx, m, &termEur, &termChf)
	// 	if err != nil {
	// 		t.Errorf("failed to calculate forward FX price")
	// 	}
	// 	fmt.Println("Month", i+1, "Forward Price", fwdFxPrice, (fwdFxPrice-currentFx)*1e4)
	// }
}

func TestZeroBondForwardPrice(t *testing.T) {
	// t.Errorf("not implemented yet")
}

func TestStockForwardPrice(t *testing.T) {
	// t.Errorf("not implemented yet")
}
