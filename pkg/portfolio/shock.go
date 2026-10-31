package portfolio

import (
	"math"

	"github.com/konimarti/fixedincome/pkg/term"
)

// shifted is a base curve with the continuously compounded zero rate
// shifted by f(t).
type shifted struct {
	base term.Curve
	f    func(t float64) float64
}

// Shift returns the curve c with the zero rate r(t) replaced by r(t)+f(t)
// (f in decimals, t in years): Discount(t) = c.Discount(t)*exp(-f(t)*t). The
// curve c is not modified; f must be a pure function. Shift panics if c is
// nil (programmer error).
func Shift(c term.Curve, f func(t float64) float64) term.Curve {
	if c == nil {
		panic("portfolio: Shift of a nil curve")
	}
	return shifted{base: c, f: f}
}

// Discount returns the shifted discount factor.
func (s shifted) Discount(t float64) float64 {
	return s.base.Discount(t) * math.Exp(-s.f(t)*t)
}

// ZeroRate returns the shifted zero rate.
func (s shifted) ZeroRate(t float64) float64 {
	return s.base.ZeroRate(t) + s.f(t)
}

// Scenario is a deterministic shift of the zero curve. The spreads of the
// positions are not shocked.
type Scenario struct {
	Name string
	// Shift is the shift of the continuously compounded zero rate (decimal)
	// at maturity t (years)
	Shift func(t float64) float64
	// Parallel marks a parallel shift by Size (decimal): Shift(t) = Size
	Parallel bool
	Size     float64
}

// ParallelScenario returns the parallel shift of the zero curve by dr
// (decimal).
func ParallelScenario(name string, dr float64) Scenario {
	return Scenario{Name: name, Shift: func(float64) float64 { return dr }, Parallel: true, Size: dr}
}

// IRRBBDecay is the decay constant (years) of the short and long rate shock
// functions of the standardised interest rate shock scenarios of the Basel
// Committee (BCBS d368, April 2016).
const IRRBBDecay = 4.0

// IRRBBParams are the currency specific magnitudes (decimals) of the
// standardised interest rate shock scenarios.
type IRRBBParams struct {
	// Parallel, Short and Long are R_parallel, R_short and R_long
	Parallel, Short, Long float64
}

// IRRBB returns the six standardised interest rate shock scenarios of the
// Basel Committee (BCBS d368, "Interest rate risk in the banking book",
// 2016), without the floor on the shocked rates:
//
//	short(t) = R_short * exp(-t/4)
//	long(t)  = R_long * (1 - exp(-t/4))
//	parallel up/down: +-R_parallel
//	steepener:  -0.65*short(t) + 0.9*long(t)   (short rates down, long up)
//	flattener:  +0.8*short(t) - 0.6*long(t)    (short rates up, long down)
//	short up/down: +-short(t)
//
// The magnitudes are currency specific and are passed in p; the coefficients
// are those of the standard text and should be checked against the current
// version of the standard or of the supervisor's circular (e.g. FINMA).
func IRRBB(p IRRBBParams) []Scenario {
	short := func(t float64) float64 { return p.Short * math.Exp(-t/IRRBBDecay) }
	long := func(t float64) float64 { return p.Long * (1 - math.Exp(-t/IRRBBDecay)) }
	return []Scenario{
		ParallelScenario("parallel_up", p.Parallel),
		ParallelScenario("parallel_down", -p.Parallel),
		{Name: "steepener", Shift: func(t float64) float64 { return -0.65*short(t) + 0.9*long(t) }},
		{Name: "flattener", Shift: func(t float64) float64 { return 0.8*short(t) - 0.6*long(t) }},
		{Name: "short_up", Shift: short},
		{Name: "short_down", Shift: func(t float64) float64 { return -short(t) }},
	}
}
