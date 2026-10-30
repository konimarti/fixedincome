package swap_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/konimarti/fixedincome/pkg/cashflow"
	"github.com/konimarti/fixedincome/pkg/instrument/swap"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/term"
)

func newSwap(start time.Time, years int, fixed float64) swap.InterestRateSwap {
	return swap.InterestRateSwap{
		Notional:      100.0,
		FixedRate:     fixed,
		FixedSchedule: maturity.Schedule{Issue: start, Maturity: start.AddDate(years, 0, 0), Frequency: maturity.Annual, DayCount: maturity.Thirty360E},
		FloatSchedule: maturity.Schedule{Issue: start, Maturity: start.AddDate(years, 0, 0), Frequency: maturity.SemiAnnual, DayCount: maturity.Act360},
		Payer:         true,
	}
}

func TestInterestRateSwap(t *testing.T) {
	// CHF yield curve at Nov-30-2021 with 17bps z-spread for counterparty
	// risk; quoted swap rates
	ts := term.WithSpread(chf20211130, 0.0017)

	maturities := []int{3, 4, 5, 6, 7, 8, 9, 10}
	swapRate := []float64{-0.0050, -0.0042, -0.0035, -0.0028, -0.0021, -0.0015, -0.0010, -0.0006}

	start := time.Date(2021, 12, 3, 0, 0, 0, 0, time.UTC)
	for i, m := range maturities {
		irs := newSwap(start, m, swapRate[i])
		value, err := irs.PresentValue(start, ts, ts)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(value) > 0.05 {
			t.Error("value of interest rate swap is wrong; maturity:", m, "got:", value, "expected: 0.0")
		}
	}
}

func TestInterestRateSwap_ParRate(t *testing.T) {
	ts := term.WithSpread(chf20211130, 0.0017)
	start := time.Date(2021, 12, 3, 0, 0, 0, 0, time.UTC)
	irs := newSwap(start, 5, 0)

	par, err := irs.ParRate(start, ts, ts)
	if err != nil {
		t.Fatal(err)
	}
	// single curve: par rate = (1 - Z(T)) / annuity
	annuity, err := irs.Annuity(start, ts)
	if err != nil {
		t.Fatal(err)
	}
	T := maturity.YearFraction(start, start.AddDate(5, 0, 0))
	if expected := 100 * (1 - ts.Discount(T)) / annuity; math.Abs(par-expected) > 1e-15 {
		t.Errorf("par rate %v, expected (1-Z(T))/annuity = %v", par, expected)
	}

	// zero value at the par rate, payer = -receiver
	irs.FixedRate = par
	if pv, _ := irs.PresentValue(start, ts, ts); math.Abs(pv) > 1e-12 {
		t.Errorf("swap at par rate has value %v", pv)
	}
	irs.FixedRate = par + 0.001
	payer, _ := irs.PresentValue(start, ts, ts)
	irs.Payer = false
	receiver, _ := irs.PresentValue(start, ts, ts)
	if payer >= 0 || receiver != -payer {
		t.Errorf("payer %v, receiver %v", payer, receiver)
	}

	// two curves and margin: PV(ParRate) = 0 also during the life of the
	// swap with a current fixing
	disc := chf20211130
	fwd := term.WithSpread(chf20211130, 0.002)
	irs.Margin = 0.001
	fixing := -0.007
	irs.CurrentFixing = &fixing
	settle := start.AddDate(0, 8, 10)
	par2, err := irs.ParRate(settle, disc, fwd)
	if err != nil {
		t.Fatal(err)
	}
	irs.FixedRate = par2
	if pv, _ := irs.PresentValue(settle, disc, fwd); math.Abs(pv) > 1e-12 {
		t.Errorf("swap at par rate during life has value %v", pv)
	}
	// matured swap
	if _, err := irs.ParRate(start.AddDate(6, 0, 0), disc, fwd); err == nil {
		t.Errorf("expected error for matured swap")
	}
	bad := irs
	bad.FixedSchedule.Frequency = 0
	if _, err := bad.PresentValue(start, disc, fwd); err == nil {
		t.Errorf("expected error for invalid schedule")
	}
	bad = irs
	bad.Notional = 0
	if _, err := bad.PresentValue(start, disc, fwd); err == nil {
		t.Errorf("expected error for zero notional")
	}
	// inside a floating period without fixing
	bad = irs
	bad.CurrentFixing = nil
	if _, err := bad.PresentValue(settle, disc, fwd); !errors.Is(err, swap.ErrNoFixing) {
		t.Errorf("expected ErrNoFixing, got %v", err)
	}
}

// TestInterestRateSwap_ParRateReference checks the par rate against a
// hand-computed value: flat 3% cc curve, 3y annual 30E/360 fixed leg from
// 2021-01-04 (accrual fractions 1, payment times 365/365, 730/365,
// 1095/365 = 1, 2, 3 years): (1 - e^-0.09) / (e^-0.03 + e^-0.06 + e^-0.09)
func TestInterestRateSwap_ParRateReference(t *testing.T) {
	c := term.Flat{Rate: 0.03}
	start := time.Date(2021, 1, 4, 0, 0, 0, 0, time.UTC)
	irs := newSwap(start, 3, 0)
	par, err := irs.ParRate(start, c, c)
	if err != nil {
		t.Fatal(err)
	}
	// computed independently (python): 0.030454533953516855
	if math.Abs(par-0.030454533953516855) > 1e-15 {
		t.Errorf("par rate %.17g, expected 0.030454533953516855", par)
	}
	annuity, _ := irs.Annuity(start, c)
	if math.Abs(annuity-100*2.826141252403985) > 1e-12 {
		t.Errorf("annuity %.17g", annuity)
	}
}

// TestInterestRateSwap_CashFlows checks the net (signed) cash flows
func TestInterestRateSwap_CashFlows(t *testing.T) {
	c := term.Flat{Rate: 0.03}
	start := time.Date(2021, 1, 4, 0, 0, 0, 0, time.UTC)
	irs := newSwap(start, 3, 0.03)
	var p cashflow.Projected = irs
	cfs, err := p.CashFlows(start, c)
	if err != nil {
		t.Fatal(err)
	}
	// semiannual float, annual fixed netted on the annual dates
	if len(cfs) != 6 {
		t.Fatalf("got %d cash flows: %+v", len(cfs), cfs)
	}
	pv, _ := irs.PresentValue(start, c, c)
	if math.Abs(cashflow.PV(cfs, c)-pv) > 1e-12 {
		t.Errorf("PV of cash flows %v != PresentValue %v", cashflow.PV(cfs, c), pv)
	}
	irs.Payer = false
	rcv, _ := irs.CashFlows(start, c)
	for i := range cfs {
		if rcv[i].Amount != -cfs[i].Amount {
			t.Errorf("receiver flows are not the negative payer flows")
		}
	}
	// the generic analytics work on the net flows (discount-curve
	// sensitivity with the forecast held fixed)
	if dv := cashflow.DV01(cfs, c); math.IsNaN(dv) || math.IsInf(dv, 0) {
		t.Errorf("DV01 not finite: %v", dv)
	}
}

// TestInterestRateSwap_ParRateFromTimes checks that the schedule-based par
// rate agrees with the simplified time grid within 0.1bp
func TestInterestRateSwap_ParRateFromTimes(t *testing.T) {
	ts := term.WithSpread(chf20211130, 0.0017)
	start := time.Date(2021, 12, 3, 0, 0, 0, 0, time.UTC)
	irs := newSwap(start, 5, 0)
	irs.FixedSchedule.Frequency = maturity.SemiAnnual
	par, _ := irs.ParRate(start, ts, ts)
	grid, _ := swap.ParRateFromTimes([]float64{0.5, 1, 1.5, 2, 2.5, 3, 3.5, 4, 4.5, 5}, 2, ts)
	if math.Abs(par-grid) > 0.1e-4 {
		t.Errorf("par rate %v vs grid %v", par, grid)
	}
}
