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

	"github.com/konimarti/bonds/pkg/term"
)

var (
	fileFlag      = flag.String("f", "term.json", "json file containing the parameters for the Nelson-Siegel-Svensson term structure")
	nInput        = flag.Int("n", 1000, "number of time steps")
	maturityInput = flag.Float64("m", 10.0, "maturity in years for simulation")
	sigmaInput    = flag.Float64("s", 0.00, "standard deviation of interest rates")
)

func main() {
	flag.Parse()

	// read term structure
	nssData, err := ioutil.ReadFile(*fileFlag)
	if err != nil {
		log.Println(err)
	}

	var ts term.NelsonSiegelSvensson
	err = json.Unmarshal(nssData, &ts)
	if err != nil {
		panic(err)
	}

	// define parameters
	n := *nInput
	T := *maturityInput
	sigma := *sigmaInput
	dt := T / float64(n)

	// define numerical grid
	r := make([]float64, n+2)
	f := make([]float64, n+1)
	theta := make([]float64, n)
	// rHoLee := make([]float64, n)

	// calculate current rates, forward rates and thetas on the grid
	for i := 0; i < n+2; i += 1 {
		r[i] = ts.Rate(float64(i+1)*dt) / 100.0
	}
	for i := 0; i < n+1; i += 1 {
		// f[i] = r[i] + float64(i+1)*(r[i+1]-r[i])
		f[i] = -math.Log(ts.Z(float64(i+2)*dt)/ts.Z(float64(i+1)*dt)) / dt
	}
	for i := 0; i < n; i += 1 {
		theta[i] = (f[i+1]-f[i])/dt + sigma*sigma*float64(i+1)*dt
	}

	// print parameters for info
	fmt.Println("Parameters")
	fmt.Println("T    :", T)
	fmt.Println("n    :", n)
	fmt.Println("dt   :", dt)
	fmt.Println("Sigma:", sigma)
	// print out current rates vs. Ho-Lee rates
	fmt.Println("Time\tZ_term\tZ_HoLee")

	// verify Ho-Lee model by calculating zero bonds
	// Z(r,0;T) = exp(A(0;T) - T * r)
	// where A(0;T) = - Int^T_0 (T-t) theta_t dt + T^3/6 * sigma^2
	// which is approximated by A(0;T) = Sum^n_{j=1} theta_{j*dt} * (T-j*dt) * dt + T^3 / 6 * sigma^2

	for t := 0.5; t <= T; t += 0.5 {
		A := math.Pow(t, 3.0) / 6.0 * math.Pow(sigma, 2.0)
		r := r[0]
		// calculate A for given maturity
		for j := 0; j < int(t/dt); j += 1 {
			A -= theta[j] * (t - float64(j+1)*dt) * dt
			r += theta[j] * dt
		}
		//price of zero bond Z(r, 0, t)
		z := math.Exp(A - t*r)
		fmt.Printf("%6.2f\t%8.6f\t%8.6f\t%8.6f\t%8.6f\n", t, ts.Z(t)*100.0, z*100.0, ts.Rate(t), 100.0*r)
	}

	// print out parameters for analysis in R
	fout, err := os.Create("result.csv")
	if err != nil {
		log.Fatal("Unable to read input file: result.csv", err)
	}
	defer fout.Close()
	w := csv.NewWriter(fout)
	output := [][]string{}
	rhl := r[0]
	for i := 0; i < n; i += 1 {
		rhl += theta[i] * dt
		output = append(output, []string{
			fmt.Sprintf("%v", float64(i+1)*dt),
			fmt.Sprintf("%v", r[i]*100.0),
			fmt.Sprintf("%v", f[i]*100.0),
			fmt.Sprintf("%v", theta[i]),
			fmt.Sprintf("%v", rhl*100.0),
		})
	}
	w.WriteAll(output) // calls Flush internally
}
