package swap_test

import (
	"math"
	"testing"
	"time"

	"github.com/konimarti/bonds/pkg/maturity"
	"github.com/konimarti/bonds/pkg/swap"
	"github.com/konimarti/bonds/pkg/term"
)

func TestSwapFxRate(t *testing.T) {
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

func TestSwapInterestRate(t *testing.T) {

	// calculating 5-year semi-annual CHF Swap Rate
	// IRS CHF 5Y (CH0002113865)

	// Parameters for CHF yield curve at Nov-30-2021
	// added 9bps z-spread for counterparty risk
	term := term.NelsonSiegelSvensson{
		-0.43381,
		-0.308942,
		4.83643,
		-4.10991,
		4.65211,
		3.33637,
		9.0,
	}

	maturities := []int{
		3, 4, 5, 6, 7, 8, 9, 10,
	}
	expectedSwapRate := []float64{
		-0.50, -0.42, -0.35, -0.28, -0.21, -0.15, -0.10, -0.06,
	}

	date := time.Date(2021, 12, 3, 0, 0, 0, 0, time.UTC)
	for i, m := range maturities {
		schedule := maturity.Schedule{
			Settlement: date,
			Maturity:   date.AddDate(m, 0, 0),
			Frequency:  2,
			Basis:      "30E360",
		}

		swapRate, err := swap.InterestRate(schedule, &term)
		if err != nil {
			t.Error(err)
		}

		expectedRate := expectedSwapRate[i]

		// fmt.Println("Swaprate", swapRate, "Expected", expectedRate)

		if math.Abs(swapRate-expectedRate) > 0.05 {
			t.Error("interest swap rate calculation is wrong; maturity:", m, "got:", swapRate, "expected:", expectedRate)
		}
	}

}
