package main

import (
	"fmt"
	"math"
	"time"

	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/rate"
	"github.com/konimarti/fixedincome/pkg/term"
)

// Valuation of an inverse floater

// An plain vanilla inverse floater is a security that pays a lower coupon as interest rates go up (hence the name inverse floater).
// We assume that the inverse floater promises to pay 15% minus the short-term interest rate on an annual basis with 3 years maturity.
// That is, the coupon on the fixed-rate bond is:
//
// c(t) = 15% - r_1(t-1)
//
// where r_1(t-1) denotes the annually compounded rate at time t -1. We assume annual payment frequency for simplicity.
//
// The leveraged inverse floater has a higher parity than one between the floating rate to the fixed rate:
//
// c(t) = 25% - 2 * r_1(t-1)
//
// this means we also must be long two zero coupon bonds.
//
//
// Source: Book by P. Veronesi, Fixed Income Securities, page 61

// InverseFloater is the security to be valued
type InverseFloater struct {
	Weights [3]float64
	// Pz = Zero coupon bond
	Pz bond.Straight
	// Pc = Fixed coupon bond
	Pc bond.Straight
	// Pf = Floating rate bond
	Pf bond.Floating
}

func (f *InverseFloater) PresentValue(ts term.Structure) float64 {
	return f.Weights[0]*f.Pz.PresentValue(ts) + f.Weights[1]*f.Pc.PresentValue(ts) + f.Weights[2]*f.Pf.PresentValue(ts)
}

func main() {

	// define term structure
	ts := &bootstrap{
		t: []float64{1.0, 2.0, 3.0},
		z: []float64{0.9642, 0.9193, 0.8745},
	}

	// define schedule
	schedule := maturity.Schedule{
		Settlement: time.Date(1993, 12, 31, 0, 0, 0, 0, time.UTC),
		Maturity:   time.Date(1996, 12, 31, 0, 0, 0, 0, time.UTC),
		Frequency:  1,
		Basis:      "30E360",
	}

	// define inverse floater with the correct weights
	invfloater := InverseFloater{
		Weights: [3]float64{1.0, 1.0, -1.0},
		Pz: bond.Straight{
			Schedule:   schedule,
			Coupon:     0.0,
			Redemption: 100.0,
		},
		Pc: bond.Straight{
			Schedule:   schedule,
			Coupon:     15.0,
			Redemption: 100.0,
		},
		Pf: bond.Floating{
			Schedule:   schedule,
			Rate:       rate.Annual(ts.Rate(1.0), 1),
			Redemption: 100.0,
		},
	}

	fmt.Printf("Pz=%8.4f\n", invfloater.Pz.PresentValue(ts))
	fmt.Printf("Pc=%8.4f\n", invfloater.Pc.PresentValue(ts))
	fmt.Printf("Pf=%8.4f\n\n", invfloater.Pf.PresentValue(ts))
	fmt.Printf("Value of inverse floater (t=0):")
	fmt.Printf("%10.4f\n", invfloater.PresentValue(ts))
	fmt.Printf("Exepected value from Veronesi :%10.4f\n", 116.28)
	fmt.Println("")

	// create the leveraged inverse floater
	levinvfloater := invfloater
	levinvfloater.Weights = [3]float64{2.0, 1.0, -2.0}
	levinvfloater.Pc.Coupon = 25.0

	fmt.Printf("Pz=%8.4f\n", levinvfloater.Pz.PresentValue(ts))
	fmt.Printf("Pc=%8.4f\n", levinvfloater.Pc.PresentValue(ts))
	fmt.Printf("Pf=%8.4f\n\n", levinvfloater.Pf.PresentValue(ts))
	fmt.Printf("Value of leveraged inverse floater (t=0):")
	fmt.Printf("%10.4f\n", levinvfloater.PresentValue(ts))
	fmt.Printf("Exepected value from Veronesi           :%10.4f\n", 131.32)
	fmt.Println("")

}

// boostrap implements a term structure in order to be in agreement with the book by Veronesi
// (on page 64 in the book)
type bootstrap struct {
	t []float64
	z []float64
}

func (b *bootstrap) SetSpread(t float64) term.Structure {
	return b
}

func (b *bootstrap) Rate(t float64) float64 {
	return -math.Log(b.Z(t)) / t * 100.0
}

func (b *bootstrap) Z(t float64) float64 {
	for i, m := range b.t {
		if math.Abs(m-t) < 0.05 {
			return b.z[i]
		}
	}
	panic("asking for Z(t) that cannot be bootstrapped")
	return 0.0
}
