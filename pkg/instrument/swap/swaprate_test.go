package swap_test

import (
	"math"
	"sort"
	"testing"

	"github.com/konimarti/fixedincome/pkg/instrument/swap"
	"github.com/konimarti/fixedincome/pkg/term"
)

// chf20211130 are the NSS parameters (decimal) for the CHF yield curve at
// Nov-30-2021
var chf20211130 = term.NSS{
	B0: -0.0043381, B1: -0.00308942, B2: 0.0483643, B3: -0.0410991,
	Tau1: 4.65211, Tau2: 3.33637,
}

func TestFxRate(t *testing.T) {
	// Example from John Heaton - Financial Instruments class at Chicago Booth, 2014
	// Teaching Notes 2, page 49
	//
	// A US firm issues 100 million Euro-denominated 5 year note with coupon c=4%.
	// The firm exchanges the proceeds into $126.73 million at the current
	// exchange rate M0 = 1.2673$/Eur
	// Every 6 months, the firm must pay EUR 2 mil (100 mil * 4%/2). In addition,
	// at T=5, the firm must pay back Eur 100 mil principal
	//
	// Plain vanilla FX swap to hedge exchange risk: what is swap rate K?
	// Assume rates are constant across maturities
	termEur := term.Flat{Rate: 0.04}
	termUsd := term.Flat{Rate: 0.06}

	swapRate, err := swap.FxRate(1.2673,
		[]float64{2.0, 2.0, 2.0, 2.0, 2.0, 2.0, 2.0, 2.0, 2.0, 102.0},
		[]float64{0.5, 1.0, 1.5, 2.0, 2.5, 3.0, 3.5, 4.0, 4.5, 5.0},
		termUsd,
		termEur,
	)
	if err != nil {
		t.Error(err)
	}
	expectedRate := 1.389
	if math.Abs(swapRate-expectedRate) > 0.001 {
		t.Error("fx swap rate calculation is wrong; got:", swapRate, "expected:", expectedRate)
	}
	if _, err := swap.FxRate(1.0, []float64{1}, []float64{1, 2}, termUsd, termEur); err == nil {
		t.Errorf("expected error for unequal lengths")
	}
}

func TestParRateFromTimes(t *testing.T) {
	// calculating 5-year semi-annual CHF Swap Rate
	// IRS CHF 5Y (CH0002113865)
	// added 17bps z-spread for counterparty risk
	ts := term.WithSpread(chf20211130, 0.0017)

	maturities := []float64{3, 4, 5, 6, 7, 8, 9, 10}
	expectedSwapRate := []float64{-0.0050, -0.0042, -0.0035, -0.0028, -0.0021, -0.0015, -0.0010, -0.0006}

	for i, m := range maturities {
		tm := []float64{}
		for k := 0.5; k <= m; k += 0.5 {
			tm = append(tm, k)
		}
		swapRate, err := swap.ParRateFromTimes(tm, 2, ts)
		if err != nil {
			t.Error(err)
		}
		if math.Abs(swapRate-expectedSwapRate[i]) > 0.0002 {
			t.Error("interest swap rate calculation is wrong; maturity:", m, "got:", swapRate, "expected:", expectedSwapRate[i])
		}
	}
}

// TestParRateFromTimes_Order checks that the swap rate does not depend on the
// order of the times
func TestParRateFromTimes_Order(t *testing.T) {
	ts := term.NSS{B0: 0.02, B1: -0.01, B2: 0.01, B3: 0.005, Tau1: 2.0, Tau2: 5.0}
	descending := []float64{5, 4.5, 4, 3.5, 3, 2.5, 2, 1.5, 1, 0.5}
	ascending := append([]float64{}, descending...)
	sort.Float64s(ascending)

	rateDesc, err := swap.ParRateFromTimes(descending, 2, ts)
	if err != nil {
		t.Fatal(err)
	}
	rateAsc, err := swap.ParRateFromTimes(ascending, 2, ts)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(rateDesc-rateAsc) > 1e-15 {
		t.Errorf("swap rate depends on order of maturities; descending: %v, ascending: %v", rateDesc, rateAsc)
	}
	if _, err := swap.ParRateFromTimes(nil, 2, ts); err == nil {
		t.Errorf("expected error for no times")
	}
}
