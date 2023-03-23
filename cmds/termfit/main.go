package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/term"
	"gonum.org/v1/gonum/optimize"
)

const DateFmt = "2006-01-02"

var (
	bonds      []bond.Straight
	prices     []float64
	file       = flag.String("file", "bonddata.csv", fmt.Sprintf("CSV file for bond data with the following fields: maturity date (format: %s), coupon, price", DateFmt))
	settlement = flag.String("date", time.Now().Format(DateFmt), fmt.Sprintf("date of the bond prices (format: %s)", DateFmt))
	saron      = flag.Float64("rate3m", 0.0, "3M SARON (SAR3MC or other 3-month short-term rates) in % (deactivate it by setting it to 0.0)")
	fileFlag   = flag.String("f", "term.json", "json file containing the parameters for term structure")
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

	lastTradingDay, err := time.Parse(DateFmt, *settlement)

	// Saron addition
	if *saron != 0.0 {
		saronBond := bond.Straight{
			Schedule: maturity.Schedule{
				Settlement: lastTradingDay,
				Maturity:   lastTradingDay.AddDate(0, 3, 0),
				Frequency:  4,
				Basis:      "ACT360",
			},
			Coupon:     *saron,
			Redemption: 100.0,
		}
		days := saronBond.Last() * 360.0
		saronPrice := 100.0 / math.Pow(1.0+*saron/100.0/days, 1.0)
		bonds = append(bonds, saronBond)
		prices = append(prices, saronPrice)
		fmt.Println("3-month rate:")
		fmt.Println(" Maturity:", saronBond.Last())
		fmt.Println(" Daycount:", days)
		fmt.Println(" Price:", saronPrice)
	}

	// read starting term structure
	termData, err := ioutil.ReadFile(*fileFlag)
	if err != nil {
		log.Println(err)
	}

	ts, err := term.Parse(termData)
	if err != nil {
		log.Println(err)
	}

	x := []float64{
		-0.421199,
		-0.32659,
		5.02375,
		-4.15252,
		4.7229,
		3.36644,
	}

	if nss, ok := ts.(*term.NelsonSiegelSvensson); ok {
		log.Println("using NSS model from file")
		x[0], x[1], x[2], x[3], x[4], x[5] = nss.B0, nss.B1, nss.B2, nss.B3, nss.T1, nss.T2
	}

	// read in bonddata
	for _, line := range records[1:] {
		maturityDay, err := time.Parse(DateFmt, line[0])
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

		bondS := bond.Straight{
			Schedule: maturity.Schedule{
				Settlement: lastTradingDay,
				Maturity:   maturityDay,
				Frequency:  1,
				Basis:      "30E360",
			},
			Coupon:     coupon,
			Redemption: 100.0,
		}
		bonds = append(bonds, bondS)
		prices = append(prices, price+bondS.Accrued()) // prices are "dirty" prices

	}

	// *******************************************************************
	// optimized NSS
	// *******************************************************************
	fun := func(xf []float64) float64 {
		termNss := term.NelsonSiegelSvensson{
			xf[0], xf[1], xf[2], xf[3], xf[4], xf[5],
			0.0,
		}
		sst := 0.0
		for i, bond := range bonds {
			t := bond.Last()
			quotedPrice := bond.PresentValue(&termNss)
			if t > 0.0 {
				sst += math.Pow(quotedPrice-prices[i], 2.0) / t
			}
		}
		return sst
	}

	// solve optimization problem
	p := optimize.Problem{
		Func: fun,
	}

	termStart := term.NelsonSiegelSvensson{
		x[0], x[1], x[2], x[3], x[4], x[5],
		0.0,
	}

	// fmt.Printf("start.X: %0.4g\n", x)
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
	printTermToFile(&termNss, "nss_opt.json")

	// *******************************************************************
	// optimized Spline
	// *******************************************************************

	// collect all maturities
	xt := []float64{}
	temp := []float64{}
	xtmap := make(map[float64]bool)
	for _, bond := range bonds {
		for _, t := range bond.Schedule.M() {
			xtmap[t] = true
		}
	}

	// sort maturities
	delete(xtmap, 0.0)
	for key, _ := range xtmap {
		temp = append(temp, key)
	}
	sort.Float64s(temp)

	// select only sqrt(k) time points for splines
	for i := 0; i < len(temp); i += int(float64(len(temp)) / math.Sqrt(float64(len(bonds)))) {
		xt = append(xt, temp[i])
	}
	xt[0] = 3.0 / 12.0
	xt[len(xt)-1] = temp[len(temp)-1]
	sort.Float64s(xt)

	// define optimitation function for cubic splines
	funSpline := func(y []float64) float64 {
		termSpline := term.NewSpline(xt, y, 0.0)
		sst := 0.0
		for i, bond := range bonds {
			t := bond.Last()
			if t >= 3.0/12.0 {
				quotedPrice := bond.PresentValue(termSpline)
				sst += math.Pow(quotedPrice-prices[i], 2.0) / t
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
		y[i] = termNss.Z(x)
	}

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
	fmt.Println("x=", xt)
	fmt.Println("y=", result.X)
	// printTerm(termSpline)
	// printTermToFile(termSpline, "spline_opt.json")

	// *******************************************************************
	// print output
	// *******************************************************************
	output := [][]string{}
	for i, bond := range bonds {
		quoteNss := bond.PresentValue(&termNss) - bond.Accrued()
		quoteSpline := bond.PresentValue(termSpline) - bond.Accrued()
		t := bond.Last()
		output = append(output, []string{
			fmt.Sprintf("%v", t),
			fmt.Sprintf("%v", prices[i]),
			fmt.Sprintf("%v", termStart.Rate(t)),
			fmt.Sprintf("%v", termNss.Rate(t)),
			fmt.Sprintf("%v", termSpline.Rate(t)),
			fmt.Sprintf("%v", quoteNss),
			fmt.Sprintf("%v", quoteSpline),
			fmt.Sprintf("%v", termNss.Z(t)),
			fmt.Sprintf("%v", termSpline.Z(t)),
		})

	}

	// write to comparison to result.csv
	fout, err := os.Create("result.csv")
	if err != nil {
		log.Fatal("Unable to read output file: result.csv", err)
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

func printTermToFile(ts term.Structure, name string) error {
	data, err := json.MarshalIndent(ts, " ", "")
	if err != nil {
		return err
	}
	return os.WriteFile(name, data, 0644)
}
