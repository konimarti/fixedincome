package bonds

import "github.com/konimarti/bonds/pkg/term"

type Security interface {
	Pricing(spread float64, ts term.Structure) (dirty float64, clean float64)
}
