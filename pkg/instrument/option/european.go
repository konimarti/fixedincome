// Package option implements the Black-Scholes valuation of European options.
//
// Conventions: the dividend yield Q, the volatility Vol and the interest
// rate r are decimals (0.02 = 2%) and continuously compounded; the
// maturity T is in years. The interest rate is the zero rate of the curve
// at the option's maturity, r = c.ZeroRate(T).
package option

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/konimarti/fixedincome/internal/solve"
	"github.com/konimarti/fixedincome/pkg/term"
)

// Type is the type of an option.
type Type int

// Option types. The zero value is invalid.
const (
	Call Type = iota + 1
	Put
)

// String returns "call" or "put".
func (t Type) String() string {
	switch t {
	case Call:
		return "call"
	case Put:
		return "put"
	}
	return fmt.Sprintf("Type(%d)", int(t))
}

// ParseType parses "call" or "put" (case-insensitive; also "c" and "p").
func ParseType(s string) (Type, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "call", "c":
		return Call, nil
	case "put", "p":
		return Put, nil
	}
	return 0, fmt.Errorf("option: invalid option type %q: must be 'call' or 'put'", s)
}

var (
	// ErrNoImpliedVol is returned if no volatility in [1e-6, 5] reprices
	// the option.
	ErrNoImpliedVol = errors.New("option: no implied volatility found")
	// ErrNoVega is returned if the implied volatility is at the lower
	// bound 1e-6 of the search interval, where the price does not depend
	// on the volatility (e.g. a deep in-the-money option priced at its
	// discounted intrinsic value).
	ErrNoVega = errors.New("option: implied volatility at lower bound (no vega)")
)

// Validate returns an error unless t is Call or Put.
func (t Type) Validate() error {
	if t != Call && t != Put {
		return fmt.Errorf("option: invalid option type %d: must be option.Call or option.Put", int(t))
	}
	return nil
}

// sign returns +1 for a call and -1 for a put.
func (t Type) sign() float64 {
	if t == Put {
		return -1
	}
	return 1
}

// European is a plain vanilla European option.
type European struct {
	// Type is Call or Put
	Type Type
	// S is the price of the underlying asset (> 0)
	S float64
	// K is the strike price (> 0)
	K float64
	// T is the remaining maturity in years (>= 0)
	T float64
	// Vol is the volatility of the underlying (decimal, > 0)
	Vol float64
	// Q is the continuous dividend yield (decimal)
	Q float64
}

// Validate returns an error for an invalid option type, non-positive S, K or
// Vol, negative T or non-finite parameters.
func (e European) Validate() error {
	if err := e.Type.Validate(); err != nil {
		return err
	}
	for _, p := range []struct {
		name  string
		value float64
	}{{"S", e.S}, {"K", e.K}, {"T", e.T}, {"Vol", e.Vol}, {"Q", e.Q}} {
		if math.IsNaN(p.value) || math.IsInf(p.value, 0) {
			return fmt.Errorf("option: %s is not finite: %v", p.name, p.value)
		}
	}
	if !(e.S > 0) || !(e.K > 0) {
		return fmt.Errorf("option: underlying price %v and strike %v must be positive", e.S, e.K)
	}
	if e.T < 0 {
		return fmt.Errorf("option: negative maturity %v", e.T)
	}
	if !(e.Vol > 0) {
		return fmt.Errorf("option: volatility %v must be positive", e.Vol)
	}
	return nil
}

// price returns the Black-Scholes price without validation.
func (e European) price(c term.Curve) float64 {
	sign := e.Type.sign()
	if e.T <= 0 {
		return math.Max(sign*(e.S-e.K), 0)
	}
	r := c.ZeroRate(e.T)
	d1 := D1(e.S, e.K, e.T, r, e.Q, e.Vol)
	d2 := D2(d1, e.T, e.Vol)
	return sign * (e.S*math.Exp(-e.Q*e.T)*N(sign*d1) - e.K*math.Exp(-r*e.T)*N(sign*d2))
}

// Price returns the Black-Scholes price with the interest rate
// r = c.ZeroRate(T). For T = 0 the intrinsic value is returned. The option
// is specified by its remaining maturity T, so Price has no settlement date
// argument (unlike the PresentValue methods of the other instruments).
func (e European) Price(c term.Curve) (float64, error) {
	if err := e.Validate(); err != nil {
		return math.NaN(), err
	}
	return e.price(c), nil
}

// Greeks are the sensitivities of the option price.
type Greeks struct {
	// Delta is dV/dS
	Delta float64
	// Gamma is d²V/dS²
	Gamma float64
	// Vega is dV/dVol per 1.00 (100%) change of the volatility
	Vega float64
	// Theta is dV/dt per year of calendar time (= -dV/dT) with the rate
	// r = r(T) and the dividend yield held fixed. The roll-down of a
	// non-flat curve (r changes to r(T-dt)) adds Rho * dr(T)/dT =
	// Rho * (f(T) - r(T)) / T, with f the instantaneous forward; e.g. for
	// an upward slope of 1% over 2 years about Rho * 0.005 per year,
	// small compared to Theta for typical options.
	Theta float64
	// Rho is dV/dr per 1.00 (100%) parallel change of the interest rate
	Rho float64
	// Phi is dV/dQ per 1.00 (100%) change of the dividend yield
	Phi float64
}

// Greeks returns the Black-Scholes Greeks with the interest rate
// r = c.ZeroRate(T). For T = 0 the limits at expiry are returned: Delta is
// +1/-1 in the money (1/2 at the money), all other Greeks are 0.
func (e European) Greeks(c term.Curve) (Greeks, error) {
	if err := e.Validate(); err != nil {
		return Greeks{}, err
	}
	sign := e.Type.sign()
	if e.T <= 0 {
		delta := 0.0
		switch {
		case sign*(e.S-e.K) > 0:
			delta = sign
		case e.S == e.K:
			delta = sign * 0.5
		}
		return Greeks{Delta: delta}, nil
	}
	r := c.ZeroRate(e.T)
	sqrtT := math.Sqrt(e.T)
	d1 := D1(e.S, e.K, e.T, r, e.Q, e.Vol)
	d2 := D2(d1, e.T, e.Vol)
	dq := math.Exp(-e.Q * e.T)
	z := math.Exp(-r * e.T)
	pdf := Napostroph(d1)
	return Greeks{
		Delta: sign * dq * N(sign*d1),
		Gamma: dq * pdf / (e.S * e.Vol * sqrtT),
		Vega:  e.S * dq * pdf * sqrtT,
		Theta: -e.S*dq*pdf*e.Vol/(2*sqrtT) - sign*r*e.K*z*N(sign*d2) + sign*e.Q*e.S*dq*N(sign*d1),
		Rho:   sign * e.K * e.T * z * N(sign*d2),
		Phi:   -sign * e.S * e.T * dq * N(sign*d1),
	}, nil
}

// ImpliedVol returns the volatility (decimal) at which the Black-Scholes price
// of opt with the curve c equals price; opt.Vol is ignored and opt is not
// modified. The volatility is searched in [1e-6, 5] (Brent's method). An
// error is returned if the option is invalid or T = 0; an error wrapping
// ErrNoImpliedVol if the price is outside the range of prices attainable in
// that interval (e.g. below the arbitrage bound), and ErrNoVega if the root
// is at the lower bound 1e-6 (the price does not determine the
// volatility).
func ImpliedVol(price float64, opt European, c term.Curve) (float64, error) {
	opt.Vol = 1.0
	if err := opt.Validate(); err != nil {
		return math.NaN(), err
	}
	if opt.T <= 0 {
		return math.NaN(), fmt.Errorf("option: implied volatility undefined at expiry")
	}
	if math.IsNaN(price) || math.IsInf(price, 0) {
		return math.NaN(), fmt.Errorf("option: invalid price %v", price)
	}
	f := func(vol float64) float64 {
		o := opt
		o.Vol = vol
		return o.price(c) - price
	}
	const lower, upper = 1e-6, 5.0
	vol, err := solve.Brent(f, lower, upper, 1e-12, 0)
	if err != nil {
		return math.NaN(), fmt.Errorf("%w for price %v in [1e-6, 5]: %v", ErrNoImpliedVol, price, err)
	}
	if vol <= lower*(1+1e-9) {
		return math.NaN(), ErrNoVega
	}
	return vol, nil
}

// D1 returns d1 = (ln(S/K) + (r - q + vol²/2) T) / (vol sqrt(T)) of the
// Black-Scholes formula; r, q and vol are decimals.
func D1(S, K, T, r, q, vol float64) float64 {
	return (math.Log(S/K) + (r-q+vol*vol/2.0)*T) / (vol * math.Sqrt(T))
}

// D2 returns d2 = d1 - vol sqrt(T).
func D2(d1, T, vol float64) float64 {
	return d1 - vol*math.Sqrt(T)
}

// N is the cumulative distribution function of the standard normal
// distribution.
func N(x float64) float64 {
	return 0.5 * math.Erfc(-x/math.Sqrt2)
}

// Napostroph is the probability density function of the standard normal
// distribution.
func Napostroph(x float64) float64 {
	return math.Exp(-(x*x)/2.0) / (math.SqrtPi * math.Sqrt2)
}
