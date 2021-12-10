package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
	"time"

	"github.com/konimarti/daycount"
	"github.com/konimarti/fixedincome"
	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/term"
)

var (
	settlementFlag = flag.String("settlement", time.Now().Format("2006-01-02"), "valuation date / settlement date")
	maturityFlag   = flag.String("maturity", time.Now().AddDate(1, 0, 0).Format("2006-01-02"), "maturity date of bond")
	coupon         = flag.Float64("coupon", 0.0, "coupon in percent of par value")
	frequency      = flag.Int("n", 1, "compounding frequency per year")
	price          = flag.Float64("quote", 0.0, "quoted bond price at settlement date")
	redemption     = flag.Float64("redemption", 100.0, "redemption value of bond at maturity")
	spread         = flag.Float64("spread", 0.0, "Static (zero-volatility) spread in basepoints for valuing risky bonds")
	fileFlag       = flag.String("f", "term.json", "json file containing the parameters for term structure")
	option         = strings.Join([]string{"day count convention for accured interest, available: ", strings.Join(daycount.Implemented(), ", ")}, "")
	daycountname   = flag.String("daycount", "30E360", option)
)

func main() {
	flag.Parse()

	// read term structure parameters and create NSS model
	termData, err := ioutil.ReadFile(*fileFlag)
	if err != nil {
		log.Println(err)
	}

	ts, err := term.Parse(termData)
	if err != nil {
		log.Println(err)
		log.Println("no file given for term structure parameters. Use template for e.g. Nelson-Siegel-Svensson:")
		data, err := json.MarshalIndent(term.NelsonSiegelSvensson{}, " ", "")
		if err != nil {
			panic(err)
		}
		fmt.Println(string(data))
		return
	}

	// parse quote and maturity dates
	quoteDate, err := time.Parse("2006-01-02", *settlementFlag)
	if err != nil {
		log.Fatal(err)
	}
	maturityDate, err := time.Parse("2006-01-02", *maturityFlag)
	if err != nil {
		log.Fatal(err)
	}

	// create fixed-coupon bond
	bond := bond.Straight{
		Schedule: maturity.Schedule{
			Settlement: quoteDate,
			Maturity:   maturityDate,
			Frequency:  *frequency,
			Basis:      *daycountname,
		},
		Coupon:     *coupon,
		Redemption: *redemption,
	}

	// set spread
	ts.SetSpread(*spread)

	// price the bond
	dirty := bond.PresentValue(ts)
	clean := dirty - bond.Accrued()

	fmt.Println("")
	fmt.Printf("Settlement Date  : %s\n", quoteDate.Format("2006-01-02"))
	fmt.Printf("Maturity Date    : %s\n", maturityDate.Format("2006-01-02"))
	fmt.Println("")
	fmt.Printf("Years to Maturity: %.2f years\n", bond.YearsToMaturity())
	fmt.Printf("Modified duration: %.2f\n", bond.Duration(ts))
	fmt.Println("")
	fmt.Printf("Coupon           : %.2f\n", *coupon)
	fmt.Printf("Frequency        : %d\n", *frequency)
	fmt.Printf("Day Convention   : %s\n", *daycountname)
	fmt.Println("")
	fmt.Printf("Spread           : %.2f\n", *spread)

	fmt.Println("")
	fmt.Printf("    Dirty Price       %10.2f\n", dirty)
	fmt.Printf("[-] Accrued Interest  %10.2f\n", bond.Accrued())
	fmt.Println("--------------------------------")
	fmt.Printf("[=] Clean Price       %10.2f\n", clean)
	fmt.Println("================================")
	fmt.Println("")

	bondPrice := clean
	if *price > 0.0 {
		bondPrice = *price
		fmt.Println("Yields for the quoted price:")
	} else {
		fmt.Println("Yields for the calculated clean price:")
	}

	fmt.Printf("  Quoted Price        %10.2f\n", bondPrice)
	fmt.Printf("  Invoice Price       %10.2f\n", bondPrice+bond.Accrued())
	irr, err := fixedincome.Irr(bondPrice+bond.Accrued(), &bond)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  Yield-to-Maturity   %10.2f %%\n", irr)

	spread, err := fixedincome.Spread(bondPrice+bond.Accrued(), &bond, ts)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  Implied spread      %10.1f bps\n", spread)

}
