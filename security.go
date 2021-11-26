package bonds

import "github.com/konimarti/bonds/pkg/term"

type Security interface {
	PresentValue(spread float64, ts term.Structure) float64
}
