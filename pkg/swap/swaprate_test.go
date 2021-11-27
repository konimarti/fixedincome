package swap_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/konimarti/bonds/pkg/swap"
	"github.com/konimarti/bonds/pkg/term"
)

func TestSwapFxRate(t *testing.T) {
	// Example from John Heaton - Fiancnial Instruments class at Chicago Booth, 2014
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
	termEur := term.ConstantRate{(math.Exp(4.0/100.0) - 1.0) * 100.0, 0.0}
	termUsd := term.ConstantRate{(math.Exp(6.0/100.0) - 1.0) * 100.0, 0.0}

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

	fmt.Println("Swaprate", swapRate, "Expected", expectedRate)

	if math.Abs(swapRate-expectedRate) > 0.001 {
		t.Error("swap rate calculation is wrong")
	}

}
