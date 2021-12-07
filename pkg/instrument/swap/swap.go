package swap

import (
	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/term"
)

// InterestRateSwap implements a plain vanilla fixed-for-floating interest
// rate swap contract. The interest rate swap is an agreement between
// two counterpaties in which one counterparty agrees to make n fixed payments
// per year at an (annualized) fixed rate c up to a maturity date T,
// while at the same time the other counterparty commits to make payments
// linked to a floating rate index.
type InterestRateSwap struct {
	// Floating rate bond (long position)
	Floating bond.Floating
	// Fixed rate bond (short position) with swap rate as coupon
	Fixed bond.Straight
}

// PresentValue returns the value of the forward contract
func (s *InterestRateSwap) PresentValue(ts term.Structure) float64 {
	return s.Floating.PresentValue(ts) - s.Fixed.PresentValue(ts)
}
