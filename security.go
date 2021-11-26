package bonds

import "github.com/konimarti/bonds/pkg/term"

type Security interface {
	PresentValue(ts term.Structure) float64
}
