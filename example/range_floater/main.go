package main

import (
	"flag"
	"fmt"
	"log"
	"math"

	"github.com/konimarti/fixedincome/pkg/mc"
	"github.com/konimarti/fixedincome/pkg/mc/model/vasicek"
)

// Valuation of a range floater
//
// Term sheet of the range floater:
//
// Today:                1.11.2004
// Maturity:             1.11.2007
// Frequency:            Quarterly
// Notional:             100.0
// Coupon:               CF(t) = Notional * (Libor_(t-1) + Spread)
//                               * Accrual factor / number of days
// Spread over floating: 1%
// Number of days:       360
// Accrual factor:       Number of days during the relevant interest rate
//                       period that the 3-month LIBOR is within the range
//                       [1.18%,4.18%]
// Current 3-month LIBOR: 2.18%
//
// Source: P. Veronesi, Fixed Income Securities, page 604; Vasicek model
// with r0 = 2.17%, r̄ = 6.41%, γ = 0.21, σ = 0.82%.
//
// Four variants are computed on the same paths (daily grid, 360 days per
// year, exact Vasicek transition), combining two LIBOR mappings with two
// discounting methods:
//
//   - LIBOR "flat": the short rate converted as if it were flat for three
//     months, L = 4 (e^{r/4} - 1) (simplification of the book); LIBOR
//     "model": implied by the Vasicek bond price, L = (1/Z(r;t,t+δ) - 1)/δ
//     with δ = 90/360.
//   - discounting "roll": rolling over the LIBOR, ∏ (1 + L_q/4) (as in the
//     book); "Df": the pathwise discount factor exp(-∫r), the risk-neutral
//     valuation consistent with the model.
//
// "flat + roll" is the Veronesi variant (book: 99.8385, standard error
// 0.0505) and "model + Df" the model-consistent valuation. With 200000
// paths (go run . -paths 200000 -seed 1):
//
//	flat L,  roll   99.8116 ± 0.0080
//	flat L,  Df     99.5759 ± 0.0082
//	model L, roll   99.6022 ± 0.0082
//	model L, Df     99.6070 ± 0.0085
//
// Decomposition of the difference of -0.205 between the model-consistent
// and the Veronesi variant:
//
//   - With the model LIBOR, rolling over the LIBOR and discounting with
//     exp(-∫r) agree (+0.005): the model LIBOR is the model's own 3-month
//     rate.
//   - With consistent discounting (Df) the LIBOR mapping changes the value
//     by only +0.03: the model LIBOR is higher (2.29% instead of 2.18% at
//     r = 2.17%, because r0 is far below r̄), which raises the coupon rate
//     but lets the LIBOR leave the range [1.18%, 4.18%] earlier at the
//     top; the two effects almost cancel.
//   - With the flat LIBOR, rolling over the LIBOR discounts at a rate that
//     is about 11bp too low, which overstates the value by 0.236 compared
//     with Df discounting.
//
// The Veronesi value is therefore high mainly because the flat-converted
// LIBOR is used for discounting (an interaction of both simplifications),
// not because of the accrual counting.

const (
	years    = 3
	days     = 360 // grid steps per year
	quarter  = 90  // days per quarter
	delta    = 0.25
	notional = 100.0
	spread   = 0.01
	lower    = 0.0118
	upper    = 0.0418
)

// rangeFloater values the range floater; libor maps the short rate to the
// 3-month LIBOR and modelDiscount selects the discounting.
type rangeFloater struct {
	libor         func(r float64) float64
	modelDiscount bool
}

func (f rangeFloater) Value(p *mc.Path) float64 {
	r := p.State[0]
	nq := years * days / quarter
	pv := 0.0
	roll := 1.0
	for q := 0; q < nq; q++ {
		start := q * quarter
		in := 0
		for d := start; d < start+quarter; d++ {
			if l := f.libor(r[d]); l > lower && l < upper {
				in++
			}
		}
		reset := f.libor(r[start])
		cf := notional * (reset + spread) * float64(in) / days
		end := start + quarter
		if f.modelDiscount {
			pv += p.Df[end] * cf
			if q == nq-1 {
				pv += p.Df[end] * notional
			}
		} else {
			roll *= 1 + reset*delta
			pv += cf / roll
			if q == nq-1 {
				pv += notional / roll
			}
		}
	}
	return pv
}

// variants values the four variants on the same paths.
type variants [4]rangeFloater

func (v variants) Len() int { return len(v) }
func (v variants) Values(p *mc.Path, out []float64) {
	for i := range v {
		out[i] = v[i].Value(p)
	}
}

// names of the four variants of price.
var names = [4]string{"flat L,  roll", "flat L,  Df", "model L, roll", "model L, Df"}

// price returns the four variants: flat/model LIBOR × roll/Df discounting
// (index 0 is the Veronesi variant, index 3 the model-consistent one).
func price(o mc.Options) ([]mc.Result, error) {
	model, err := vasicek.New(0.0217, 0.0641, 0.21, 0.0082)
	if err != nil {
		return nil, err
	}
	g, err := mc.NewUniformGrid(years, years*days)
	if err != nil {
		return nil, err
	}
	// Z(r;t,t+δ) = exp(c - b r)
	c := math.Log(model.Z(0, 0, delta))
	b := c - math.Log(model.Z(1, 0, delta))
	flat := func(r float64) float64 { return (math.Exp(r*delta) - 1) / delta }
	implied := func(r float64) float64 { return (math.Exp(b*r-c) - 1) / delta }
	return mc.PriceMulti(model, variants{{flat, false}, {flat, true}, {implied, false}, {implied, true}}, g, o)
}

func main() {
	paths := flag.Int("paths", 20000, "number of Monte Carlo paths")
	seed := flag.Uint64("seed", 2004, "seed")
	flag.Parse()
	res, err := price(mc.Options{Paths: *paths, Seed: *seed})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Range floater (t=0), %d paths, seed %d\n", res[0].Paths, *seed)
	for i, r := range res {
		fmt.Printf("%-14s %10.4f ± %.4f\n", names[i], r.Mean, r.StdErr)
	}
	fmt.Printf("Veronesi (book: flat L, roll) %10.4f ± %.4f\n", 99.8385, 0.0505)
	fmt.Printf("Difference model-consistent - Veronesi variant: %.4f\n", res[3].Mean-res[0].Mean)
}
