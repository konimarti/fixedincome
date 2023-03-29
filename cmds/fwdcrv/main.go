package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"

	"github.com/konimarti/fixedincome/pkg/term"
)

var (
	fileFlag = flag.String("f", "term.json", "json file containing the parameters for term structure")
	maturity = flag.Float64("m", 1.0, "term maturity of forward rate in decimal years")
	t        = flag.Float64("t", 0.0, "start time for forward rate in decimal years")
)

func main() {
	// read input files
	flag.Parse()

	// read smetarting term structure
	termData, err := ioutil.ReadFile(*fileFlag)
	if err != nil {
		log.Println(err)
	}

	ts, err := term.Parse(termData)
	if err != nil {
		log.Println(err)
	}

	// term maturity
	m := *maturity
	if math.Abs(m) < 1e-16 {
		panic("Term maturity too small")
	}

	// calculate
	t1 := *t
	if math.Abs(t1) > 1e-16 {

		fmt.Println("Discount Factors and Forward Rate")
		fmt.Println("")
		fmt.Printf("Z(0,%4.2f)\t\t%3.4f\n", t1, ts.Z(t1))
		Ffac := ts.Z(t1+m) / ts.Z(t1)
		fmt.Printf("F(0,%4.2f,%4.2f)\t\t%3.4f\n", t1, t1+m, Ffac)
		fmt.Printf("Z(0,%4.2f)\t\t%3.4f\n", t1+m, ts.Z(t1+m))
		fmt.Println("------------------------------")
		fmt.Printf("r(0,%4.2f)\t\t%3.4f%%\n", t1, ts.Rate(t1))
		fmt.Printf("f(0,%4.2f,%4.2f)\t\t%3.4f%%\n", t1, t1+m, -math.Log(Ffac)/m*100.0)
		fmt.Printf("r(0,%4.2f)\t\t%3.4f%%\n", t1+m, ts.Rate(t1+m))
		fmt.Println("------------------------------")

	}

	output := [][]string{
		[]string{"x", "SpotRate", "FwdRate", "Z", "F"},
	}

	for i := 1; i <= 10*12; i++ {
		t := float64(i) * 1.0 / 12.0

		z := ts.Z(t)
		f := ts.Z(t+m) / z

		spot := -math.Log(z) / t * 100.0
		fwd := -math.Log(f) / m * 100.0

		output = append(output, []string{
			fmt.Sprintf("%v", t),
			fmt.Sprintf("%v", spot),
			fmt.Sprintf("%v", fwd),
			fmt.Sprintf("%v", z),
			fmt.Sprintf("%v", f),
		})

	}

	// write to file
	name := "forward.csv"
	fout, err := os.Create(name)
	if err != nil {
		log.Fatal("Unable to read output file:", name, err)
	}
	defer fout.Close()
	w := csv.NewWriter(fout)
	w.WriteAll(output) // calls Flush internally

}
