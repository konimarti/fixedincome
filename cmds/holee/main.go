// Command holee calibrates the Ho-Lee short rate model to a term structure
// and checks the calibration by Monte Carlo: the simulated zero-coupon bond
// prices E[exp(-∫_0^t r ds)] are compared with the curve on all grid nodes.
//
// Usage:
//
//	holee [-f term.json] [-n 1000] [-m 10] [-s 0.02] [-paths 10000] [-seed 0] [-o holee.csv]
//
// The CSV (header, one row per grid node t_0 = 0, ..., t_N = T; all rates
// continuously compounded decimals, discount factors per unit face value)
// is written to stdout or the file given by -o; the summary is written to
// stderr. Columns:
//
//	t                  time in years
//	zero_rate          curve zero rate
//	fwd_rate           curve instantaneous forward rate f(0,t)
//	theta              Ho-Lee drift θ(t) = f'(0,t) + σ²t (per year)
//	exp_short_rate     E[r(t)] = f(0,t) + σ²t²/2
//	discount           curve discount factor Z(0,t)
//	mc_discount        Monte Carlo estimate of E[exp(-∫_0^t r)]
//	mc_stderr          standard error of mc_discount
//	mc_zero_rate       -ln(mc_discount)/t (the short rate at t = 0)
//
// Plot with: Rscript analysis.R holee.csv holee.png (the defaults)
package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/konimarti/fixedincome/pkg/mc"
	"github.com/konimarti/fixedincome/pkg/mc/model/holee"
	"github.com/konimarti/fixedincome/pkg/term"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("holee: ")
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		log.Fatal(err)
	}
}

func run(args []string, stdout, stderr io.Writer) (err error) {
	fs := flag.NewFlagSet("holee", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("f", "term.json", "JSON file with the term structure (see pkg/term)")
	n := fs.Int("n", 1000, "number of time steps (>= 1)")
	horizon := fs.Float64("m", 10.0, "horizon (maturity) of the simulation in years")
	sigma := fs.Float64("s", 0.02, "volatility of the short rate as a decimal per square root of a year (0.01 = 100bp)")
	paths := fs.Int("paths", 10000, "number of Monte Carlo paths")
	seed := fs.Uint64("seed", 0, "seed of the random number generator (0: time-based seed, printed)")
	out := fs.String("o", "-", "output CSV file ('-' for stdout)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %v", fs.Args())
	}

	data, err := os.ReadFile(*file)
	if err != nil {
		return fmt.Errorf("reading term structure: %w", err)
	}
	curve, _, err := term.Parse(data)
	if err != nil {
		return fmt.Errorf("parsing term structure from %s: %w", *file, err)
	}
	g, err := mc.NewUniformGrid(*horizon, *n)
	if err != nil {
		return err
	}
	model, err := holee.New(curve, *sigma)
	if err != nil {
		return err
	}
	s := *seed
	if s == 0 {
		s = uint64(time.Now().UnixNano())
	}

	// open the output before the (possibly long) simulation
	w := stdout
	if *out != "-" {
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		defer func() {
			if cerr := f.Close(); err == nil && cerr != nil {
				err = cerr
			}
		}()
		w = f
	}

	res, err := mc.PriceMulti(model, mc.ZeroBonds{Grid: g}, g, mc.Options{Paths: *paths, Seed: s})
	if err != nil {
		return err
	}

	// CSV
	bw := bufio.NewWriter(w)
	cw := csv.NewWriter(bw)
	if err := cw.Write([]string{"t", "zero_rate", "fwd_rate", "theta", "exp_short_rate", "discount", "mc_discount", "mc_stderr", "mc_zero_rate"}); err != nil {
		return err
	}
	ff := func(x float64) string { return strconv.FormatFloat(x, 'g', 12, 64) }
	r0 := model.R0()
	for i, t := range g.Times {
		mcZero := r0
		if t > 0 {
			mcZero = -math.Log(res[i].Mean) / t
		}
		if err := cw.Write([]string{
			ff(t), ff(curve.ZeroRate(t)), ff(term.InstForward(curve, t)), ff(model.Theta(t)),
			ff(model.Phi(t)), ff(curve.Discount(t)), ff(res[i].Mean), ff(res[i].StdErr), ff(mcZero),
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return err
	}
	if err := bw.Flush(); err != nil {
		return err
	}

	// summary
	fmt.Fprintf(stderr, "Ho-Lee model: T=%v, N=%d, dt=%v, σ=%v, r0=%.6f, paths=%d, seed=%d\n",
		g.T(), g.Steps(), g.Dt(0), *sigma, r0, *paths, s)
	fmt.Fprintf(stderr, "%8s %12s %12s %10s %8s\n", "t", "100·Z(0,t)", "100·E[Df]", "100·SE", "dev/SE")
	step := g.Steps() / 10
	if step < 1 {
		step = 1
	}
	underflow, tinySE, unreliable := false, false, false
	for i := step; i <= g.Steps(); i += step {
		z := curve.Discount(g.Times[i])
		m, se := res[i].Mean, res[i].StdErr
		dev := "n/a"
		switch {
		case !(m > 1e-8*z):
			// (practically) all paths underflow
			underflow = true
		case !(se > 1e-12*m):
			// deterministic up to rounding: a ratio would be noise
			tinySE = *sigma > 0
		default:
			d := (m - z) / se
			dev = fmt.Sprintf("%.2f", d)
			if math.Abs(d) > 1000 {
				dev = fmt.Sprintf("%.1e", d)
				unreliable = true
			}
		}
		fmt.Fprintf(stderr, "%8.4g %12.6f %12.6f %10.6f %8s\n", g.Times[i], 100*z, 100*m, 100*se, dev)
	}
	if underflow {
		fmt.Fprintln(stderr, "warning: simulated discount factors underflow (σ too large for the horizon); the estimates are meaningless")
	}
	if unreliable {
		fmt.Fprintln(stderr, "warning: Monte Carlo estimates differ from the curve by more than 1000 standard errors: the discount factors are too skewed (σ too large) for this number of paths")
	}
	if tinySE {
		fmt.Fprintln(stderr, "warning: standard errors are at rounding level (horizon too short for σ); dev/SE not shown")
	}
	return nil
}
