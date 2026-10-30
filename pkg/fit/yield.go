package fit

import "math"

// priceDuration returns the price sum(amt*exp(-y t)) and the Macaulay
// duration sum(t*amt*exp(-y t))/price (years) of non-negative cash flows.
func priceDuration(t, amt []float64, y float64) (price, duration float64) {
	pt := 0.0
	for i := range t {
		v := amt[i] * math.Exp(-y*t[i])
		price += v
		pt += t[i] * v
	}
	return price, pt / price
}

// logPriceDuration returns ln(sum(exp(la - y t))) and the Macaulay
// duration for the log amounts la (math.Inf(-1) for zero amounts),
// computed with a shifted exponent so that neither overflows nor
// underflows for extreme yields.
func logPriceDuration(t, la []float64, y float64) (lp, duration float64) {
	m := math.Inf(-1)
	for i := range t {
		if e := la[i] - y*t[i]; e > m {
			m = e
		}
	}
	s, st := 0.0, 0.0
	for i := range t {
		v := math.Exp(la[i] - y*t[i] - m)
		s += v
		st += t[i] * v
	}
	return m + math.Log(s), st / s
}

// modelYield returns the continuously compounded yield y (decimal) with
// sum(exp(la - y t)) = price for the log cash-flow amounts la and a
// positive price, starting from y0 (warm start). It applies Newton's
// method to g(y) = ln(PV(y)) - ln(price), which is convex and decreasing
// with slope -D(y) bounded by the first and last cash-flow times, so that
// every iterate is finite and the iteration converges from any start (at
// most one overshoot, then monotonically). It also returns the Macaulay
// duration D(y) (years) at the solution, i.e. dP/dy = -D*price.
func modelYield(t, la []float64, price, y0 float64) (y, duration float64, ok bool) {
	if !(price > 0) || len(t) == 0 || !finiteValue(price) {
		return math.NaN(), math.NaN(), false
	}
	lnP := math.Log(price)
	y = y0
	if !finiteValue(y) {
		y = 0
	}
	step := math.Inf(1)
	for iter := 0; iter < 200; iter++ {
		lp, d := logPriceDuration(t, la, y)
		if !(d > 0) || !finiteValue(lp) {
			return math.NaN(), math.NaN(), false
		}
		step = (lp - lnP) / d
		y += step
		if math.Abs(step) <= 1e-15*math.Max(1, math.Abs(y)) {
			break
		}
	}
	if !(math.Abs(step) <= 1e-12*math.Max(1, math.Abs(y))) {
		return y, math.NaN(), false
	}
	_, d := logPriceDuration(t, la, y)
	return y, d, true
}
