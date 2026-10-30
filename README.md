<h1 align="center">fixedincome</h1>

<p align="center">
  Term structures and valuation of fixed income securities in Go,<br>
  with a command line tool, <code>fic</code>, for everyday curve and bond work.
</p>

<p align="center">
  <a href="https://github.com/konimarti/fixedincome/blob/main/LICENSE"><img alt="License" src="https://img.shields.io/badge/license-MIT-red.svg?style=flat"></a>
  <a href="https://pkg.go.dev/github.com/konimarti/fixedincome"><img alt="Go Reference" src="https://pkg.go.dev/badge/github.com/konimarti/fixedincome.svg"></a>
  <a href="https://goreportcard.com/report/github.com/konimarti/fixedincome"><img alt="Go Report Card" src="https://goreportcard.com/badge/github.com/konimarti/fixedincome"></a>
</p>

---

## Features

| | |
|---|---|
| **Curves** | Flat, Nelson-Siegel-Svensson (NSS) and cubic-spline term structures; z-spread overlays; forward rates |
| **Curve fitting** | Fit NSS or spline curves to bond prices with a yield-error objective, optional overnight-rate constraint and robust (Huber) loss |
| **Central bank data** | Download the published NSS parameters of the SNB (CHF) and the ECB (EUR) |
| **Bonds** | Fixed-coupon and floating-rate: price, yield, z-spread, duration, convexity, DV01 |
| **Derivatives** | Forwards, FRAs, interest rate swaps (single and dual curve), European options with Greeks and implied volatility |
| **Monte Carlo** | Ho-Lee, Hull-White, Vasicek and GBM; European, Asian and American (Longstaff-Schwartz) options; seedable and parallel |

## Install

```sh
go install github.com/konimarti/fixedincome/cmds/fic@latest   # command line tool
go get github.com/konimarti/fixedincome                       # library
```

## Use cases with `fic`

**Get a current yield curve** (SNB for CHF, ECB for EUR)

```sh
fic curve fetch --source ecb -o curve.json
fic curve show --curve curve.json --t 1y,2y,5y,10y
```

**Fit a curve to your own bonds**

```sh
fic curve fit --bonds bonds.csv --date 2021-11-26 --model nss -o fit.json --residuals residuals.csv
```

The bond file is `maturity,coupon_pct,price_clean[,id,freq,daycount,face]`; see [`data/`](data/README.md) for a sample.

**Price a bond: yield, z-spread and risk from a quote**

```sh
fic bond price --curve curve.json --coupon 1.25 --maturity 2026-05-25 --price 109.70
```

```
yield_cc_pct: -0.59963302      mod_duration: 4.9821577
zspread_bp:   0.029061277      dv01:         -0.05516782
```

**Price a whole bond list against a curve**

```sh
fic bond price --curve curve.json --in bonds.csv --format csv > priced.csv
```

**Par swap rates, option prices and short-rate simulation**

```sh
fic swap rate --curve curve.json --tenors 2y,5y,10y
fic option price --type call --spot 110 --strike 100 --expiry 2y --vol 30 --rate 2
fic sim zcb --model holee --curve curve.json --sigma 1 --paths 10000 --antithetic
```

Output is `text`, `csv` or `json` (`--format`); data goes to stdout, diagnostics to stderr. Run `fic help <command>` for
all flags, or read the [full reference](docs/cli.md).

## Library

```go
settle := time.Date(2021, 4, 17, 0, 0, 0, 0, time.UTC)

// SNB curve of 2021-03-31 (decimals: 0.0568789 = 5.68789%)
curve, _ := term.NewNSS(-0.00266372, -0.00471343, 0.0568789, -0.0512324, 5.74881, 4.14426)

// 1.25% annual bond, 30E/360 accrual
b, _ := bond.NewStraight(maturity.Schedule{
	Maturity:  time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC),
	Frequency: maturity.Annual,
	DayCount:  maturity.Thirty360E,
}, 0.0125)

cfs, _ := b.CashFlows(settle)
accrued, _ := b.Accrued(settle)

dirty := cashflow.PV(cfs, curve)                          // value on the curve
clean := dirty - accrued
duration := cashflow.Duration(cfs, curve)                 // positive
dv01 := cashflow.DV01(cfs, curve)                         // negative for a long bond
yCC, _ := cashflow.YieldCC(109.70+accrued, cfs)           // from a clean quote
z, _ := cashflow.ZSpread(109.70+accrued, cfs, curve)
```

Options (`pkg/instrument/option`), swaps (`pkg/instrument/swap`), Monte Carlo (`pkg/mc`) and curve fitting
(`pkg/fit`) follow the same pattern; see the [package documentation](https://pkg.go.dev/github.com/konimarti/fixedincome).
More involved securities (range floater, leveraged inverse floater) are in [`example/`](example/).

## Conventions

- **Library: decimals. CLI, CSV and JSON: percent** for rates, yields, volatilities and dividend yields, and
  **basis points** for spreads.
- Rates are **continuously compounded**; outputs show the annual-compounded equivalent next to it.
- The time axis is **ACT/365 Fixed** from the curve date, independent of the accrual day count.
- Curves are immutable and safe for concurrent use. Invalid input yields errors, not panics.

## More

- [`docs/cli.md`](docs/cli.md): complete `fic` reference (flags, output columns, schemas, pipeline example, data sources)
- [`r/`](r/README.md): R scripts for plotting fitted curves and simulations
- [`CHANGELOG.md`](CHANGELOG.md): changes and migration tables from the previous tools

Central bank parameters: [SNB](https://data.snb.ch/en/topics/ziredev#!/cube/rendopar) (the cube currently ends on
2025-07-31, so use `--source ecb` for current data) and
[ECB](https://www.ecb.europa.eu/stats/financial_markets_and_interest_rates/euro_area_yield_curves/html/index.en.html).
Check their terms of use before redistributing data.

## License

MIT
