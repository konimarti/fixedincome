package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/konimarti/fixedincome/pkg/cashflow"
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
	option         = "day count convention for accured interest, available: " + strings.Join(maturity.DayCountNames(), ", ")
	daycountname   = flag.String("daycount", "30E360", option)
)

func main() {
	flag.Parse()

	// read term structure parameters and create NSS model
	termData, err := ioutil.ReadFile(*fileFlag)
	if err != nil {
		fatalTerm(fmt.Errorf("reading term structure failed: %w", err))
	}

	ts, _, err := term.Parse(termData)
	if err != nil {
		fatalTerm(fmt.Errorf("parsing term structure from %s failed: %w", *fileFlag, err))
	}
	fmt.Fprintln(os.Stderr, "Term model read from", *fileFlag)

	// parse quote and maturity dates
	quoteDate, err := time.Parse("2006-01-02", *settlementFlag)
	if err != nil {
		log.Fatal(err)
	}
	maturityDate, err := time.Parse("2006-01-02", *maturityFlag)
	if err != nil {
		log.Fatal(err)
	}
	if *frequency <= 0 {
		// legacy behaviour: frequency <= 0 means annual
		*frequency = 1
	}
	if !quoteDate.Before(maturityDate) {
		log.Fatalf("bond has matured: settlement date %s is not before maturity date %s",
			quoteDate.Format("2006-01-02"), maturityDate.Format("2006-01-02"))
	}

	// create fixed-coupon bond (flags in percent are converted to decimals)
	dayCount, err := maturity.ParseDayCount(*daycountname)
	if err != nil {
		log.Fatal(err)
	}
	bond, err := bond.NewStraight(maturity.Schedule{
		Maturity:  maturityDate,
		Frequency: maturity.Frequency(*frequency),
		DayCount:  dayCount,
	}, *coupon/100.0, bond.WithRedemption(*redemption))
	if err != nil {
		log.Fatal(err)
	}
	cfs, err := bond.CashFlows(quoteDate)
	if err != nil {
		log.Fatal(err)
	}
	accrued, err := bond.Accrued(quoteDate)
	if err != nil {
		log.Fatal(err)
	}

	// set the spread (flag in bp); it replaces the spread of the curve file
	base, _ := term.Spread(ts)
	ts = term.WithSpread(base, *spread/1e4)

	// price the bond
	dirty := cashflow.PV(cfs, ts)
	clean := dirty - accrued

	fmt.Println("")
	fmt.Printf("Settlement Date  : %s\n", quoteDate.Format("2006-01-02"))
	fmt.Printf("Maturity Date    : %s\n", maturityDate.Format("2006-01-02"))
	fmt.Println("")
	fmt.Printf("Years to Maturity: %.4f years\n", maturity.YearFraction(quoteDate, maturityDate))
	fmt.Printf("Modified duration: %.4f\n", cashflow.Duration(cfs, ts))
	fmt.Println("")
	fmt.Printf("Coupon           : %.2f\n", *coupon)
	fmt.Printf("Frequency        : %d\n", *frequency)
	fmt.Printf("Day Convention   : %s\n", *daycountname)
	if days, err := dayCount.Days(quoteDate, maturityDate); err == nil {
		fmt.Printf("Days             : %d\n", int(days))
	}
	fmt.Println("")
	fmt.Printf("Spread           : %.2f\n", *spread)

	fmt.Println("")
	fmt.Printf("    Dirty Price       %10.4f\n", dirty)
	fmt.Printf("[-] Accrued Interest  %10.4f\n", accrued)
	fmt.Println("----------------------------------")
	fmt.Printf("[=] Clean Price       %10.4f\n", clean)
	fmt.Println("==================================")
	fmt.Println("")

	bondPrice := clean
	if *price > 0.0 {
		bondPrice = *price
		fmt.Println("Yields for the quoted price:")
	} else {
		fmt.Println("Yields for the calculated clean price:")
	}

	fmt.Printf("  Quoted Price        %10.4f\n", bondPrice)
	fmt.Printf("  Invoice Price       %10.4f\n", bondPrice+accrued)
	irr, err := cashflow.YieldCC(bondPrice+accrued, cfs)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  Yield-to-Maturity   %10.4f %%\n", irr*100.0)

	// implied spread over the base curve (without the spread flag)
	spread, err := cashflow.ZSpread(bondPrice+accrued, cfs, base)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  Implied spread      %10.1f bps\n", spread*1e4)

}

// fatalTerm prints the error and a template for the term structure file to
// stderr and exits with a non-zero exit code
func fatalTerm(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	fmt.Fprintln(os.Stderr, "Use the following template for e.g. the Nelson-Siegel-Svensson term structure (betas in percent):")
	fmt.Fprintln(os.Stderr, nssTemplate)
	os.Exit(1)
}

// nssTemplate is a template of a curve file with a Nelson-Siegel-Svensson
// term structure
const nssTemplate = `{
  "model": "nss",
  "asof": "2021-03-31",
  "currency": "CHF",
  "params": {"b0": -0.266372, "b1": -0.471343, "b2": 5.68789, "b3": -5.12324, "t1": 5.74881, "t2": 4.14426},
  "spread_bp": 0
}`
