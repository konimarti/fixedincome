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

## Conventions

- All rates, yields, spreads and volatilities are decimals (`0.02` = 2%, `0.0001` = 1bp) and
  continuously compounded; use `pkg/rate` to convert to annually (or n-times) compounded rates.
  Percent and basis points are only used in curve files and on the command line.
- Times are in years, ACT/365 Fixed from the settlement (curve) date.
- Curves (`term.Curve`) are immutable and safe for concurrent use; a z-spread is applied with
  `term.WithSpread`.
- Every bond reduces to cash flows; the analytics in `pkg/cashflow` operate on cash flows.

## Code example for a straight bond

- Valuation of more exotic securities are given in the example folder

```go
	// define straight bond (coupon as decimal; face value and redemption 100 by default)
	settle := time.Date(2021, 4, 17, 0, 0, 0, 0, time.UTC)
	straightBond, err := bond.NewStraight(maturity.Schedule{
		Maturity:  time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC),
		Frequency: maturity.Annual,
		DayCount:  maturity.Thirty360E,
	}, 0.0125)
	if err != nil {
		log.Fatal(err)
	}

	// define term structure
	// Nelson-Siegel-Svensson parameters as 2021-03-31 for Swiss government bonds
	// (the SNB publishes the betas in percent)
	curve, err := term.NewNSS(-0.00266372, -0.00471343, 0.0568789, -0.0512324, 5.74881, 4.14426)
	if err != nil {
		log.Fatal(err)
	}
```

```go
	// cash flows after the settlement date
	cfs, err := straightBond.CashFlows(settle)
	if err != nil {
		log.Fatal(err)
	}

	// price risk-free bond ("dirty" price) and accrued interest (30E/360)
	dirty := cashflow.PV(cfs, curve)
	accrued, _ := straightBond.Accrued(settle)
	cleanPrice := dirty - accrued

	// modified duration (positive, in years) and convexity
	duration := cashflow.Duration(cfs, curve)
	convexity := cashflow.Convexity(cfs, curve)

	// yield given a market price (the "dirty" price, i.e. quote plus accrued interest):
	// continuously compounded and annually compounded
	yieldCC, _ := cashflow.YieldCC(109.70+accrued, cfs)
	yieldAnnual, _ := cashflow.Yield(109.70+accrued, cfs, 1)

	// implied static (z-)spread over the curve
	spread, _ := cashflow.ZSpread(109.70+accrued, cfs, curve)

	// value with a spread of 50bp
	risky := cashflow.PV(cfs, term.WithSpread(curve, 0.005))
```

## Curve files

Curves are read with `term.Parse` and written with `term.Marshal`. Rates and betas are in
percent, the spread in basis points:

```json
{"model": "nss", "asof": "2021-03-31", "currency": "CHF", "source": "SNB",
 "params": {"b0": -0.266372, "b1": -0.471343, "b2": 5.68789, "b3": -5.12324, "t1": 5.74881, "t2": 4.14426},
 "spread_bp": 0}
```

The legacy format without `"model"` (e.g. `term.json`) is still accepted.

## Further reading

- [Nelson-Siegel-Svensson model at SNB](https://www.snb.ch/de/mmr/reference/quartbul_2002_2_komplett/source/quartbul_2002_2_komplett.de.pdf) on page 64
