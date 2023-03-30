package swap_test

import (
	"math"
	"testing"

	"github.com/konimarti/fixedincome/pkg/instrument/swap"
	"github.com/konimarti/fixedincome/pkg/term"
)

func TestFxRate(t *testing.T) {
	// Example from John Heaton - FinanciaFinancial Instruments class at Chicago Booth, 2014
	// Teaching Notes 2, page 49
	//
	// Example:
	// A US firm issues 100 million Euro-denominated 5 year note with coupon c=4%.
	// The firm exchanges the proceeds into $126.73 million at the current
	// exchange rate M0 = 1.2673$/Eur
	// Every 6 months, the firm must pay EUR 2 mil (100 mil * 4%/2). In addition,
	// at T=5, the firm must pay back Eur 100 mil principal

	// Plain vanilly FX swap to hedge exchange risk: what is swap rate K?

	// Assume rates are constant across maturities
	termEur := term.Flat{4.0, 0.0}
	termUsd := term.Flat{6.0, 0.0}

	m0 := 1.2673

	swapRate, err := swap.FxRate(m0,
		[]float64{2.0, 2.0, 2.0, 2.0, 2.0, 2.0, 2.0, 2.0, 2.0, 102.0},
		[]float64{0.5, 1.0, 1.5, 2.0, 2.5, 3.0, 3.5, 4.0, 4.5, 5.0},
		&termUsd,
		&termEur,
	)
	if err != nil {
		t.Error(err)
	}

	expectedRate := 1.389

	// fmt.Println("Swaprate", swapRate, "Expected", expectedRate)

	if math.Abs(swapRate-expectedRate) > 0.001 {
		t.Error("fx swap rate calculation is wrong; got:", swapRate, "expected:", expectedRate)
	}

}

func TestInterestRate(t *testing.T) {

	// calculating 5-year semi-annual CHF Swap Rate
	// IRS CHF 5Y (CH0002113865)

	// Parameters for CHF yield curve at Nov-30-2021
	// added 17bps z-spread for counterparty risk
	term := term.NelsonSiegelSvensson{
		-0.43381,
		-0.308942,
		4.83643,
		-4.10991,
		4.65211,
		3.33637,
		17.0,
	}

	maturities := []float64{
		3, 4, 5, 6, 7, 8, 9, 10,
	}
	expectedSwapRate := []float64{
		-0.50, -0.42, -0.35, -0.28, -0.21, -0.15, -0.10, -0.06,
	}

	for i, m := range maturities {
		tm := []float64{}
		for k := 0.5; k <= m; k += 0.5 {
			tm = append(tm, k)
		}
		swapRate, err := swap.InterestRate(tm, 2, &term)
		if err != nil {
			t.Error(err)
		}
		expectedRate := expectedSwapRate[i]
		if math.Abs(swapRate-expectedRate) > 0.02 {
			t.Error("interest swap rate calculation is wrong; maturity:", m, "got:", swapRate, "expected:", expectedRate)
		}
	}

}
