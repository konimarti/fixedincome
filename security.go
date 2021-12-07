package bonds

import "github.com/konimarti/fixedincome/pkg/term"

type Security interface {
	PresentValue(ts term.Structure) float64
}

type Option interface {
	Security
	SetVola(float64)
}
