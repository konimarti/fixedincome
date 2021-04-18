# Valuation of Fixed Income Securities

[![License](http://img.shields.io/badge/license-MIT-red.svg?style=flat)](https://github.com/konimarti/bonds/blob/master/LICENSE)
[![GoDoc](https://godoc.org/github.com/konimarti/observer?status.svg)](https://godoc.org/github.com/konimarti/bonds)
[![goreportcard](https://goreportcard.com/badge/github.com/konimarti/observer)](https://goreportcard.com/report/github.com/konimarti/bonds)

Valuation of fixed income securities with a spot-rate term structure based on the Nelson-Siegel-Svensson method and static (zero-volatility) spreads.

```go get github.com/konimarti/bonds```


## Usage of bonds-cli 

* ```bonds-cli``` is a command-line tool to value fixed income securities

  - Install the app: ```go install github.com/konimarti/bonds/cmds/bonds-cli```

  - Create a file with the parameters of the Nelson-Siegel-Svensson model in a JSON file, e.g. term.json: 
    ```
	{
	 "b0": -0.266372,
	 "b1": -0.471343,
	 "b2": 5.68789,            
	 "b3": -5.12324,           
	 "t1": 5.74881,            
	 "t2": 4.14426             
	 }
    ```

  - Run the application and provide the details of the bond to be valued:
    ```
    $ bonds-cli -coupon 1.25 -maturity 2026-05-28 -price 109.64 -freq 2
    ```

  - This produces the following output:
    ```
    Settlement Date  : 2021-04-18
    Maturity Date    : 2026-05-28

    Remaining years  : 5.11 years

    Coupon           : 1.25
    Frequency        : 2
    Day Convention   : 30/360
    Maturities       : Act/365 fixed

        Dirty Price           110.11
    [-] Accrued Interest        0.49
    --------------------------------
    [=] Clean Price           109.62
    ================================

    Yields for the quoted price:
      Price                   109.64
      Yield-to-Maturity        -0.60
      Z-Spread (bps)            -0.3
    ```

* The following options are implemented in ```bonds-cli```:
```
Usage of bonds-cli:
  -coupon float
    	coupon in percent of par value (default: 0.0%)
  -f string
    	json file containing the parameters for the Nelson-Siegel-Svensson term stucture (default "term.json")
  -freq int
    	number of coupon payments per year (default: 1x per year) (default 1)
  -maturity string
    	maturity date of bond (default "2022-04-18")
  -price float
    	quote of bond at valuation date (optional but required for z-spread or IRR calculation)
  -settlement string
    	valuation date / settlement date (default "2021-04-18")
  -spread float
    	Static (zero-volatility) spread in basepoints for valuing risky bonds (default: 0.0 bps)
```

## Nelson-Siegel-Svensson parameters 

Many central banks offer daily updates of the fitted parameters for the Nelson-Siegel-Svensson model:

* Swiss National Bank (SNB) for [CHF risk-free spot rates](https://data.snb.ch/en/topics/ziredev#!/cube/rendopar)

* European Central Bank (ECB) for [EUR risk-free spot rates](https://www.ecb.europa.eu/stats/financial_markets_and_interest_rates/euro_area_yield_curves/html/index.en.html)


## Code
```go
	// define bond
	bond := bonds.Bond{
		Schedule: bonds.Maturities{
			Settlement: time.Date(2021,4,17,0,0,0,0,time.UTC),
			Maturity:   time.Date(2026,5,25,0,0,0,0,time.UTC),
			Frequency:  1,
		},
		Coupon:     1.25,
		Redemption: 100.0,
	}

	// define term structure 
        // Nelson-Siegel-Svensson parameters as 2021-03-31 for Swiss government bonds
	term := bonds.NelsonSiegelSvensson{
		-0.266372,
		-0.471343,
		5.68789,
		-5.12324,
		5.74881,
		4.14426,
	}
```

```go
	// price risk-free bond with a spread of 0.0
	dirty, clean := bond.Pricing(0.0, &term)

	// calculate accrued interest (30/360 day convention)
	accrued := bond.Accrued()

	// estimate yield to maturity (IRR) given a market price
	irr, _ := bonds.IRR(109.70, bond)

	// estimate implied static (zero-volatility) spread (useful for risky bonds)
	spread, _ := bond.Spread(109.70, bond, &term)
```

## Further reading

* [Nelson-Siegel-Svensson model at SNB](https://www.snb.ch/de/mmr/reference/quartbul_2002_2_komplett/source/quartbul_2002_2_komplett.de.pdf) on page 64

