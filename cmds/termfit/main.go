package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/konimarti/fixedincome/pkg/cashflow"
	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/rate"
	"github.com/konimarti/fixedincome/pkg/term"
	"gonum.org/v1/gonum/optimize"
)

const DateFmt = "2006-01-02"

// dateFormats are the accepted formats for the maturity dates in the CSV file
var dateFormats = []string{DateFmt, "02.01.2006"}

var (
	bonds   []bond.Straight
	flows   [][]cashflow.CashFlow // cash flows of the bonds after the settlement date
	accrued []float64             // accrued interest of the bonds
	quotes  []float64             // quoted "clean" prices
	prices  []float64             // "dirty" prices
	yields  []float64             // yields of the "dirty" prices (cc, in percent)
)

// yieldPercent returns the continuously compounded yield in percent (the
// unit of the objective functions)
func yieldPercent(dirty float64, cfs []cashflow.CashFlow) (float64, error) {
	y, err := cashflow.YieldCC(dirty, cfs)
	return y * 100.0, err
}

// nssFromParams returns the NSS curve for the optimization parameters
// (betas in percent, decay parameters in years)
func nssFromParams(x []float64) term.NSS {
	return term.NSS{B0: x[0] / 100, B1: x[1] / 100, B2: x[2] / 100, B3: x[3] / 100, Tau1: x[4], Tau2: x[5]}
}

var (
	file       = flag.String("file", "bonddata.csv", fmt.Sprintf("CSV file for bond data with the following fields: maturity date (format: %s or DD.MM.YYYY), coupon, price; an optional header row is skipped", DateFmt))
	settlement = flag.String("date", time.Now().Format(DateFmt), fmt.Sprintf("date of the bond prices (format: %s or DD.MM.YYYY)", DateFmt))
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

	lastTradingDay, err := parseDate(*settlement)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid -date: %v\n", err)
		os.Exit(1)
	}

	// read starting term structure
	termData, err := ioutil.ReadFile(*fileFlag)
	if err != nil {
		log.Println(err)
	}

	ts, _, err := term.Parse(termData)
	if err != nil {
		log.Printf("no valid starting term structure in %s (%v), using default parameters", *fileFlag, err)
	}

	// convert annual O/N rate to a continuously compounded rate
	onCC, err := rate.ToContinuous(*onRate/100.0, 360)
	if err != nil {
		log.Fatal(err)
	}
	onCC *= 100.0 // percent

	// if an overnight rate is given, use a constraint in the optimization
	// procedure
	conOpt := math.Abs(onCC) > 1e-7
	if conOpt {
		log.Println("using O/N constraint")
	}

	// initial term structure parameters for the optimization procedure
	x := []float64{-0.421199, -0.32659, 5.02375, -4.15252, 4.7229, 3.36644}

	if ts != nil {
		base, _ := term.Spread(ts)
		if nss, ok := base.(term.NSS); ok {
			log.Println("using model from file")
			x[0], x[1], x[2], x[3], x[4], x[5] = nss.B0*100, nss.B1*100, nss.B2*100, nss.B3*100, nss.Tau1, nss.Tau2
		}
	}

	if conOpt {
		fmt.Println("")
		fmt.Printf("O/N rate:                         %3.4f %%\n", *onRate)
		fmt.Printf("continuously compounded O/N rate: %3.4f %%\n", onCC)
		fmt.Printf("implied starting O/N rate:        %3.4f %%\n", x[0]+x[1])
		fmt.Println("")
	}

	// skip header row
	if len(records) > 0 && isHeader(records[0]) {
		records = records[1:]
	}

	// read in the bonds
	for nr, line := range records {
		if len(line) < 3 {
			log.Fatalf("%s: row %d: expected at least 3 fields (maturity, coupon, price), got %d", filePath, nr+1, len(line))
		}
		maturityDay, err := parseDate(line[0])
		if err != nil {
			log.Fatalf("%s: row %d: %v", filePath, nr+1, err)
		}
		coupon, err := strconv.ParseFloat(line[1], 64)
		if err != nil {
			log.Fatalf("%s: row %d: invalid coupon: %v", filePath, nr+1, err)
		}
		price, err := strconv.ParseFloat(line[2], 64)
		if err != nil {
			log.Fatalf("%s: row %d: invalid price: %v", filePath, nr+1, err)
		}
		if !lastTradingDay.Before(maturityDay) {
			log.Printf("%s: row %d: bond matured on %s, skipped", filePath, nr+1, maturityDay.Format(DateFmt))
			continue
		}

		bnd, err := bond.NewStraight(maturity.Schedule{
			Maturity:  maturityDay,
			Frequency: maturity.Annual,
			DayCount:  maturity.Thirty360E,
		}, coupon/100.0)
		if err != nil {
			log.Fatalf("%s: row %d: %v", filePath, nr+1, err)
		}
		cfs, err := bnd.CashFlows(lastTradingDay)
		if err != nil {
			log.Fatalf("%s: row %d: %v", filePath, nr+1, err)
		}
		acc, err := bnd.Accrued(lastTradingDay)
		if err != nil {
			log.Fatalf("%s: row %d: %v", filePath, nr+1, err)
		}
		bonds = append(bonds, bnd)
		flows = append(flows, cfs)
		accrued = append(accrued, acc)
		quotes = append(quotes, price)
		// reminder: prices are "dirty"
		dirty := price + acc
		prices = append(prices, dirty)
		yield, err := yieldPercent(dirty, cfs)
		if err != nil {
			log.Println(err)
			yield = math.NaN()
		}
		yields = append(yields, yield)

	}

	// the term structure cannot be fitted with less bonds than parameters
	if len(bonds) < minBonds {
		fmt.Fprintf(os.Stderr, "too few bonds to fit the term structure: got %d (not matured at %s), need at least %d\n",
			len(bonds), lastTradingDay.Format(DateFmt), minBonds)
		os.Exit(1)
	}

	// *******************************************************************
	// optimized NSS
	// *******************************************************************
	fun := func(xf []float64) float64 {

		ts := nssFromParams(xf)

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

		for i := range bonds {
			// minimize least-squares of yields
			if math.IsNaN(yields[i]) {
				continue
			}

			value := cashflow.PV(flows[i], ts)
			est, err := yieldPercent(value, flows[i])
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

	termStart := nssFromParams(x)

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
	termNss := nssFromParams(result.X)
	meta := term.Meta{AsOf: lastTradingDay}

	fmt.Println("Nelson-Siegel-Svensson term structure:")
	printTerm(termNss, meta)
	if err := printTermToFile(termNss, meta, "nss_opt.json"); err != nil {
		log.Println(err)
	}

	// *******************************************************************
	// optimized Spline
	// *******************************************************************

	// collect all maturities
	temp := []float64{}
	xtmap := make(map[float64]bool)
	for _, cfs := range flows {
		for _, cf := range cfs {
			xtmap[cf.T] = true
		}
	}

	// sort maturities
	delete(xtmap, 0.0)
	for key, _ := range xtmap {
		temp = append(temp, key)
	}
	sort.Float64s(temp)

	xt := splineKnots(temp, len(bonds))

	// define optimization function for the (cubic) splines
	funSpline := func(y []float64) float64 {
		sst := 0.0
		penalty := 1.0e3
		ts, err := newSpline(xt, y)
		if err != nil {
			// invalid trial point (e.g. non-positive discount factors)
			return penalty * float64(len(bonds)+1)
		}
		for i := range bonds {
			// minimize least-squares of yields
			if math.IsNaN(yields[i]) {
				continue
			}

			value := cashflow.PV(flows[i], ts)
			est, err := yieldPercent(value, flows[i])
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
		y[i] = termNss.Discount(x)
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
	termSpline, err := newSpline(xt, result.X)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Cubic spline term structure")
	fmt.Println("Maturities      :", xt)
	fmt.Println("Discount Factors:", result.X)
	if err := printTermToFile(termSpline, meta, "spline_opt.json"); err != nil {
		log.Println(err)
	}

	// *******************************************************************
	// print output
	// *******************************************************************
	output := [][]string{}
	for i := range bonds {
		quoteNss := cashflow.PV(flows[i], termNss) - accrued[i]
		quoteSpline := cashflow.PV(flows[i], termSpline) - accrued[i]
		t := flows[i][len(flows[i])-1].T
		output = append(output, []string{
			fmt.Sprintf("%v", t),
			fmt.Sprintf("%v", quotes[i]),
			fmt.Sprintf("%v", termStart.ZeroRate(t)*100.0),
			fmt.Sprintf("%v", termNss.ZeroRate(t)*100.0),
			fmt.Sprintf("%v", termSpline.ZeroRate(t)*100.0),
			fmt.Sprintf("%v", quoteNss),
			fmt.Sprintf("%v", quoteSpline),
			fmt.Sprintf("%v", termNss.Discount(t)),
			fmt.Sprintf("%v", termSpline.Discount(t)),
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

// minBonds is the minimum number of bonds for the fit (number of parameters
// of the Nelson-Siegel-Svensson model)
const minBonds = 6

// isHeader returns true if the CSV row is a header row, i.e. if the
// maturity is not a date and the coupon and price are not numeric
func isHeader(row []string) bool {
	if len(row) < 3 {
		return false
	}
	if _, err := parseDate(row[0]); err == nil {
		return false
	}
	for _, field := range row[1:3] {
		if _, err := strconv.ParseFloat(strings.TrimSpace(field), 64); err == nil {
			return false
		}
	}
	return true
}

// splineKnots returns the maturities used as knots for the cubic spline,
// given the sorted maturities of all cash flows (strictly increasing and
// positive, without duplicates)
func splineKnots(maturities []float64, nbonds int) []float64 {
	if len(maturities) == 0 {
		return nil
	}
	n := 0
	if nbonds > 0 {
		n = len(maturities) / int(math.Sqrt(float64(nbonds)))
	}
	xt := append([]float64{maturities[0]}, determine_weights(maturities, n)...)
	xt = append(xt, maturities[len(maturities)-1])

	// remove unset weights (zero) and duplicates
	sort.Float64s(xt)
	knots := []float64{}
	for _, t := range xt {
		if t > 0.0 && (len(knots) == 0 || t > knots[len(knots)-1]) {
			knots = append(knots, t)
		}
	}
	return knots
}

// newSpline returns a cubic spline term structure through the discount
// factors at the knots; it returns an error for invalid input (e.g.
// non-positive discount factors proposed by the optimizer)
func newSpline(maturities, factors []float64) (*term.Spline, error) {
	return term.NewSpline(maturities, factors)
}

// parseDate parses a date in one of the accepted date formats
func parseDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range dateFormats {
		if d, err := time.Parse(layout, value); err == nil {
			return d, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date %q (formats: %s)", value, strings.Join(dateFormats, ", "))
}

func printResult(result *optimize.Result) {
	fmt.Println("Optimization results:")
	fmt.Printf("Status: %v\n", result.Status)
	fmt.Printf("X     : %0.4g\n", result.X)
	fmt.Printf("F     : %0.4g\n", result.F)
	fmt.Printf("Stats.FuncEvaluations: %d\n\n", result.Stats.FuncEvaluations)
}

func printTerm(ts term.Curve, meta term.Meta) {
	text, err := term.Marshal(ts, meta)
	if err != nil {
		log.Println(err)
		return
	}
	fmt.Println(string(text))
}

func printTermToFile(ts term.Curve, meta term.Meta, name string) error {
	data, err := term.Marshal(ts, meta)
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
