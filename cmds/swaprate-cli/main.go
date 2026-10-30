package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/konimarti/fixedincome/pkg/instrument/swap"
	"github.com/konimarti/fixedincome/pkg/term"
)

var (
	spread   = flag.Float64("s", 0.0, "spread in bps for yield curve")
	fileFlag = flag.String("f", "term.json", "json file containing the parameters for the Nelson-Siegel-Svensson term structure")
)

func main() {
	flag.Parse()

	// read term structure parameters
	termData, err := ioutil.ReadFile(*fileFlag)
	if err != nil {
		fatalTerm(fmt.Errorf("reading term structure failed: %w", err))
	}

	ts, _, err := term.Parse(termData)
	if err != nil {
		fatalTerm(fmt.Errorf("parsing term structure from %s failed: %w", *fileFlag, err))
	}
	fmt.Fprintln(os.Stderr, "Term model read from", *fileFlag)

	// set the spread of the term structure explicitly (flag in bp; replaces
	// the spread of the curve file)
	ts, _ = term.Spread(ts)
	ts = term.WithSpread(ts, *spread/1e4)
	fmt.Fprintf(os.Stderr, "Spread: %.2f bps\n", *spread)

	// calculate swap rate for maturities t
	fmt.Println("Maturity\tSwap Rate")
	for t := 2.0; t <= 10.0; t += 1.0 {
		m := []float64{}
		for k := 0.5; k <= t; k += 0.5 {
			m = append(m, k)
		}
		swaprate, err := swap.ParRateFromTimes(m, 2, ts)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Printf("%2.1f\t\t%6.2f\n", t, swaprate*100.0)
	}
}

// fatalTerm prints the error and a template for the term structure file to
// stderr and exits with a non-zero exit code
func fatalTerm(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	fmt.Fprintln(os.Stderr, "Use the following template for the Nelson-Siegel-Svensson yield curve (betas in percent):")
	fmt.Fprintln(os.Stderr, `{"model": "nss", "params": {"b0": -0.266372, "b1": -0.471343, "b2": 5.68789, "b3": -5.12324, "t1": 5.74881, "t2": 4.14426}, "spread_bp": 0}`)
	os.Exit(1)
}
