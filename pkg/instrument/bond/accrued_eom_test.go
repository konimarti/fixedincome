package bond_test

import (
	"math"
	"testing"
	"time"

	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/maturity"
)

// TestStraight_AccruedEndOfMonth checks the accrued interest of a bond with
// end-of-month coupon dates.
// Previous coupon is 2024-08-31, next coupon 2025-02-28; 30E360 gives 15 days
// accrued, i.e. 4 * 15/360 = 0.1667.
func TestStraight_AccruedEndOfMonth(t *testing.T) {
	b := bond.Straight{
		Schedule: maturity.Schedule{
			Settlement: time.Date(2024, 9, 15, 0, 0, 0, 0, time.UTC),
			Maturity:   time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
			Frequency:  2,
			Basis:      "30E360",
		},
		Coupon:     4.0,
		Redemption: 100.0,
	}
	expected := 4.0 * 15.0 / 360.0
	if got := b.Accrued(); math.Abs(got-expected) > 1e-9 {
		t.Errorf("got %v, expected %v", got, expected)
	}
}
