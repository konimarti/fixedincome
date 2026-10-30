package bond_test

import (
	"math"
	"testing"

	"github.com/konimarti/fixedincome/pkg/cashflow"
	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/term"
)

// TestMatured checks that bonds that have matured have no value
func TestMatured(t *testing.T) {
	ts := term.Flat{Rate: 0.02}
	maturityDate := date(2026, 1, 1)
	schedule := maturity.Schedule{
		Maturity:  maturityDate,
		Frequency: maturity.Annual,
		DayCount:  maturity.ActAct,
	}
	straight := mustStraight(t, schedule, 0.05)
	floating := mustFloating(t, schedule, 0, bond.WithFixing(0.05))

	for _, settlement := range []string{"on", "after", "year after"} {
		settle := maturityDate
		switch settlement {
		case "after":
			settle = maturityDate.AddDate(0, 0, 1)
		case "year after":
			settle = maturityDate.AddDate(1, 0, 0)
		}
		cfs, err := straight.CashFlows(settle)
		if err != nil {
			t.Fatal(err)
		}
		if len(cfs) != 0 {
			t.Errorf("straight settled %s maturity: got cash flows %v", settlement, cfs)
		}
		if pv, _ := straight.DirtyPrice(settle, ts); pv != 0.0 {
			t.Errorf("straight settled %s maturity: present value %v, expected 0", settlement, pv)
		}
		if a, _ := straight.Accrued(settle); a != 0.0 {
			t.Errorf("straight settled %s maturity: accrued %v, expected 0", settlement, a)
		}
		if d := cashflow.Duration(cfs, ts); d != 0.0 {
			t.Errorf("straight settled %s maturity: duration %v, expected 0", settlement, d)
		}
		if c := cashflow.Convexity(cfs, ts); c != 0.0 {
			t.Errorf("straight settled %s maturity: convexity %v, expected 0", settlement, c)
		}
		if pv, _ := floating.PresentValue(settle, ts, ts); pv != 0.0 {
			t.Errorf("floating settled %s maturity: present value %v, expected 0", settlement, pv)
		}
		if d, _ := bond.FloatingDuration(floating, settle, ts, ts); d != 0.0 {
			t.Errorf("floating settled %s maturity: duration %v, expected 0", settlement, d)
		}
		if c, _ := bond.FloatingConvexity(floating, settle, ts, ts); c != 0.0 {
			t.Errorf("floating settled %s maturity: convexity %v, expected 0", settlement, c)
		}
	}

	// one day before maturity: final coupon and redemption are discounted
	// over one day (1/365 years)
	settle := maturityDate.AddDate(0, 0, -1)
	expected := 105.0 * math.Exp(-0.02/365.0)
	if pv, _ := straight.DirtyPrice(settle, ts); math.Abs(pv-expected) > 1e-9 {
		t.Errorf("straight one day before maturity; got %v, expected %v", pv, expected)
	}
	if pv, _ := floating.PresentValue(settle, ts, ts); math.Abs(pv-expected) > 1e-9 {
		t.Errorf("floating one day before maturity; got %v, expected %v", pv, expected)
	}
	cfs, _ := straight.CashFlows(settle)
	if d := cashflow.Duration(cfs, ts); math.Abs(d-1.0/365.0) > 1e-12 {
		t.Errorf("straight duration one day before maturity; got %v, expected %v", d, 1.0/365.0)
	}
}
