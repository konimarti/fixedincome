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

	"github.com/konimarti/fixedincome"
	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/rate"
	"github.com/konimarti/fixedincome/pkg/term"
	"gonum.org/v1/gonum/optimize"
)

const DateFmt = "2006-01-02"

var (
	bonds  []bond.Straight
	prices []float64 // "dirty" prices
	yields []float64 // "dirty" prices
)

var (
	file       = flag.String("file", "bonddata.csv", fmt.Sprintf("CSV file for bond data with the following fields: maturity date (format: %s), coupon, price", DateFmt))
	settlement = flag.String("date", time.Now().Format(DateFmt), fmt.Sprintf("date of the bond prices (format: %s)", DateFmt))
	onRate     = flag.Float64("onrate", 0.0, "Overnight rate (e.g. Swiss Average Rate Overnight) in % (deactivate it by setting it to 0.0)")
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

	// read starting term structure
	termData, err := ioutil.ReadFile(*fileFlag)
	if err != nil {
		log.Println(err)
	}

	ts, err := term.Parse(termData)
	if err != nil {
		log.Println(err)
	}

	// convert annual O/N rate to a continuously compounded rate
	onCC := rate.Continuous(*onRate, 360)

	// if an overnight rate is given, use a constraint in the optimization
	// procedure
	conOpt := math.Abs(onCC) > 1e-7
	if conOpt {
		log.Println("using O/N constraint")
	}

	// initial term structure parameters for the optimization procedure
	x := []float64{-0.421199, -0.32659, 5.02375, -4.15252, 4.7229, 3.36644}

	if nss, ok := ts.(*term.NelsonSiegelSvensson); ok {
		log.Println("using model from file")
		x[0], x[1], x[2], x[3], x[4], x[5] = nss.B0, nss.B1, nss.B2, nss.B3, nss.T1, nss.T2
	}

	if conOpt {
		fmt.Println("")
		fmt.Printf("O/N rate:                         %3.4f %%\n", *onRate)
		fmt.Printf("continuously compounded O/N rate: %3.4f %%\n", onCC)
		fmt.Printf("implied starting O/N rate:        %3.4f %%\n", x[0]+x[1])
		fmt.Println("")
	}

	// read in the bonds
	for _, line := range records[0:] {
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

		bnd := bond.Straight{
			Schedule: maturity.Schedule{
				Settlement: lastTradingDay,
				Maturity:   maturityDay,
				Frequency:  1,
				Basis:      "30E360",
			},
			Coupon:     coupon,
			Redemption: 100.0,
		}
		bonds = append(bonds, bnd)
		// reminder: prices are "dirty"
		dirty := price + bnd.Accrued()
		prices = append(prices, dirty)
		yield, err := fixedincome.Irr(dirty, &bnd)
		if err != nil {
			log.Println(err)
			yield = math.NaN()
		}
		yields = append(yields, yield)

	}

	// *******************************************************************
	// optimized NSS
	// *******************************************************************
	fun := func(xf []float64) float64 {

		ts := &term.NelsonSiegelSvensson{
			xf[0], xf[1], xf[2], xf[3], xf[4], xf[5],
			0.0,
		}

		penalty := 1.0e3
		scale := float64(len(bonds))
		sst := 0.0

		// add contraints penalty
		if conOpt {
			sst += scale * math.Pow(onCC-(xf[0]+xf[1]), 2.0)
			if math.Abs(xf[0])+math.Abs(xf[1]) > 10 {
				sst += penalty
			}
		}

		for i, bond := range bonds {
			// minimize least-squares of yields
			if math.IsNaN(yields[i]) {
				continue
			}

			value := bond.PresentValue(ts)
			est, err := fixedincome.Irr(value, &bond)
			if err != nil {
				log.Printf("yield for bond [%d] not "+
					"converged\n", i)
				sst += penalty
				continue
			}

			sst += math.Pow(yields[i]-est, 2.0)
		}
		return sst
	}

	termStart := term.NelsonSiegelSvensson{
		x[0], x[1], x[2], x[3], x[4], x[5],
		0.0,
	}

	// solve optimization problem
	p := optimize.Problem{
		Func: fun,
	}

	log.Println("minimize squared error of yields..")

	result, err := optimize.Minimize(p, x, nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	if err = result.Status.Err(); err != nil {
		log.Fatal(err)
	}
	printResult(result)

	log.Println("..done")

	// store optmizated parameters
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

	xt := append([]float64{temp[0]}, determine_weights(temp, len(temp)/int(math.Sqrt(float64(len(bonds)))))...)
	xt = append(xt, temp[len(temp)-1])

	// define optimization function for the (cubic) splines
	funSpline := func(y []float64) float64 {
		ts := term.NewSpline(xt, y, 0.0)
		sst := 0.0
		penalty := 1.0e3
		for i, bond := range bonds {
			// minimize least-squares of yields
			if math.IsNaN(yields[i]) {
				continue
			}

			value := bond.PresentValue(ts)
			est, err := fixedincome.Irr(value, &bond)
			if err != nil {
				log.Printf("yield for bond [%d] not "+
					"converged\n", i)
				sst += penalty
				continue
			}

			sst += math.Pow(yields[i]-est, 2.0)
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
	fmt.Println("Cubic spline term structure")
	fmt.Println("Maturities      :", xt)
	fmt.Println("Discount Factors:", result.X)
	// printTerm(termSpline)
	printTermToFile(termSpline, "spline_opt.json")

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

// determine_weights returns n weights that will split t in (almost) equal
// chunkgs
func determine_weights(t []float64, n int) []float64 {
	// initialize with equal weights
	w := make([]float64, n)

	step := len(t) / (n + 1)

	for i := 0; i < n; i++ {
		idx := (i + 1) * step
		if idx-1 > 0 && idx < len(t) {
			w[i] = (t[idx] + t[idx-1]) * 0.5
		}
	}

	return w
}
