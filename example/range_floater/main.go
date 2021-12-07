package main

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/konimarti/fixedincome/pkg/mc"
	"github.com/konimarti/fixedincome/pkg/mc/model/vasicek"
)

// Valuation of an range floater

// Term sheet of the range floater:
//
// Today: 		1.11.2004
// Maturity: 		1.11.2007
// Frequency: 		Quarterly
// Notional: 		100.0
// Coupon: 		CF(t) = Notional * (Libor_(t-1) + Spread)
// 				* Accrual factor / number of days
// Spread over floating: 1%
// Number of days: 	360
// Accrual factor: 	Number of fdas during the relevant interest rate period
// 			that the 3-month LIBOR is within the range [1.18%,4.18%]
// Current 3-month LIBOR: 2.18%
//
// Source: Book by P. Veronesi, Fixed Income Securities, page 604

func main() {

	T := 3.0
	N := int(360 * 3)
	model := vasicek.Vasicek{
		R0:    2.17 / 100.0,
		Rbar:  6.410 / 100.0,
		Gamma: 0.210,
		Sigma: 0.82 / 100.0,
		T:     T,
		N:     N,
		Rng:   rand.New(rand.NewSource(time.Now().UnixNano())),
		// Rng:   rand.New(rand.NewSource(99)),
		Payoff: func(rates []float64) float64 {
			// calculate 3-month libor
			libor3m := make([]float64, len(rates))
			for i, ri := range rates {
				libor3m[i] = 360.0 / 90.0 * (math.Exp(ri*0.25) - 1.0)
			}
			// make 90 days segements and count days
			q := int(T / 0.25)
			days := make([]int, q)
			for i, libor := range libor3m {
				if libor > 1.18/100.0 && libor < 4.18/100.0 {
					days[i/90] += 1
				}
			}
			// calculate quarterly cash flows
			cashflow := make([]float64, q)
			notional := 100.0
			spread := 1.0 / 100.0
			for i, d := range days {
				cashflow[i] = notional * (libor3m[i*90] + spread) * float64(d) / 360.0
			}
			// calculate present value of cash flows
			pv := 0.0
			discountRate := 1.0
			for i, cf := range cashflow {
				discountRate *= (1.0 + libor3m[i*90]/4.0)
				pv += cf / discountRate
			}
			pv += notional / discountRate
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
	fmt.Printf("Value of range floater (t=0) :%10.4f\n", average)
	fmt.Printf("Exepected value from Veronesi:%10.4f\n\n", 99.8385)

	stderr, err := mcsim.StdError()
	if err != nil {
		panic(err)
	}
	fmt.Printf("Standard error               :%10.4f\n", stderr)
	fmt.Printf("Exepected error from Veronesi:%10.4f\n\n", 0.0505)

	// lower, upper, err := mcsim.CI()
	// if err != nil {
	// 	panic(err)
	// }
	// fmt.Println("Confidence interval:", lower, upper)

	// fmt.Printf("Exepected value from Veronesi :%10.4f\n", 116.28)
	// fmt.Println("")

}
