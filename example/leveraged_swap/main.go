package main

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/khezen/rootfinding"
	"github.com/konimarti/fixedincome/pkg/mc"
	"github.com/konimarti/fixedincome/pkg/mc/model/vasicek"
)

// Valuation of a levered swap: Case Study Procter & Gamble / Bankers Trust

// Term sheet of the range floater:
//
// Contract date: 	4.11.1993
// Maturity: 		4.11.1998
// Notional: 		200.0
// Bankers Trust pays:  5.3%
// P&G pays:            Commercial paper rate - 0.75% + "spread"
// 			spread = 0 until 4.11.1994
//                      spread = (5yr T-note yield * 98.5 / 5.78) - Price of 6.25% 30yr T-Bond
// 			spread cannot be negative
//
// To avoid using a multi-factor model, we simplify with the assumption
// that the spread between the commercial paper rate and treasurey rate is constant:
//  			spread_cp = 0.2488 bp
//
// Also, we ignore default risks.
//
// Source: Book by P. Veronesi, Fixed Income Securities, page 619

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func main() {

	T := 30.5
	N := int(52 * T)
	dt := T / float64(N)
	spreadCb := 0.2488 * 0.0001
	notional := 200.0

	vasi := vasicek.Vasicek{
		Rbar:  7.32 / 100.0,
		Gamma: 0.2904,
		Sigma: 0.006352, // long-term: 0.010776 short-term: 0.0063
	}

	model := vasicek.Vasicek{
		R0:    3.0 / 100.0,
		Rbar:  vasi.Rbar,
		Gamma: vasi.Gamma,
		Sigma: vasi.Sigma,
		T:     T,
		N:     N,
		Rng:   rand.New(rand.NewSource(time.Now().UnixNano())),
		// Rng:   rand.New(rand.NewSource(99)),
		Payoff: func(rates []float64) float64 {

			Z := func(rates []float64, t, T float64) float64 {
				i := int(t / dt)
				I := int(T / dt)
				r := 0.0
				for _, rate := range rates[0:I] {
					r += rate * dt
				}
				r0 := 0.0
				for _, rate := range rates[0:i] {
					r0 += rate * dt
				}
				return math.Exp(-r*T) / math.Exp(-r0*t)
			}

			// make 6-month time steps until maturity
			tstar := 0.5
			// i := int(tstar / dt)

			// value of 30yr 6.25% T-bond
			p30 := Z(rates, tstar, tstar+30.0)
			//p30 := vasi.Z(rates[i], tstar, tstar+30.0)
			for k := 1; k <= 60; k += 1 {
				p30 += 6.25 / 100.0 / 2.0 * Z(rates, tstar, tstar+float64(k)*0.5)
				//p30 += 6.25 / 100.0 / 2.0 * vasi.Z(rates[i], tstar, tstar+float64(k)*0.5)
			}
			// fmt.Println("p30     :", p30)

			// value of 5yr 9.10% T-bond
			p5 := Z(rates, tstar, tstar+5.0)
			//p5 := vasi.Z(rates[i], tstar, tstar+5.0)
			for k := 1; k <= 10; k += 1 {
				p5 += 9.10 / 100.0 / 2.0 * Z(rates, tstar, tstar+float64(k)*0.5)
				//p5 += 9.10 / 100.0 / 2.0 * vasi.Z(rates[i], tstar, tstar+float64(k)*0.5)
			}

			// calculate yield-to-maturity of 5yr T-bond
			fun := func(y0 float64) float64 {
				y := y0 / 100.0
				p5new := math.Pow(1.0+y/2.0, -10.0)
				for k := 1; k <= 10; k += 1 {
					p5new += 9.10 / 100.0 / 2.0 * math.Pow(1.0+y/2.0, -float64(k))
				}
				return p5 - p5new
			}
			root, err := rootfinding.Brent(fun, 0.0, 100.0, 6)
			if err != nil {
				panic(err)
			}
			yield := root / 100.0
			// fmt.Println("yield   :", yield)

			// calculate spread_j
			spreadJ := max((yield*98.5/5.78)-p30, 0.0)

			// calculate cash flows and present value
			pv := 0.0
			for t := 0.5; t < 5.0; t += 0.5 {
				// i := int(t / dt)
				ySemi := 2.0 * (math.Pow(Z(rates, t, t+0.5), -0.5) - 1.0)
				//ySemi := 2.0 * (math.Pow(vasi.Z(rates[i], t, t+0.5), -0.5) - 1.0)

				// fmt.Println("t       :", t)
				// fmt.Println("y_t(0.5):", ySemi)

				cf := 0.5 * (5.3/100.0 - (ySemi + spreadCb - 0.75/100.0 + spreadJ)) * notional
				pv += cf * Z(rates, 0, t+0.5)
				//pv += cf * vasi.Z(rates[0], 0, t+0.5)
			}

			return pv
		},
	}

	mcsim := mc.New(&model, 5000)
	err := mcsim.Run()
	if err != nil {
		panic(err)
	}
	average, err := mcsim.Estimate()
	if err != nil {
		panic(err)
	}
	fmt.Printf("Value of levered swap        :%10.4fM USD\n", average)
	fmt.Printf("Exepected value from Veronesi:%10.4fM USD\n\n", 1.5713)

	stderr, err := mcsim.StdError()
	if err != nil {
		panic(err)
	}
	fmt.Printf("Standard error               :%10.4fM USD\n", stderr)
	fmt.Printf("Exepected error from Veronesi:%10.4fM USD\n\n", 0.193)

}
