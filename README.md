# Fixed Income Valuation in Go

[![License](http://img.shields.io/badge/license-MIT-red.svg?style=flat)](https://github.com/konimarti/bonds/blob/master/LICENSE)
[![GoDoc](https://godoc.org/github.com/konimarti/observer?status.svg)](https://godoc.org/github.com/konimarti/bonds)
[![goreportcard](https://goreportcard.com/badge/github.com/konimarti/observer)](https://goreportcard.com/report/github.com/konimarti/bonds)

Valuation of fixed income securities with a spot-rate term structure based on the Nelson-Siegel-Svensson method and static (zero-volatility) spreads.

```go get github.com/konimarti/bonds```

## Usage
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
	// price risk-free bond with spead of 0.0
	dirty, clean := bond.Pricing(0.0, &term)

	// calculate accrued interest (30/360 day convention)
	accrued := bond.Accrued()

	// estimate yield to maturity (IRR) given a market price
	irr, _ := bonds.IRR(109.70, bond)

	// estimate implied static (zero-volatility) spread (useful for risky bonds)
	spread, _ := bond.Spread(109.70, bond, &term)
```

### Nelson-Siegel-Svensson parameters 

Many central banks offer daily updates of the fitted parameters for the Nelson-Siegel-Svensson model of the government bonds:

* Swiss Natinonal Bank (SNB) for CHF [link](https://data.snb.ch/en/topics/ziredev#!/cube/rendopar)

* European Central Bank (ECB) for EUR [link](https://www.ecb.europa.eu/stats/financial_markets_and_interest_rates/euro_area_yield_curves/html/index.en.html)

## Applications 

### bonds-cli

* ```bonds-cli``` is a command-line tool to value bonds

  - Install the app: ```go install github.com/konimarti/bonds/cmds/bonds-cli```

  - Run cli: 
    ```
    $ bonds-cli -coupon 1.25 -maturity 2026-05-28 -price 109.64 -freq 2
    ```

  - Output:
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

## Futher reading

* [Nelson-Siegel-Svensson model at SNB](https://www.snb.ch/de/mmr/reference/quartbul_2002_2_komplett/source/quartbul_2002_2_komplett.de.pdf) on page 64

## Credits

This software package has been developed for and is in production at [Caliza Holding](http://www.caliza.ch/en).

