package fixedincome

import "github.com/konimarti/fixedincome/pkg/term"

type Security interface {
	PresentValue(ts term.Structure) float64
}

type TermSecurity interface {
	Security
	Duration(ts term.Structure) float64
	Convexity(ts term.Structure) float64
}

type Option interface {
	Security
	SetVola(float64)
}
