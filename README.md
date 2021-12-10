# Valuation of Fixed Income Securities

[![License](http://img.shields.io/badge/license-MIT-red.svg?style=flat)](https://github.com/konimarti/fixedincome/blob/master/LICENSE)
[![GoDoc](https://godoc.org/github.com/konimarti/observer?status.svg)](https://godoc.org/github.com/konimarti/fixedincome)
[![goreportcard](https://goreportcard.com/badge/github.com/konimarti/observer)](https://goreportcard.com/report/github.com/konimarti/fixedincome)

Valuation of fixed income securities with a spot-rate term structure or continuous-time interest-rate models.
This package can handle and optimize Nelson-Siegel-Svensson or cubic splines term structures from a list of bonds.
Monte Carlo simulations can be used to price exotic securities with an interest rate model. Currently, the Ho-Lee and Vasicek models are implemented.

Financial instruments covered:

- Fixed-coupon and floating rate bonds
- Foward contracts and forward rate agreeements
- Interest rate swaps
- European options (with Black-Scholes)
- European, Asian, American options with Monte Carlo
- Ho-Lee and Vasicek interest rate models

`go get github.com/konimarti/fixedincome`

## Apps

- `termfit` fits a spot-rate curve to a set of bonds given their quoted prices and maturity dates.
- `bonds-cli` can be used to value a simple straight fixed-coupon bond
- `swaprate-cli` provides the swap rates for a set of maturities for the given spot-rate curve
- `option-cli` is pricing plain vanilla European call or put options and calculates all the 'Greeks'

## Nelson-Siegel-Svensson parameters

Many central banks offer daily updates of the fitted parameters for the Nelson-Siegel-Svensson model:

- Swiss National Bank (SNB) for [CHF risk-free spot rates](https://data.snb.ch/en/topics/ziredev#!/cube/rendopar)

- European Central Bank (ECB) for [EUR risk-free spot rates](https://www.ecb.europa.eu/stats/financial_markets_and_interest_rates/euro_area_yield_curves/html/index.en.html)

## Code example for a straight bond

- Valuation of more exoctic securities are given in the example folder

```go
	// define straight bond
	straightBond := bond.Straight{
		Schedule: maturity.Schedule{
			Settlement: time.Date(2021,4,17,0,0,0,0,time.UTC),
			Maturity:   time.Date(2026,5,25,0,0,0,0,time.UTC),
			Frequency:  1,
		},
		Coupon:     1.25,
		Redemption: 100.0,
	}

	// define term structure
        // Nelson-Siegel-Svensson parameters as 2021-03-31 for Swiss government bonds
	term := term.NelsonSiegelSvensson{
		-0.266372,
		-0.471343,
		5.68789,
		-5.12324,
		5.74881,
		4.14426,
		0.0,
	}
```

```go
	// price risk-free bond
	value := straightBond.PresentValue(&term)

	// modified duration
	duration := straightBond.Duration( &term)

	// accrued interest (30/360 day convention) and "dirty" price of bond
	accrued := straightBond.Accrued()
	cleanPrice := value - accrued

	// internal rate of return given a market price
	irr, _ := fixedincome.IRR(109.70, straightBond)

	// implied static spread
	spread, _ := fixedincome.Spread(109.70, straightBond, &term)
```

## Further reading

- [Nelson-Siegel-Svensson model at SNB](https://www.snb.ch/de/mmr/reference/quartbul_2002_2_komplett/source/quartbul_2002_2_komplett.de.pdf) on page 64
