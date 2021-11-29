package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/konimarti/bonds/pkg/bond"
	"github.com/konimarti/bonds/pkg/maturity"
	"github.com/konimarti/bonds/pkg/term"
	"gonum.org/v1/gonum/optimize"
)

var (
	bonds  []bond.Straight
	prices []float64
	file   = flag.String("file", "bonddata.csv", "CSV file for bond data with the following fields: maturity date (format: 02.01.2006), coupon, price")
)

func main() {
	// read input files
	flag.Parse()
	// read data
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

	lastTradingDay := time.Date(2021, 11, 26, 0, 0, 0, 0, time.UTC)
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

	fun := func(x []float64) float64 {
		term := term.NelsonSiegelSvensson{
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
				sst += math.Pow(bond.PresentValue(&term)-prices[i], 2.0) / (t * t)
			}
		}
		return sst
	}

	// solve optimization problem
	p := optimize.Problem{
		Func: fun,
	}

	x := []float64{-0.352323, -0.392947, 5.34703, -3.93181, 4.8696, 3.87489}
	fmt.Printf("statr.X: %0.4g\n", x)
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
	fmt.Printf("result.Status: %v\n", result.Status)
	fmt.Printf("result.X: %0.4g\n", result.X)
	fmt.Printf("result.F: %0.4g\n", result.F)
	fmt.Printf("result.Stats.FuncEvaluations: %d\n", result.Stats.FuncEvaluations)

	// print out price comparison
	term := term.NelsonSiegelSvensson{
		result.X[0],
		result.X[1],
		result.X[2],
		result.X[3],
		result.X[4],
		result.X[5],
		0.0,
	}
	output := [][]string{}
	for i, bond := range bonds {
		value := bond.PresentValue(&term)
		t := bond.YearsToMaturity()
		output = append(output, []string{
			fmt.Sprintf("%v", t),
			fmt.Sprintf("%v", termStart.Rate(t)),
			fmt.Sprintf("%v", term.Rate(t)),
			fmt.Sprintf("%v", prices[i]),
			fmt.Sprintf("%v", value),
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
