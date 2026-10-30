package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/konimarti/fixedincome/pkg/instrument/option"
	"github.com/konimarti/fixedincome/pkg/term"
)

var (
	optionT      = flag.String("type", "call", "type is either 'call' or 'put' (default: 'call')")
	stockPrice   = flag.Float64("s", 110.0, "current stock price")
	strikePrice  = flag.Float64("k", 100.0, "strike price")
	maturity     = flag.Float64("t", 2.0, "time to expiration date in years")
	divyield     = flag.Float64("q", 0.0, "dividend yield in percent (default: 0.0)")
	vola         = flag.Float64("vola", 0.3, "volatility (default: 0.3)")
	discountRate = flag.Float64("r", 2.0, "continuously compoundd discount rate in percent assuming flat yield curve (default: 2.0)")
)

func main() {
	flag.Parse()

	// parse option type
	optionType, err := option.ParseType(*optionT)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	// create European option (dividend yield flag in percent)
	euOption := option.European{
		Type: optionType,
		S:    *stockPrice,
		K:    *strikePrice,
		T:    *maturity,
		Q:    *divyield / 100.0,
		Vol:  *vola,
	}

	// create flat yield curve (rate flag in percent)
	term := term.Flat{Rate: *discountRate / 100.0}

	price, err := euOption.Price(term)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	greeks, err := euOption.Greeks(term)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	// print option price and the 'Greeks'
	if euOption.Type == option.Call {
		fmt.Println("Call Option")
	} else {
		fmt.Println("Put Option")
	}
	fmt.Printf("Price: %9.4f\n", price)
	fmt.Println("\nThe 'Greeks'")
	fmt.Printf("Delta: %9.4f\n", greeks.Delta)
	fmt.Printf("Gamma: %9.4f\n", greeks.Gamma)
	fmt.Printf("Rho  : %9.4f\n", greeks.Rho)
	fmt.Printf("Vega : %9.4f\n", greeks.Vega)

}
