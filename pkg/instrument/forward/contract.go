// Package forward implements forward contracts and forward rate agreements.
//
// Conventions: rates are decimals; times are in years (ACT/365 Fixed from
// the valuation date) and discounting uses term.Curve.
package forward

import (
	"math"

	"github.com/konimarti/fixedincome/pkg/term"
)

// Contract (forward contract) is a contract between two counterparties in
// which one counterparty agrees to purchase and the other agrees to sell a
// given asset at a given future time, and at a given price, called the
// forward price. Its value for the buyer (long position) is (F - K) * Z(T).
// It is specified by the remaining maturity T (years) instead of dates, so
// PresentValue has no settlement date argument.
type Contract struct {
	// K is the delivery price agreed upon at initiation
	K float64
	// F is the current forward price F(t,T) for delivery at T
	F float64
	// T is the remaining maturity in years
	T float64
}

// PresentValue returns the value of the long forward position (F-K)*Z(T).
func (f Contract) PresentValue(c term.Curve) float64 {
	return (f.F - f.K) * c.Discount(f.T)
}

// ZeroBondPrice returns the forward price at time t of a zero-coupon bond
// with unit face value maturing at m: Z(m)/Z(t).
func ZeroBondPrice(t, m float64, c term.Curve) float64 {
	return c.Discount(m) / c.Discount(t)
}

// FxPrice returns the forward exchange rate for delivery in t years, given
// the spot exchange rate (units of domestic currency per unit of foreign
// currency) and the domestic and foreign curves: spot * Zf(t) / Zd(t).
func FxPrice(spot, t float64, domestic, foreign term.Curve) float64 {
	return spot * foreign.Discount(t) / domestic.Discount(t)
}

// AssetPrice returns the forward price for delivery in t years of an asset
// (stock, commodity) with spot price spot and continuous yield q (dividend
// yield or convenience yield minus storage costs, decimal):
// spot * exp(-q*t) / Z(t).
func AssetPrice(spot, q, t float64, c term.Curve) float64 {
	return spot * math.Exp(-q*t) / c.Discount(t)
}
