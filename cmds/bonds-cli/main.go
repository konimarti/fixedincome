package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"time"

	"github.com/konimarti/bonds"
)

var (
	settlementFlag = flag.String("settlement", time.Now().Format("2006-01-02"), "valuation date / settlement date")
	maturityFlag   = flag.String("maturity", time.Now().AddDate(1, 0, 0).Format("2006-01-02"), "maturity date of bond")
	coupon         = flag.Float64("coupon", 0.0, "coupon in percent of par value (default: 0.0%)")
	frequency      = flag.Int("n", 1, "compounding frequency per year (default: 1x per year)")
	price          = flag.Float64("quote", 0.0, "quote of bond (optional but required for static spread or internal rate of return calculation)")
	redemption     = flag.Float64("redemption", 100.0, "redemption value of bond at maturity (default: 100)")
	spread         = flag.Float64("spread", 0.0, "Static (zero-volatility) spread in basepoints for valuing risky bonds (default: 0.0 bps)")
	fileFlag       = flag.String("f", "term.json", "json file containing the parameters for the Nelson-Siegel-Svensson term structure")
)

func main() {
	flag.Parse()

	// read term structure parameters and create NSS model
	nssData, err := ioutil.ReadFile(*fileFlag)
	if err != nil {
		log.Println(err)
	}

	var nss bonds.NelsonSiegelSvensson
	err = json.Unmarshal(nssData, &nss)
	if err != nil {
		log.Println(err)
		log.Println("no file given for term structure parameters. Use template for Nelson-Siegel-Svensson:")
		data, err := json.MarshalIndent(bonds.NelsonSiegelSvensson{}, " ", "")
		if err != nil {
			panic(err)
		}
		fmt.Println(string(data))
		return

	}

	// parse quote and maturity dates
	quote, err := time.Parse("2006-01-02", *settlementFlag)
	if err != nil {
		log.Fatal(err)
	}
	maturity, err := time.Parse("2006-01-02", *maturityFlag)
	if err != nil {
		log.Fatal(err)
	}

	// create fixed-coupon bond
	bond := bonds.Bond{
		Schedule: bonds.Maturities{
			Settlement: quote,
			Maturity:   maturity,
			Frequency:  *frequency,
		},
		Coupon:     *coupon,
		Redemption: *redemption,
	}

	// price the bond
	dirty, clean := bond.Pricing(*spread, &nss)

	fmt.Println("")
	fmt.Printf("Settlement Date  : %s\n", quote.Format("2006-01-02"))
	fmt.Printf("Maturity Date    : %s\n", maturity.Format("2006-01-02"))
	fmt.Println("")
	fmt.Printf("Years to Maturity: %.2f years\n", bond.YearsToMaturity())
	fmt.Printf("Modified duration: %.2f\n", bond.Duration(*spread, &nss))
	fmt.Println("")
	fmt.Printf("Coupon           : %.2f\n", *coupon)
	fmt.Printf("Frequency        : %d\n", *frequency)
	fmt.Printf("Day Convention   : 30/360\n")
	fmt.Printf("Maturities       : Act/365 fixed\n")
	fmt.Println("")
	fmt.Printf("Spread           : %.2f\n", *spread)

	fmt.Println("")
	fmt.Printf("    Dirty Price       %10.2f\n", dirty)
	fmt.Printf("[-] Accrued Interest  %10.2f\n", bond.Accrued())
	fmt.Println("--------------------------------")
	fmt.Printf("[=] Clean Price       %10.2f\n", clean)
	fmt.Println("================================")
	fmt.Println("")

	if *price > 0.0 {
		fmt.Println("Yields for the quoted price:")
		fmt.Printf("  Quote               %10.2f\n", *price)
		irr, err := bonds.IRR(*price, bond)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("  Yield-to-Maturity   %10.2f %%\n", irr)

		spread, err := bonds.Spread(*price, bond, &nss)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("  Implied spread      %10.1f bps\n", spread)
	}

}
