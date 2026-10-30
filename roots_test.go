package fixedincome_test

import (
	"math"
	"testing"

	"github.com/konimarti/fixedincome"
	"github.com/konimarti/fixedincome/pkg/instrument/option"
	"github.com/konimarti/fixedincome/pkg/term"
)

// zeroBond is a security paying 100 in one year
type zeroBond struct{}

func (zeroBond) PresentValue(ts term.Structure) float64 { return 100.0 * ts.Z(1.0) }

// nanSecurity is a security without a valid present value
type nanSecurity struct{}

func (nanSecurity) PresentValue(ts term.Structure) float64 { return math.NaN() }

// TestIrr_RootAtBracketEndpoint checks that a root exactly at the end of the
// search interval is returned (and not 0)
func TestIrr_RootAtBracketEndpoint(t *testing.T) {
	for _, r := range []float64{-20.0, 20.0} {
		price := zeroBond{}.PresentValue(&term.Flat{R: r})
		irr, err := fixedincome.Irr(price, zeroBond{})
		if err != nil {
			t.Errorf("rate %v: %v", r, err)
			continue
		}
		if math.Abs(irr-r) > 1e-6 {
			t.Errorf("got irr %v, expected %v", irr, r)
		}
	}
}

// TestSpread_RootAtBracketEndpoint checks the same for the z-spread
func TestSpread_RootAtBracketEndpoint(t *testing.T) {
	ts := &term.Flat{R: 0.0}
	price := zeroBond{}.PresentValue(&term.Flat{R: 0.0, Spread: 10000.0})
	spread, err := fixedincome.Spread(price, zeroBond{}, ts)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(spread-10000.0) > 1e-6 {
		t.Errorf("got spread %v, expected %v", spread, 10000.0)
	}
}

// TestIrr_NaN checks that an error is returned for securities without a valid
// present value
func TestIrr_NaN(t *testing.T) {
	if irr, err := fixedincome.Irr(100.0, nanSecurity{}); err == nil {
		t.Errorf("expected error, got irr %v", irr)
	}
	if spread, err := fixedincome.Spread(100.0, nanSecurity{}, &term.Flat{}); err == nil {
		t.Errorf("expected error, got spread %v", spread)
	}
}

// recordingOption records the smallest volatility it is evaluated with
type recordingOption struct {
	option.European
	minVola float64
}

func (o *recordingOption) SetVola(v float64) {
	if v < o.minVola {
		o.minVola = v
	}
	o.European.SetVola(v)
}

// TestImpliedVola_LowerBound checks that the option is never evaluated with
// a volatility below 1e-6
func TestImpliedVola_LowerBound(t *testing.T) {
	ts := &term.Flat{R: 2.0}
	o := &recordingOption{
		European: option.European{Type: option.Call, S: 110.0, K: 100.0, T: 2.0, Vola: 0.3},
		minVola:  math.Inf(1),
	}
	price := o.PresentValue(ts)
	vola, err := fixedincome.ImpliedVola(price, o, ts)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(vola-0.3) > 1e-6 {
		t.Errorf("got vola %v, expected 0.3", vola)
	}
	if o.minVola < 1e-6 {
		t.Errorf("option evaluated with volatility %v < 1e-6", o.minVola)
	}
}
