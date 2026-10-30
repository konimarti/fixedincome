package fixedincome_test

import (
	"math"
	"testing"
	"time"

	"github.com/konimarti/fixedincome"
	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/term"
)

// TestSpread_SplineVsNss checks that the z-spread of a bond is the same for an
// NSS curve and a cubic spline built from the NSS discount factors
func TestSpread_SplineVsNss(t *testing.T) {
	nss := &term.NelsonSiegelSvensson{B0: -0.266372, B1: -0.471343, B2: 5.68789, B3: -5.12324, T1: 5.74881, T2: 4.14426}

	var m, z []float64
	for x := 0.05; x <= 15.0; x += 0.05 {
		m = append(m, x)
		z = append(z, nss.Z(x))
	}
	spline := term.NewSpline(m, z, 0.0)

	b := &bond.Straight{
		Schedule: maturity.Schedule{
			Settlement: time.Date(2021, 4, 1, 0, 0, 0, 0, time.UTC),
			Maturity:   time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC),
			Frequency:  1,
			Basis:      "30E360",
		},
		Coupon:     1.25,
		Redemption: 100.0,
	}

	// price the bond with a known spread of 50 bps on the NSS curve
	nss.SetSpread(50.0)
	price := b.PresentValue(nss)
	nss.SetSpread(0.0)

	spreadNss, err := fixedincome.Spread(price, b, nss)
	if err != nil {
		t.Fatal(err)
	}
	spreadSpline, err := fixedincome.Spread(price, b, spline)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(spreadNss-50.0) > 0.1 {
		t.Errorf("nss spread; got: %v, expected: %v", spreadNss, 50.0)
	}
	if math.Abs(spreadSpline-spreadNss) > 0.1 {
		t.Errorf("spline spread does not agree with nss spread; got: %v, expected: %v", spreadSpline, spreadNss)
	}
}

// TestSpread_RestoresCurve checks that Spread does not leave the trial spread
// on the caller's term structure
func TestSpread_RestoresCurve(t *testing.T) {
	b := &bond.Straight{
		Schedule: maturity.Schedule{
			Settlement: time.Date(2021, 4, 1, 0, 0, 0, 0, time.UTC),
			Maturity:   time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC),
			Frequency:  1,
			Basis:      "30E360",
		},
		Coupon:     1.25,
		Redemption: 100.0,
	}

	curves := []term.Structure{
		&term.Flat{R: 1.0, Spread: 7.0},
		&term.NelsonSiegelSvensson{B0: -0.266372, B1: -0.471343, B2: 5.68789, B3: -5.12324, T1: 5.74881, T2: 4.14426, Spread: 7.0},
		term.NewSpline([]float64{0.5, 1.0, 2.0, 5.0, 10.0}, []float64{0.995, 0.99, 0.98, 0.95, 0.9}, 7.0),
	}

	for _, ts := range curves {
		ref := ts.Rate(3.0)
		_, err := fixedincome.Spread(90.0, b, ts)
		if err != nil {
			t.Fatal(err)
		}
		if got := ts.Rate(3.0); got != ref {
			t.Errorf("%T: Spread modified the term structure; got rate %v, expected %v", ts, got, ref)
		}
	}
}

// TestIrr_HighYield checks that the IRR is found also for very high yields
// outside of the initial bracket
func TestIrr_HighYield(t *testing.T) {
	b := &bond.Straight{
		Schedule: maturity.Schedule{
			Settlement: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
			Maturity:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Frequency:  1,
			Basis:      "30E360",
		},
		Coupon:     5.0,
		Redemption: 100.0,
	}

	for _, price := range []float64{40.0, 10.0, 300.0} {
		irr, err := fixedincome.Irr(price, b)
		if err != nil {
			t.Errorf("price %v: %v", price, err)
			continue
		}
		// reprice independently: sum of 5 cash flows at t=1..5 discounted
		// with the continuously compounded yield
		pv := 0.0
		for k := 1; k <= 5; k++ {
			pv += 5.0 * math.Exp(-irr/100.0*float64(k))
		}
		pv += 100.0 * math.Exp(-irr/100.0*5.0)
		if math.Abs(pv-price) > 1e-4 {
			t.Errorf("price %v: irr %v reprices to %v", price, irr, pv)
		}
	}
}
