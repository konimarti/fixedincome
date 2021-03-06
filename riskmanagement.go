package fixedincome

import "github.com/konimarti/fixedincome/pkg/term"

// PVBP calculates the price value of a base point (bps)
// dp = - p * D * dr + 0.5 * p * convex * dr^2
func PVBP(s TermSecurity, ts term.Structure) float64 {
	return InterestSensitivity(0.0001, s, ts) * s.PresentValue(ts)
}

// InterestSensitivity calculated the percent change in value for a parallel increase of the yield curve
// dp/p = -D * dr + 0.5 * Convex * dr^2
func InterestSensitivity(dr float64, s TermSecurity, ts term.Structure) float64 {
	return s.Duration(ts)*dr + 0.5*s.Convexity(ts)*dr*dr
}
