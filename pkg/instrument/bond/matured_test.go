package bond_test

import (
	"math"
	"testing"
	"time"

	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/term"
)

// TestMatured checks that bonds that have matured have no value
func TestMatured(t *testing.T) {
	ts := &term.Flat{R: 2.0, Spread: 0.0}
	maturityDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, settlement := range []time.Time{maturityDate, maturityDate.AddDate(0, 0, 1), maturityDate.AddDate(1, 0, 0)} {
		schedule := maturity.Schedule{
			Settlement: settlement,
			Maturity:   maturityDate,
			Frequency:  1,
			Basis:      "ACTACT",
		}
		straight := bond.Straight{Schedule: schedule, Coupon: 5.0, Redemption: 100.0}
		floating := bond.Floating{Schedule: schedule, Rate: 5.0, Redemption: 100.0}

		for name, sec := range map[string]interface {
			PresentValue(term.Structure) float64
			Duration(term.Structure) float64
			Convexity(term.Structure) float64
		}{"straight": &straight, "floating": &floating} {
			if pv := sec.PresentValue(ts); pv != 0.0 {
				t.Errorf("%s settled %s: present value of matured bond; got %v, expected 0", name, settlement.Format("2006-01-02"), pv)
			}
			if d := sec.Duration(ts); d != 0.0 {
				t.Errorf("%s settled %s: duration of matured bond; got %v, expected 0", name, settlement.Format("2006-01-02"), d)
			}
			if c := sec.Convexity(ts); c != 0.0 {
				t.Errorf("%s settled %s: convexity of matured bond; got %v, expected 0", name, settlement.Format("2006-01-02"), c)
			}
		}
	}

	// one day before maturity: final coupon and redemption are discounted
	// over one day (ACTACT: 1/365 years)
	schedule := maturity.Schedule{
		Settlement: maturityDate.AddDate(0, 0, -1),
		Maturity:   maturityDate,
		Frequency:  1,
		Basis:      "ACTACT",
	}
	straight := bond.Straight{Schedule: schedule, Coupon: 5.0, Redemption: 100.0}
	floating := bond.Floating{Schedule: schedule, Rate: 5.0, Redemption: 100.0}
	expected := 105.0 * math.Exp(-0.02/365.0)
	if pv := straight.PresentValue(ts); math.Abs(pv-expected) > 1e-9 {
		t.Errorf("straight one day before maturity; got %v, expected %v", pv, expected)
	}
	if pv := floating.PresentValue(ts); math.Abs(pv-expected) > 1e-9 {
		t.Errorf("floating one day before maturity; got %v, expected %v", pv, expected)
	}
	if d := straight.Duration(ts); math.Abs(d+1.0/365.0) > 1e-9 {
		t.Errorf("straight duration one day before maturity; got %v, expected %v", d, -1.0/365.0)
	}
}
