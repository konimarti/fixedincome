package bonds

import "github.com/konimarti/bonds/pkg/term"

type Security interface {
	// FIXME: simplify function to cover more securities
	Pricing(spread float64, ts term.Structure) (dirty float64, clean float64)
}
