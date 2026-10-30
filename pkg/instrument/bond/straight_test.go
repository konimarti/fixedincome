package bond_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/konimarti/fixedincome/pkg/cashflow"
	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/term"
)

func date(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}

func mustStraight(t testing.TB, s maturity.Schedule, coupon float64, opts ...bond.Option) bond.Straight {
	t.Helper()
	b, err := bond.NewStraight(s, coupon, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// snb20210331 are the NSS parameters for CH govt bonds as of 2021-03-31
var snb20210331 = term.NSS{
	B0: -0.00266372, B1: -0.00471343, B2: 0.0568789, B3: -0.0512324,
	Tau1: 5.74881, Tau2: 4.14426,
}

func TestStraight_PresentValue(t *testing.T) {
	// ISIN CH0224396983 (quote per 2021-04-01)
	b := mustStraight(t, maturity.Schedule{
		Maturity:  date(2026, 5, 28),
		Frequency: maturity.Annual,
	}, 0.0125)
	clean, err := b.CleanPrice(date(2021, 4, 1), snb20210331)
	if err != nil {
		t.Fatal(err)
	}
	expected := 109.70
	if math.Abs(clean-expected) > 0.05 {
		t.Errorf("got %f, expected %f", clean, expected)
	}
}

func TestStraight_CashFlows(t *testing.T) {
	settle := date(2021, 4, 1)
	b := mustStraight(t, maturity.Schedule{Maturity: date(2023, 5, 28), Frequency: maturity.SemiAnnual}, 0.04,
		bond.WithFace(1000), bond.WithRedemption(1010))
	cfs, err := b.CashFlows(settle)
	if err != nil {
		t.Fatal(err)
	}
	expected := []cashflow.CashFlow{
		{Date: date(2021, 5, 28), T: 57.0 / 365.0, Amount: 20},
		{Date: date(2021, 11, 28), T: 241.0 / 365.0, Amount: 20},
		{Date: date(2022, 5, 28), T: 422.0 / 365.0, Amount: 20},
		{Date: date(2022, 11, 28), T: 606.0 / 365.0, Amount: 20},
		{Date: date(2023, 5, 28), T: 787.0 / 365.0, Amount: 1030},
	}
	if len(cfs) != len(expected) {
		t.Fatalf("got %v, expected %v", cfs, expected)
	}
	for i := range cfs {
		if !cfs[i].Date.Equal(expected[i].Date) || math.Abs(cfs[i].T-expected[i].T) > 1e-15 || math.Abs(cfs[i].Amount-expected[i].Amount) > 1e-12 {
			t.Errorf("cash flow %d: got %+v, expected %+v", i, cfs[i], expected[i])
		}
	}
}

func TestStraight_Accrued(t *testing.T) {
	testData := []struct {
		Quote     time.Time
		Maturity  time.Time
		Frequency maturity.Frequency
		Expected  float64
	}{
		// 30E/360: 319 days since 2020-05-28 -> 1.25 * 319/360
		{date(2021, 4, 17), date(2026, 5, 28), maturity.Annual, 1.11},
		// 30E/360: 139 days since 2020-11-28 -> 1.25 * 139/360
		{date(2021, 4, 17), date(2026, 5, 28), maturity.SemiAnnual, 0.48},
	}

	for nr, test := range testData {
		b := mustStraight(t, maturity.Schedule{Maturity: test.Maturity, Frequency: test.Frequency}, 0.0125)
		accrued, err := b.Accrued(test.Quote)
		if err != nil {
			t.Fatal(err)
		}
		accrued = math.Round(accrued*100.0) / 100.0
		if math.Abs(accrued-test.Expected) > 0.001 {
			t.Errorf("test nr %d, got %f, expected %f", nr, accrued, test.Expected)
		}
	}
}

// TestStraight_AccruedEndOfMonth checks the accrued interest of a bond with
// end-of-month coupon dates.
// Previous coupon is 2024-08-31, next coupon 2025-02-28; 30E360 gives 15 days
// accrued, i.e. 4 * 15/360 = 0.1667.
func TestStraight_AccruedEndOfMonth(t *testing.T) {
	b := mustStraight(t, maturity.Schedule{Maturity: date(2026, 8, 31), Frequency: maturity.SemiAnnual, DayCount: maturity.Thirty360E}, 0.04)
	expected := 4.0 * 15.0 / 360.0
	if got, err := b.Accrued(date(2024, 9, 15)); err != nil || math.Abs(got-expected) > 1e-12 {
		t.Errorf("got %v, %v, expected %v", got, err, expected)
	}
}

// TestStraight_AccruedConvention checks that accrued interest for the 30/360
// family is coupon * days/360 and not days/periodDays/frequency (which is
// wrong for periods ending at the end of February)
func TestStraight_AccruedConvention(t *testing.T) {
	// coupon dates 31-Aug / 28-Feb; previous coupon 2006-08-31, settle
	// 2006-09-30: EUROBOND counts 30 days (the period 2006-08-31 to
	// 2007-02-28 has only 178 30/360 days)
	b := mustStraight(t, maturity.Schedule{Maturity: date(2009, 2, 28), Frequency: maturity.SemiAnnual, DayCount: maturity.Eurobond}, 0.06)
	expected := 100.0 * 0.06 * 30.0 / 360.0
	if got, err := b.Accrued(date(2006, 9, 30)); err != nil || math.Abs(got-expected) > 1e-12 {
		t.Errorf("EUROBOND: got %v, %v, expected %v", got, err, expected)
	}
	// ACT/360: 46 actual days since 2021-01-15
	b = mustStraight(t, maturity.Schedule{Maturity: date(2025, 7, 15), Frequency: maturity.SemiAnnual, DayCount: maturity.Act360}, 0.05)
	expected = 100.0 * 0.05 * 46.0 / 360.0
	if got, err := b.Accrued(date(2021, 3, 2)); err != nil || math.Abs(got-expected) > 1e-12 {
		t.Errorf("ACT360: got %v, %v, expected %v", got, err, expected)
	}
	// on a coupon date nothing is accrued and the coupon is not received
	b.DayCount = maturity.Thirty360E
	if got, err := b.Accrued(date(2021, 7, 15)); err != nil || got != 0 {
		t.Errorf("on coupon date: got %v, %v", got, err)
	}
	cfs, _ := b.CashFlows(date(2021, 7, 15))
	if !cfs[0].Date.Equal(date(2022, 1, 15)) {
		t.Errorf("coupon on settlement date must not be received: %v", cfs[0])
	}
}

func TestStraight_Stub(t *testing.T) {
	// short first period 2021-01-15 .. 2021-06-01 (30E/360: 136 days)
	b := mustStraight(t, maturity.Schedule{Issue: date(2021, 1, 15), Maturity: date(2023, 6, 1), Frequency: maturity.Annual}, 0.036)
	cfs, err := b.CashFlows(date(2021, 1, 15))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfs) != 3 || math.Abs(cfs[0].Amount-3.6*136.0/360.0) > 1e-12 || math.Abs(cfs[1].Amount-3.6) > 1e-12 || math.Abs(cfs[2].Amount-103.6) > 1e-12 {
		t.Errorf("wrong cash flows %+v", cfs)
	}
	// accrued from the issue date (30E/360: 46 days to 2021-03-01), 0 before issue
	if got, _ := b.Accrued(date(2021, 3, 1)); math.Abs(got-3.6*46.0/360.0) > 1e-12 {
		t.Errorf("accrued in stub: got %v, expected %v", got, 3.6*46.0/360.0)
	}
	if got, _ := b.Accrued(date(2020, 12, 1)); got != 0 {
		t.Errorf("accrued before issue: got %v", got)
	}
}

func TestStraight_DurationConvexity(t *testing.T) {
	// zero bond with 10 years (3652 days) to maturity
	b := mustStraight(t, maturity.Schedule{Maturity: date(2031, 1, 1), Frequency: maturity.Annual}, 0)
	cfs, err := b.CashFlows(date(2021, 1, 1))
	if err != nil {
		t.Fatal(err)
	}
	T := 3652.0 / 365.0
	if d := cashflow.Duration(cfs, snb20210331); math.Abs(d-T) > 1e-12 {
		t.Errorf("duration: got %f, expected %f", d, T)
	}
	if c := cashflow.Convexity(cfs, snb20210331); math.Abs(c-T*T) > 1e-10 {
		t.Errorf("convexity: got %f, expected %f", c, T*T)
	}
}

func TestStraight_Invalid(t *testing.T) {
	invalid := []bond.Straight{
		{Schedule: maturity.Schedule{Maturity: date(2031, 1, 1)}, Face: 100},                                   // no frequency
		{Schedule: maturity.Schedule{Maturity: date(2031, 1, 1), Frequency: 1}, Coupon: math.NaN(), Face: 100}, // NaN coupon
		{Schedule: maturity.Schedule{Maturity: date(2031, 1, 1), Frequency: 1}, Face: -100},                    // negative face
		{Schedule: maturity.Schedule{Maturity: date(2031, 1, 1), Frequency: 1}},                                // zero face (struct literal)
		{Schedule: maturity.Schedule{Maturity: date(2031, 1, 1), Frequency: 1}, Face: 100, Redemption: -1},     // negative redemption
		{Schedule: maturity.Schedule{Maturity: date(2031, 1, 1), Frequency: 1, DayCount: 42}, Face: 100},       // unknown basis
	}
	for i, b := range invalid {
		if _, err := b.CashFlows(date(2021, 1, 1)); err == nil {
			t.Errorf("bond %d: expected error from CashFlows", i)
		}
		if _, err := b.Accrued(date(2021, 1, 1)); err == nil {
			t.Errorf("bond %d: expected error from Accrued", i)
		}
		if _, err := b.DirtyPrice(date(2021, 1, 1), term.Flat{}); err == nil {
			t.Errorf("bond %d: expected error from DirtyPrice", i)
		}
	}
	// invalid settlement dates
	b := mustStraight(t, maturity.Schedule{Maturity: date(2031, 1, 1), Frequency: 1}, 0.01)
	for _, settle := range []time.Time{{}, date(1800, 1, 1)} {
		if _, err := b.CashFlows(settle); !errors.Is(err, maturity.ErrSettlement) {
			t.Errorf("settle %v: expected ErrSettlement from CashFlows, got %v", settle, err)
		}
		if _, err := b.Accrued(settle); !errors.Is(err, maturity.ErrSettlement) {
			t.Errorf("settle %v: expected ErrSettlement from Accrued, got %v", settle, err)
		}
	}
	if _, err := bond.NewStraight(maturity.Schedule{Maturity: date(2031, 1, 1), Frequency: 1}, 0.01, bond.WithFixing(0.01)); err == nil {
		t.Errorf("expected error for WithFixing on a straight bond")
	}
}

// TestStraight_Redemption checks explicit face and redemption values,
// including a zero redemption (coupon stream only)
func TestStraight_Redemption(t *testing.T) {
	s := maturity.Schedule{Maturity: date(2024, 1, 1), Frequency: maturity.Annual}
	b := mustStraight(t, s, 0.05, bond.WithRedemption(0))
	cfs, err := b.CashFlows(date(2021, 1, 1))
	if err != nil {
		t.Fatal(err)
	}
	for _, cf := range cfs {
		if math.Abs(cf.Amount-5) > 1e-12 {
			t.Errorf("zero redemption: unexpected cash flow %+v", cf)
		}
	}
	d := mustStraight(t, s, 0.05)
	if d.Face != 100 || d.Redemption != 100 {
		t.Errorf("defaults: face %v, redemption %v", d.Face, d.Redemption)
	}
	e := mustStraight(t, s, 0.05, bond.WithFace(1000))
	if e.Redemption != 1000 {
		t.Errorf("redemption must default to the face value: %v", e.Redemption)
	}
}

// TestStraight_Act360Smooth checks that the clean price of an ACT/360 bond
// does not jump across a coupon date (coupon = accrued over the period)
func TestStraight_Act360Smooth(t *testing.T) {
	c := term.Flat{Rate: 0.03}
	for _, dc := range []maturity.DayCount{maturity.Act360, maturity.Thirty360E, maturity.ActAct} {
		b := mustStraight(t, maturity.Schedule{Maturity: date(2027, 7, 15), Frequency: maturity.SemiAnnual, DayCount: dc}, 0.05)
		// coupon date 2022-01-15 (period of 184 days)
		var prices []float64
		for settle := date(2022, 1, 12); !settle.After(date(2022, 1, 18)); settle = settle.AddDate(0, 0, 1) {
			p, err := b.CleanPrice(settle, c)
			if err != nil {
				t.Fatal(err)
			}
			prices = append(prices, p)
		}
		// the daily changes are smooth: the change over the coupon date
		// differs from the neighbouring changes by much less than a day of
		// accrual (100*0.05/360 = 0.0139)
		maxJump := 0.0
		for i := 2; i < len(prices); i++ {
			jump := math.Abs((prices[i] - prices[i-1]) - (prices[i-1] - prices[i-2]))
			maxJump = math.Max(maxJump, jump)
		}
		if maxJump > 1e-3 {
			t.Errorf("%v: clean price not smooth across coupon date: %v (max second difference %v)", dc, prices, maxJump)
		}
	}
	// ACT/360 coupon equals the accrual over the period
	b := mustStraight(t, maturity.Schedule{Maturity: date(2027, 7, 15), Frequency: maturity.SemiAnnual, DayCount: maturity.Act360}, 0.05)
	cfs, _ := b.CashFlows(date(2021, 12, 1))
	if math.Abs(cfs[0].Amount-100*0.05*184.0/360.0) > 1e-12 {
		t.Errorf("ACT/360 coupon %v, expected %v", cfs[0].Amount, 100*0.05*184.0/360.0)
	}
}

// TestStraight_LongFirstCoupon checks the long first coupon of a 30E/360
// bond and its ACT/ACT ICMA counterpart
func TestStraight_LongFirstCoupon(t *testing.T) {
	s := maturity.Schedule{Issue: date(2020, 1, 15), FirstCoupon: date(2021, 6, 1), Maturity: date(2023, 6, 1), Frequency: maturity.Annual}
	b := mustStraight(t, s, 0.04)
	cfs, err := b.CashFlows(date(2020, 1, 15))
	if err != nil {
		t.Fatal(err)
	}
	// 30E/360: 2020-01-15 .. 2021-06-01 = 1*360 + 5*30 - 14 = 496 days
	if len(cfs) != 3 || math.Abs(cfs[0].Amount-4*496.0/360.0) > 1e-12 {
		t.Errorf("long first coupon: %+v, expected %v", cfs, 4*496.0/360.0)
	}
	s.DayCount = maturity.ActAct
	b = mustStraight(t, s, 0.04)
	// accrued at 2020-09-01: 138/366 + 92/365 years
	acc, err := b.Accrued(date(2020, 9, 1))
	if err != nil || math.Abs(acc-4*(138.0/366.0+92.0/365.0)) > 1e-12 {
		t.Errorf("ICMA long stub accrued %v, %v; expected %v", acc, err, 4*(138.0/366.0+92.0/365.0))
	}
}

// BenchmarkBondPVOnly times cashflow.PV only (cash flows built once)
func BenchmarkBondPVOnly(b *testing.B) {
	bnd := mustStraight(b, maturity.Schedule{Maturity: date(2051, 5, 28), Frequency: maturity.SemiAnnual}, 0.0125)
	cfs, err := bnd.CashFlows(date(2021, 4, 1))
	if err != nil {
		b.Fatal(err)
	}
	var c term.Curve = snb20210331
	sum := 0.0
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sum += cashflow.PV(cfs, c)
	}
	_ = sum
}

// 30y semiannual bond for the end-to-end benchmarks
func benchBond(b *testing.B) bond.Straight {
	return mustStraight(b, maturity.Schedule{Maturity: date(2051, 5, 28), Frequency: maturity.SemiAnnual}, 0.0125)
}

func BenchmarkBondCashFlows(b *testing.B) {
	bnd := benchBond(b)
	settle := date(2021, 4, 17)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := bnd.CashFlows(settle); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBondDirtyPrice(b *testing.B) {
	bnd := benchBond(b)
	settle := date(2021, 4, 17)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := bnd.DirtyPrice(settle, snb20210331); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBondCleanPrice(b *testing.B) {
	bnd := benchBond(b)
	settle := date(2021, 4, 17)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := bnd.CleanPrice(settle, snb20210331); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBondAccrued(b *testing.B) {
	bnd := benchBond(b)
	settle := date(2021, 4, 17)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := bnd.Accrued(settle); err != nil {
			b.Fatal(err)
		}
	}
}
