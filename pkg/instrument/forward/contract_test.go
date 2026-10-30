package forward_test

import (
	"math"
	"testing"
	"time"

	"github.com/konimarti/fixedincome/pkg/cashflow"
	"github.com/konimarti/fixedincome/pkg/instrument/forward"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/term"
)

func TestContract(t *testing.T) {
	ts := term.Flat{Rate: 0.02}
	forwardPrice := forward.ZeroBondPrice(1.0, 2.0, ts)
	contract := forward.Contract{
		K: forwardPrice,
		F: ts.Discount(2.0) / ts.Discount(1.0),
		T: 1.0,
	}
	if value := contract.PresentValue(ts); math.Abs(value) > 1e-15 {
		t.Errorf("wrong forward contract value; got: %v, expected: %v", value, 0.0)
	}
	contract.K = 0.9
	expected := (math.Exp(-0.02) - 0.9) * math.Exp(-0.02)
	if value := contract.PresentValue(ts); math.Abs(value-expected) > 1e-15 {
		t.Errorf("wrong forward contract value; got: %v, expected: %v", value, expected)
	}
}

func TestZeroBondPrice(t *testing.T) {
	ts := term.Flat{Rate: 0.02}
	forwardPrice := forward.ZeroBondPrice(1.0, 2.0, ts)
	expected := math.Exp(-0.02)
	if math.Abs(forwardPrice-expected) > 1e-15 {
		t.Errorf("wrong zero-bond forward price; got: %v, expected: %v", forwardPrice, expected)
	}
}

func TestFxPrice(t *testing.T) {
	// covered interest parity with flat curves
	usd, eur := term.Flat{Rate: 0.06}, term.Flat{Rate: 0.04}
	got := forward.FxPrice(1.2673, 2.0, usd, eur)
	expected := 1.2673 * math.Exp((0.06-0.04)*2.0)
	if math.Abs(got-expected) > 1e-14 {
		t.Errorf("got %v, expected %v", got, expected)
	}
}

func TestAssetPrice(t *testing.T) {
	got := forward.AssetPrice(100, 0.01, 0.5, term.Flat{Rate: 0.03})
	expected := 100 * math.Exp(0.02*0.5)
	if math.Abs(got-expected) > 1e-12 {
		t.Errorf("got %v, expected %v", got, expected)
	}
}

func TestFRA(t *testing.T) {
	ts := term.Flat{Rate: 0.02}
	settle := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	start, end := settle.AddDate(1, 0, 0), settle.AddDate(2, 0, 0) // 365 and 730 days
	fra, err := forward.NewFRA(1e6, 0, start, end, true)
	if err != nil {
		t.Fatal(err)
	}

	// ACT/360: 365 days
	tau := 365.0 / 360.0
	if got, _ := fra.AccrualFraction(); got != tau {
		t.Fatalf("accrual fraction %v, expected %v", got, tau)
	}
	fwd, err := fra.ForwardRate(settle, ts)
	if err != nil {
		t.Fatal(err)
	}
	expectedFwd := (math.Exp(0.02) - 1) / tau
	if math.Abs(fwd-expectedFwd) > 1e-15 {
		t.Errorf("forward rate %v, expected %v", fwd, expectedFwd)
	}

	// at the forward rate the FRA has zero value
	fra.FixedRate = fwd
	if value, err := fra.PresentValue(settle, ts, ts); err != nil || math.Abs(value) > 1e-9 {
		t.Errorf("wrong forward rate agreement value; got: %v, %v, expected: %v", value, err, 0.0)
	}

	// payer gains if the forward rate is above the fixed rate
	fra.FixedRate = 0.01
	expected := 1e6 * tau * (expectedFwd - 0.01) * math.Exp(-0.04)
	payer, _ := fra.PresentValue(settle, ts, ts)
	if math.Abs(payer-expected) > 1e-8 {
		t.Errorf("payer value %v, expected %v", payer, expected)
	}
	fra.Payer = false
	if receiver, _ := fra.PresentValue(settle, ts, ts); receiver != -payer {
		t.Errorf("receiver value %v != -payer %v", receiver, -payer)
	}

	// invalid: fixing in the past, end before start
	if _, err := fra.PresentValue(start.AddDate(0, 0, 1), ts, ts); err == nil {
		t.Errorf("expected error for start before valuation date")
	}
	bad := forward.FRA{Notional: 1, Start: end, End: start}
	if _, err := bad.PresentValue(settle, ts, ts); err == nil {
		t.Errorf("expected error for end before start")
	}
	// ACT/ACT is not defined for an FRA period
	bad = fra
	bad.DayCount = maturity.ActAct
	if _, err := bad.PresentValue(settle, ts, ts); err == nil {
		t.Errorf("expected error for ACTACT")
	}
	bad = fra
	bad.Notional = -1
	if _, err := bad.PresentValue(settle, ts, ts); err == nil {
		t.Errorf("expected error for negative notional")
	}
	// 30E/360 accrual: 360 days
	fra.DayCount = maturity.Thirty360E
	if got, _ := fra.AccrualFraction(); got != 1.0 {
		t.Errorf("30E360 accrual %v", got)
	}
	// projected cash flow: single settlement amount at End
	var p cashflow.Projected = fra
	cfs, err := p.CashFlows(settle, ts)
	if err != nil || len(cfs) != 1 || !cfs[0].Date.Equal(end) {
		t.Errorf("cash flows %+v, %v", cfs, err)
	}
}
