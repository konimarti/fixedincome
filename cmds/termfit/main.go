package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/konimarti/bonds/pkg/instrument/bond"
	"github.com/konimarti/bonds/pkg/maturity"
	"github.com/konimarti/bonds/pkg/term"
	"gonum.org/v1/gonum/optimize"
)

var (
	bonds      []bond.Straight
	prices     []float64
	file       = flag.String("file", "bonddata.csv", "CSV file for bond data with the following fields: maturity date (format: 02.01.2006), coupon, price")
	settlement = flag.String("date", "26.11.2021", "date of the bond prices (format: 02.01.2006)")
)

func main() {
	// read input files
	flag.Parse()

	// read bond data from CSV file
	filePath := *file
	f, err := os.Open(filePath)
	if err != nil {
		log.Fatal("Unable to read input file "+filePath, err)
	}
	defer f.Close()
	csvReader := csv.NewReader(f)
	records, err := csvReader.ReadAll()
	if err != nil {
		log.Fatal("Unable to parse file as CSV for "+filePath, err)
	}

	lastTradingDay, err := time.Parse("02.01.2006", *settlement)
	// lastTradingDay := time.Date(2021, 11, 26, 0, 0, 0, 0, time.UTC)
	for _, line := range records[1:] {
		maturityDay, err := time.Parse("02.01.2006", line[0])
		if err != nil {
			panic(err)
		}
		coupon, err := strconv.ParseFloat(line[1], 64)
		if err != nil {
			panic(err)
		}
		price, err := strconv.ParseFloat(line[2], 64)
		if err != nil {
			panic(err)
		}

		bond := bond.Straight{
			Schedule: maturity.Schedule{
				Settlement: lastTradingDay,
				Maturity:   maturityDay,
				Frequency:  1,
				Basis:      "30E360",
			},
			Coupon:     coupon,
			Redemption: 100.0,
		}
		bonds = append(bonds, bond)
		prices = append(prices, price)
	}

	// *******************************************************************
	// optimized NSS
	// *******************************************************************
	fun := func(x []float64) float64 {
		termNss := term.NelsonSiegelSvensson{
			x[0],
			x[1],
			x[2],
			x[3],
			x[4],
			x[5],
			0.0,
		}
		sst := 0.0
		for i, bond := range bonds {
			t := bond.YearsToMaturity()
			if t >= 3.0/12.0 {
				quotedPrice := bond.PresentValue(&termNss) // aka clean price
				sst += math.Pow(quotedPrice-prices[i], 2.0) / (t * t)
			}
		}
		return sst
	}

	// solve optimization problem
	p := optimize.Problem{
		Func: fun,
	}

	x := []float64{-0.352323, -0.392947, 5.34703, -3.93181, 4.8696, 3.87489}
	// fmt.Printf("start.X: %0.4g\n", x)
	termStart := term.NelsonSiegelSvensson{
		x[0],
		x[1],
		x[2],
		x[3],
		x[4],
		x[5],
		0.0,
	}

	result, err := optimize.Minimize(p, x, nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	if err = result.Status.Err(); err != nil {
		log.Fatal(err)
	}
	printResult(result)

	// print out price comparison
	termNss := term.NelsonSiegelSvensson{
		result.X[0],
		result.X[1],
		result.X[2],
		result.X[3],
		result.X[4],
		result.X[5],
		0.0,
	}
	fmt.Println("Nelson-Siegel-Svensson term structure:")
	printTerm(&termNss)

	// *******************************************************************
	// optimized Spline
	// *******************************************************************

	// find all maturities in xt for interpolation
	xt := []float64{}
	xtmap := make(map[float64]bool)
	for _, bond := range bonds {
		for _, t := range bond.Schedule.M() {
			xtmap[math.Round(t*1.0)/1.0] = true
		}
	}
	for key, _ := range xtmap {
		xt = append(xt, key)
	}
	sort.Float64s(xt)

	funSpline := func(y []float64) float64 {
		termSpline := term.NewSpline(xt, y, 0.0)
		sst := 0.0
		for i, bond := range bonds {
			t := bond.YearsToMaturity()
			if t >= 1.5/12.0 {
				quotedPrice := bond.PresentValue(termSpline) // aka clean price
				sst += math.Pow(quotedPrice-prices[i], 2.0)
			}
		}
		return sst
	}

	// solve optimization problem
	p = optimize.Problem{
		Func: funSpline,
	}

	// use flat estimates
	y := make([]float64, len(xt))
	for i, x := range xt {
		y[i] = termStart.Rate(x)
	}
	// fmt.Printf("start.X: %0.4g\n", y)

	result, err = optimize.Minimize(p, y, nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	if err = result.Status.Err(); err != nil {
		log.Fatal(err)
	}
	printResult(result)

	// print out price comparison
	termSpline := term.NewSpline(xt, result.X, 0.0)
	// fmt.Println("Cubic spline term structure:")
	// printTerm(termSpline)

	// *******************************************************************
	// print output
	// *******************************************************************
	output := [][]string{}
	for i, bond := range bonds {
		quoteNss := bond.PresentValue(&termNss) - bond.Accrued()
		quoteSpline := bond.PresentValue(termSpline) - bond.Accrued()
		t := bond.YearsToMaturity()
		output = append(output, []string{
			fmt.Sprintf("%v", t),
			fmt.Sprintf("%v", prices[i]),
			fmt.Sprintf("%v", termStart.Rate(t)),
			fmt.Sprintf("%v", termNss.Rate(t)),
			fmt.Sprintf("%v", termSpline.Rate(t)),
			fmt.Sprintf("%v", quoteNss),
			fmt.Sprintf("%v", quoteSpline),
		})

	}

	// write to comparison to result.csv
	fout, err := os.Create("result.csv")
	if err != nil {
		log.Fatal("Unable to read input file: result.csv", err)
	}
	defer fout.Close()
	w := csv.NewWriter(fout)
	w.WriteAll(output) // calls Flush internally

}

func printResult(result *optimize.Result) {
	fmt.Println("Optimization results:")
	fmt.Printf("Status: %v\n", result.Status)
	fmt.Printf("X     : %0.4g\n", result.X)
	fmt.Printf("F     : %0.4g\n", result.F)
	fmt.Printf("Stats.FuncEvaluations: %d\n\n", result.Stats.FuncEvaluations)
}

func printTerm(ts term.Structure) {
	text, err := json.MarshalIndent(ts, " ", "")
	if err != nil {
		log.Println(err)
		return
	}
	fmt.Println(string(text))
}
