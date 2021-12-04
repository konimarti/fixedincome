package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/konimarti/bonds/pkg/instrument/option"
	"github.com/konimarti/bonds/pkg/term"
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
	optionType := option.Call
	if strings.ToUpper(*optionT) == "PUT" {
		optionType = option.Put
	}

	// create European option
	euOption := option.European{
		optionType, *stockPrice, *strikePrice, *maturity, *divyield, *vola,
	}

	// create flat yield curve
	term := term.Flat{*discountRate, 0.0}

	// print option price and the 'Greeks'
	if euOption.Type == option.Call {
		fmt.Println("Call Option")
	} else {
		fmt.Println("Put Option")
	}
	fmt.Printf("Price: %9.4f\n", euOption.PresentValue(&term))
	fmt.Println("\nThe 'Greeks'")
	fmt.Printf("Delta: %9.4f\n", euOption.Delta(&term))
	fmt.Printf("Gamma: %9.4f\n", euOption.Gamma(&term))
	fmt.Printf("Rho  : %9.4f\n", euOption.Rho(&term))
	fmt.Printf("Vega : %9.4f\n", euOption.Vega(&term))

}
