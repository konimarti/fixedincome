package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"os"
	"time"

	"github.com/konimarti/bonds/pkg/term"
)

var (
	fileFlag      = flag.String("f", "term.json", "json file containing the parameters for the Nelson-Siegel-Svensson term structure")
	nInput        = flag.Int("n", 1000, "number of time steps")
	maturityInput = flag.Float64("m", 10.0, "maturity in years for simulation")
	sigmaInput    = flag.Float64("s", 0.02, "standard deviation of interest rates")
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

	// print parameters for info
	fmt.Println("Parameters")
	fmt.Println("T    :", T)
	fmt.Println("n    :", n)
	fmt.Println("dt   :", dt)
	fmt.Println("Sigma:", sigma)

	// define numerical grid
	r := make([]float64, n+2)
	f := make([]float64, n+1)
	theta := make([]float64, n)
	z := make([]float64, n)
	rHoLee := make([]float64, n)
	zHoLee := make([]float64, n)

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

	// calculate Ho-Lee model spot curve
	// Z(r,0;T) = exp(A(0;T) - T * r)
	// where A(0;T) = - Int^T_0 (T-t) theta_t dt + T^3/6 * sigma^2
	// which is approximated by A(0;T) = Sum^n_{j=1} theta_{j*dt} * (T-j*dt) * dt + T^3 / 6 * sigma^2
	for i := 0; i < n; i += 1 {
		t := float64(i+1) * dt
		// last term of A
		A := math.Pow(t, 3.0) / 6.0 * math.Pow(sigma, 2.0)
		// calculate integral part of A
		for j := 0; j < i; j += 1 {
			A -= theta[j] * (t - float64(j+1)*dt) * dt
		}
		// calculate r_HoLee = -ln(Z_HoLee)/t
		rHoLee[i] = -A/t + r[0]
		// calulate zero bonds: Z_HoLee(t) = exp(A - r_0 * t)
		zHoLee[i] = math.Exp(-rHoLee[i] * t)
		z[i] = ts.Z(t)
	}

	// print out current rates vs. Ho-Lee rates
	fmt.Println("Time\tZ_term\tZ_HoLee")
	for t := 0.5; t <= T; t += 0.5 {
		i := int((t / dt)) - 1
		z := math.Exp(-rHoLee[i] * t)
		fmt.Printf("%6.2f\t%8.6f\t%8.6f\t%8.6f\t%8.6f\n", t, ts.Z(t)*100.0, z*100.0, ts.Rate(t), 100.0*rHoLee[i])
	}

	// print out parameters for analysis in R
	fout, err := os.Create("result.csv")
	if err != nil {
		log.Fatal("Unable to read input file: result.csv", err)
	}
	defer fout.Close()
	w := csv.NewWriter(fout)
	output := [][]string{}
	for i := 0; i < n; i += 1 {
		output = append(output, []string{
			fmt.Sprintf("%v", float64(i+1)*dt),
			fmt.Sprintf("%v", r[i]*100.0),
			fmt.Sprintf("%v", f[i]*100.0),
			fmt.Sprintf("%v", theta[i]),
			fmt.Sprintf("%v", rHoLee[i]*100.0),
			fmt.Sprintf("%v", z[i]*100.0),
			fmt.Sprintf("%v", zHoLee[i]*100.0),
		})
	}
	w.WriteAll(output) // calls Flush internally

	// Monte Carlo pricing with the continuous-time Ho-Lee interest rate model
	// Pricing of Zero Bond with maturity t=4.0
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	t := 10.0
	n = int(t / dt)
	nsim := 10000
	payoff := 100.0
	mc := make([]float64, nsim)
	rates := make([]float64, n)
	for j := 0; j < nsim; j += 1 {
		// simulate interest rates
		rates[0] = r[0]
		for i := 0; i < (n - 1); i += 1 {
			rates[i+1] = rates[i] + theta[i]*dt + sigma*math.Sqrt(dt)*rng.NormFloat64()
		}
		// integrate rates: int^N-1_{i=0}
		rate := 0.0
		for i := 0; i < (n - 1); i += 1 {
			rate += rates[i] * dt
		}

		// calculate monte carlo expectation value
		mc[j] = math.Exp(-rate) * payoff
	}

	estimate := 0.0
	for j := 0; j < nsim; j += 1 {
		estimate += mc[j]
	}
	estimate = math.Round(1e4*estimate/float64(nsim)) / 1e4

	stderror := 0.0
	for j := 0; j < nsim; j += 1 {
		stderror += math.Pow(mc[j]-estimate, 2.0)
	}
	stderror = math.Round(1e4*math.Sqrt(stderror/float64(nsim))/math.Sqrt(float64(nsim))) / 1e4

	fmt.Println("Monte Carlo Z(", t, ")=", estimate)
	fmt.Println("[", math.Round(1e4*(estimate-1.96*stderror))/1e4, ",", math.Round(1e4*(estimate+1.96*stderror))/1e4, "]")
	fmt.Println("Reference   Z(", t, ")=", ts.Z(t)*100.0)

}
