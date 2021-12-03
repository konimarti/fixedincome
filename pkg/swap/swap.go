package swap

import (
	"github.com/konimarti/bonds/pkg/bond"
	"github.com/konimarti/bonds/pkg/term"
)

type Swap struct {
	// Floating rate bond (long position)
	Floating bond.Floating
	// Fixed rate bond (short position) with swap rate as coupon
	Fixed bond.Straight
}

// PresentValue returns the value of the forward contract
func (s *Swap) PresentValue(ts term.Structure) float64 {
	return s.Floating.PresentValue(ts) - s.Fixed.PresentValue(ts)
}
