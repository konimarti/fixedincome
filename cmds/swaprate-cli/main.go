package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"

	"github.com/konimarti/bonds/pkg/instrument/swap"
	"github.com/konimarti/bonds/pkg/term"
)

var (
	spread   = flag.Float64("s", 10.0, "spread in bps for yield curve")
	fileFlag = flag.String("f", "term.json", "json file containing the parameters for the Nelson-Siegel-Svensson term structure")
)

func main() {
	flag.Parse()

	// read term structure parameters
	termData, err := ioutil.ReadFile(*fileFlag)
	if err != nil {
		log.Println(err)
	}

	ts, err := term.Parse(termData)
	if err != nil {
		log.Println(err)
		log.Println("Use the following template for the Nelson-Siegel-Svensson yield curve:")
		data, err := json.MarshalIndent(term.NelsonSiegelSvensson{}, " ", "")
		if err != nil {
			panic(err)
		}
		fmt.Println(string(data))
		return
	}

	// add spread to term structure
	ts.SetSpread(*spread)

	// calculate swap rate for maturities t
	fmt.Println("Maturity\tSwap Rate")
	for t := 2.0; t <= 10.0; t += 1.0 {
		m := []float64{}
		for k := 0.5; k <= t; k += 0.5 {
			m = append(m, k)
		}
		swaprate, err := swap.InterestRate(m, 2, ts)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Printf("%2.1f\t\t%6.2f\n", t, swaprate)
	}
}
