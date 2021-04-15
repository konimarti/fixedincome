package bonds

import "math"

type ConstantRate struct {
	R float64
}

func (c *ConstantRate) Forward(m, t float64) float64 {
	return c.R
}

func (c *ConstantRate) Rate(m float64) float64 {
	return c.R
}

func (c *ConstantRate) D(m float64, zspread float64, freq int) float64 {
	return math.Pow((1.0 + (c.R/100.0+zspread/1e4)/float64(freq)), -m*float64(freq))
}
