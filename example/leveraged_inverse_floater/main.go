package main

import (
	"fmt"
	"math"
	"time"

	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/maturity"
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

// PresentValue returns the value of the inverse floater; ts is used as the
// discount and forecast curve
func (f *InverseFloater) PresentValue(settle time.Time, ts term.Curve) float64 {
	return f.Weights[0]*must(f.Pz.DirtyPrice(settle, ts)) +
		f.Weights[1]*must(f.Pc.DirtyPrice(settle, ts)) +
		f.Weights[2]*must(f.Pf.PresentValue(settle, ts, ts))
}

func mustStraight(b bond.Straight, err error) bond.Straight {
	if err != nil {
		panic(err)
	}
	return b
}

func mustFloating(f bond.Floating, err error) bond.Floating {
	if err != nil {
		panic(err)
	}
	return f
}

func must(v float64, err error) float64 {
	if err != nil {
		panic(err)
	}
	return v
}

func main() {

	// define term structure
	ts := &bootstrap{
		t: []float64{1.0, 2.0, 3.0},
		z: []float64{0.9642, 0.9193, 0.8745},
	}

	// define schedule
	settle := time.Date(1993, 12, 31, 0, 0, 0, 0, time.UTC)
	schedule := maturity.Schedule{
		Maturity:  time.Date(1996, 12, 31, 0, 0, 0, 0, time.UTC),
		Frequency: maturity.Annual,
		DayCount:  maturity.Thirty360E,
	}

	// define inverse floater with the correct weights
	invfloater := InverseFloater{
		Weights: [3]float64{1.0, 1.0, -1.0},
		Pz:      mustStraight(bond.NewStraight(schedule, 0.0)),
		Pc:      mustStraight(bond.NewStraight(schedule, 0.15)),
		// the coupons are reset annually at the annually compounded rate,
		// which is projected from the curve (settlement on a reset date,
		// no current fixing)
		Pf: mustFloating(bond.NewFloating(schedule, 0.0)),
	}

	fmt.Printf("Pz=%8.4f\n", must(invfloater.Pz.DirtyPrice(settle, ts)))
	fmt.Printf("Pc=%8.4f\n", must(invfloater.Pc.DirtyPrice(settle, ts)))
	fmt.Printf("Pf=%8.4f\n\n", must(invfloater.Pf.PresentValue(settle, ts, ts)))
	fmt.Printf("Value of inverse floater (t=0):")
	fmt.Printf("%10.4f\n", invfloater.PresentValue(settle, ts))
	fmt.Printf("Exepected value from Veronesi :%10.4f\n", 116.28)
	fmt.Println("")

	// create the leveraged inverse floater
	levinvfloater := invfloater
	levinvfloater.Weights = [3]float64{2.0, 1.0, -2.0}
	levinvfloater.Pc.Coupon = 0.25

	fmt.Printf("Pz=%8.4f\n", must(levinvfloater.Pz.DirtyPrice(settle, ts)))
	fmt.Printf("Pc=%8.4f\n", must(levinvfloater.Pc.DirtyPrice(settle, ts)))
	fmt.Printf("Pf=%8.4f\n\n", must(levinvfloater.Pf.PresentValue(settle, ts, ts)))
	fmt.Printf("Value of leveraged inverse floater (t=0):")
	fmt.Printf("%10.4f\n", levinvfloater.PresentValue(settle, ts))
	fmt.Printf("Exepected value from Veronesi           :%10.4f\n", 131.32)
	fmt.Println("")

}

// boostrap implements a term structure in order to be in agreement with the book by Veronesi
// (on page 64 in the book)
type bootstrap struct {
	t []float64
	z []float64
}

// ZeroRate returns the continuously compounded zero rate (decimal)
func (b *bootstrap) ZeroRate(t float64) float64 {
	return -math.Log(b.Discount(t)) / t
}

// Discount returns the bootstrapped discount factor (1 for t close to 0)
func (b *bootstrap) Discount(t float64) float64 {
	if math.Abs(t) < 0.05 {
		return 1.0
	}
	for i, m := range b.t {
		if math.Abs(m-t) < 0.05 {
			return b.z[i]
		}
	}
	panic("asking for Z(t) that cannot be bootstrapped")
}
