# Valuation of Fixed Income Securities

[![License](http://img.shields.io/badge/license-MIT-red.svg?style=flat)](https://github.com/konimarti/fixedincome/blob/master/LICENSE)
[![GoDoc](https://godoc.org/github.com/konimarti/fixedincome?status.svg)](https://godoc.org/github.com/konimarti/fixedincome)
[![goreportcard](https://goreportcard.com/badge/github.com/konimarti/fixedincome)](https://goreportcard.com/report/github.com/konimarti/fixedincome)

A Go library and a command line tool (`fic`) for the valuation of fixed income securities with spot-rate
term structures or continuous-time interest-rate models:

- Curves: flat, Nelson-Siegel-Svensson (NSS) and cubic-spline term structures, fitted to bond prices
  (`pkg/fit`), z-spread overlays, forward rates
- Fixed-coupon and floating rate bonds (price, yield, z-spread, duration, convexity, DV01)
- Forward contracts, forward rate agreements, interest rate swaps (single and dual curve)
- European options (Black-Scholes, Greeks, implied volatility)
- Monte Carlo (parallel, seedable, antithetic and control variates): Ho-Lee, Hull-White and Vasicek short rate
  models with exact simulation, geometric Brownian motion; European, Asian and American (Longstaff-Schwartz) options

## Install

```sh
go get github.com/konimarti/fixedincome            # library
go install github.com/konimarti/fixedincome/cmds/fic@latest   # command line tool
```

## Conventions

- **Decimal inside, percent and basis points at the boundary.** The library works with decimals (`0.02` = 2%,
  `0.0001` = 1bp). Command line flags, CSV and JSON files use percent for rates, yields, volatilities and
  dividend yields and basis points for spreads; the conversion happens in one place (`internal/cli/units.go`).
- Rates are **continuously compounded** (`pkg/rate` converts to annual, n-times or simple rates). Outputs that show
  a yield or zero rate show both `_cc_pct` and `_annual_pct`.
- The time axis is **ACT/365 Fixed** from the settlement (curve) date, independent of the accrual day count.
- Curves (`term.Curve`) are immutable and safe for concurrent use; `term.WithSpread` adds a z-spread.
- Every bond reduces to cash flows; the analytics in `pkg/cashflow` operate on cash flows.
- Invalid user input yields errors, not panics.

## Library quick start

```go
settle := time.Date(2021, 4, 17, 0, 0, 0, 0, time.UTC)

// curve: NSS parameters of the SNB as of 2021-03-31 (decimals; the curve files have percent)
curve, err := term.NewNSS(-0.00266372, -0.00471343, 0.0568789, -0.0512324, 5.74881, 4.14426)

// bond: 1.25% annual coupon, 30E/360 accrual, face and redemption 100
b, err := bond.NewStraight(maturity.Schedule{
	Maturity:  time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC),
	Frequency: maturity.Annual,
	DayCount:  maturity.Thirty360E,
}, 0.0125)

// cash flows, accrued interest and prices
cfs, _ := b.CashFlows(settle)
accrued, _ := b.Accrued(settle)
dirty := cashflow.PV(cfs, curve)
clean := dirty - accrued

// risk (duration is positive; DV01 is the value change for +1bp, negative for a long bond)
duration := cashflow.Duration(cfs, curve)
convexity := cashflow.Convexity(cfs, curve)
dv01 := cashflow.DV01(cfs, curve)

// yield and z-spread of a quoted clean price (the dirty price is the quote plus accrued)
yCC, _ := cashflow.YieldCC(109.70+accrued, cfs)
yAnnual, _ := cashflow.Yield(109.70+accrued, cfs, 1)
z, _ := cashflow.ZSpread(109.70+accrued, cfs, curve)

// value with a spread of 50bp
risky := cashflow.PV(cfs, term.WithSpread(curve, 0.005))

// forward rate between 1 and 2 years, annually compounded
fwd, _ := rate.FromContinuous(term.Forward(curve, 1, 2), 1)

// par swap rate for annual fixed payments from the curve discount factors
par, _ := swap.ParRateFromTimes([]float64{1, 2, 3, 4, 5}, 1, curve)

// European option on a flat curve; vol and rates are decimals
opt := option.European{Type: option.Call, S: 110, K: 100, T: 2, Vol: 0.3}
price, _ := opt.Price(term.Flat{Rate: 0.02})
greeks, _ := opt.Greeks(term.Flat{Rate: 0.02}) // Delta, Gamma, Vega, Theta, Rho, Phi
iv, _ := option.ImpliedVol(price, opt, term.Flat{Rate: 0.02})

// Monte Carlo: zero bond prices of a Ho-Lee model (exact, seeded, parallel)
model, _ := holee.New(curve, 0.01)
grid, _ := mc.NewUniformGrid(10, 120)
zeros, _ := mc.PriceMulti(model, mc.ZeroBonds{Grid: grid}, grid, mc.Options{Paths: 100000, Seed: 1, Antithetic: true})
```

More exotic securities (range floater, leveraged inverse floater) are in `example/`. Fitting curves to bond
prices: `fit.ReadBondsCSV`, `fit.PrepareBonds`, `fit.FitNSS`, `fit.FitSpline` (see the package documentation).

## The `fic` command

```
fic curve fetch | fit | show | forward | template     fic bond price     fic swap rate     fic option price     fic sim zcb
fic version        fic help [command]
```

Flags work with one or two dashes (`--curve` is documented). `fic help <command>` lists the flags.

### Global conventions

| | |
|---|---|
| Units | rates, yields, vols, dividend yields in **percent**; spreads in **bp**; dates `YYYY-MM-DD` or `DD.MM.YYYY` (everywhere a date is read, output dates are ISO); maturities, tenors and horizons as dates or `6m`, `2y`, `10d`, `3w` |
| Curve | `--curve FILE` (env `FIC_CURVE`, default `./curve.json`; `-` is stdin; the old name `FI_CURVE` is still accepted with a deprecation warning when it is the only one set). The curve date is the `asof` of the file. A `--spread` so large that the discount factors under- or overflow is an error (exit 1) |
| Date | `--date`: default the `asof` of the curve file, else today; a warning on stderr if `--date` and `asof` differ |
| Spread | the `spread_bp` of the curve file is kept, `--spread BP` is **added** to it |
| Formats | `--format text\|csv\|json`, `-o FILE` (default stdout). **Output files are atomic and non-destructive:** the data is written to a temporary file next to `-o` and renamed on success, an existing file is never damaged or deleted by a failing run, and `-o` equal to an input file (`--curve`, `--in`, `--bonds`, `--start`, `--fwd-curve`; compared with `os.SameFile`) is a usage error (exit 2). Default format for tables (`curve show/forward`, batch `bond price`, `swap rate`, `sim zcb`): text on a terminal, **csv** when stdout is not a terminal or `-o` is given (a terminal is a character device other than `/dev/null`; this is not a full `isatty` check); for single values (`bond price` of one bond, `option price`): **text** |
| CSV / JSON | header always, snake_case names with unit suffix (`ttm_years`, `price_clean`, `yield_cc_pct`, `zspread_bp`, ...), comma separated, ISO dates, numbers `%.10g` (negative zero is printed as `0`); text fields that start with `=`, `+`, `-`, `@`, tab or CR get a leading apostrophe in CSV (spreadsheet formula guard; numbers are untouched); JSON is one object (single value) or an array of objects (tables) with the same names; non-finite numbers are empty (csv) or `null` |
| Streams | stdout is data only; warnings, summaries and progress go to stderr; nothing is written to the working directory unless `-o` (or `--residuals`) is given |
| Exit codes | `0` ok, `1` runtime or numerical error (unreadable file, matured bond, no yield found, all batch rows failed), `2` usage error (unknown command or flag, invalid value, missing required flag). Errors are one line `fic <command>: message` |

### Commands and flags

| Command | Flags (units) |
|---|---|
| `fic curve fetch` | `--source snb\|ecb` (required), `--date D\|latest` (default latest = the latest date with all six parameters), `--file FILE\|-` (parse a downloaded file, no network), `--ecb-issuers aaa\|all` (default aaa), `--max-age DAYS` (default 14; warn if the latest parameters are older, 0 = never), `--strict` (with `latest`: exit 1 and write no output if the data is older than `--max-age`), `--timeout 30s`, `--spread BP` (written as `spread_bp`, at most 100000 in absolute value), `-o FILE` (curve JSON, default stdout). Downloads the published NSS parameters (SNB: CHF, source `SNB rendopar`; ECB: EUR, `ECB AAA` or `ECB all issuers`) and writes a curve file with `asof`, `currency` and `source`; after the output was written, stderr gets warnings and a summary with the 1y/5y/10y zero rates (tagged `[STALE: N days]` when the data is older than `--max-age`). `--ecb-issuers` given explicitly must match the variant of a `--file`; files that mix several instruments, currencies or areas are rejected. Exit 1 for network or data errors (also a `--date` without a complete parameter set: the error names the nearest earlier date) |
| `fic curve fit` | `--bonds FILE\|-` (bond CSV), `--date D` (`YYYY-MM-DD` or `DD.MM.YYYY`, default today), `--model nss\|spline\|both` (default nss; `both` needs `-o` and writes `<stem>.nss.json`, `<stem>.spline.json`), `--onrate PCT` (overnight rate, simple ACT/360; constrains the short end), `--start CURVE` (NSS starting point), `--min-ttm Y` (default 0.1; <= 0 keeps all), `--lambda X\|gcv`, `--objective yield\|price`, `--huber BP`, `--settle-lag DAYS`, `-o FILE` (curve JSON, default stdout), `--residuals FILE` (per-bond CSV). The `asof` of the curve is the settlement date; `source` the bond file. Warnings of the fit go to stderr |
| `fic curve show` | `--curve`, `--grid start:end:step` (default `1m:10y:1m`) or `--t 1y,2y,5y`, `--spread BP`, `--format`, `-o`. Columns `t_years, zero_cc_pct, zero_annual_pct, discount_factor, inst_fwd_cc_pct` |
| `fic curve forward` | as `show` plus `--tenor 1y`. Columns `t_years, tenor_years, zero_cc_pct, discount_factor, fwd_cc_pct, fwd_annual_pct, fwd_discount_factor` |
| `fic curve template` | `--model nss\|spline\|flat`, `-o`: a valid curve file |
| `fic bond price` | `--curve`, `--date`, `--spread BP`, `--maturity D\|5y`, `--coupon PCT`, `--freq 1\|2\|3\|4\|6\|12` (default 1), `--daycount 30E360\|EUROBOND\|BONDBASIS\|ACT360\|ACTACT`, `--face 100`, `--redemption` (default: the face value), one of `--price CLEAN` (percent of face) or `--yield PCT` (cc), or `--in FILE\|-` for a bond CSV (batch) |
| `fic swap rate` | `--curve`, `--fwd-curve FILE` (forecast curve of the floating leg, default the same curve), `--tenors 2y,3y,...,10y`, `--freq 2`, `--spread BP` (on the discount curve). Swaps start at the curve date, so par rates do not depend on a valuation date and `swap rate` has no `--date` flag. Columns `tenor, tenor_years, freq, par_rate_pct, annuity` |
| `fic option price` | `--type call\|put`, `--spot S`, `--strike K`, `--expiry 2y\|2.5\|DATE` (a date `YYYY-MM-DD` / `DD.MM.YYYY` is relative to `--date`), `--vol PCT` or `--implied-from PRICE`, `--rate PCT` or `--curve FILE` (zero rate at the expiry), `--div PCT`, `--date`. Columns `type, spot, strike, expiry_years, vol_pct, rate_cc_pct, div_pct, price, delta, gamma, vega_per_volpt, theta_per_day, rho_per_pct, phi_per_pct` |
| `fic sim zcb` | `--model holee\|hullwhite\|vasicek`, `--curve`, `--sigma PCT` (short rate vol per sqrt(year), 1 = 100bp), `--mean-reversion A` (hullwhite), `--spread BP` (added to the curve file spread), `--horizon 10y`, `--steps 120`, `--paths 10000`, `--seed N` (default 1, 0 = time based), `--antithetic`. Columns `t_years, curve_zero_cc_pct, curve_fwd_cc_pct, curve_discount, model_discount, mc_discount, mc_stderr, mc_zero_cc_pct, z_score` |

**`fic option price`, Greek units.** The Greeks are scaled at the boundary to practical units: `delta` (per 1 of
spot), `gamma` (per 1 of spot, per 1 of spot), `vega_per_volpt` (price change per 1 volatility point = 1%),
`theta_per_day` (per calendar day, the annual theta divided by 365), `rho_per_pct` (per 1 percentage point of the
rate) and `phi_per_pct` (per 1 percentage point of the dividend yield). The library (`option.Greeks`) returns
vega, rho and phi per 1.00 (100%) and theta per year.

**`fic bond price`, single bond.** `--price` gives the yields and the spreads; `--yield` gives the price; without
either the bond is valued on the curve. All prices are per 100 of face. Yields are those of the market dirty price
(quote plus accrued). Two spreads are reported:

- `zspread_bp`: the spread over the effective curve (file `spread_bp` plus `--spread`), i.e. the shift that reprices
  the market price;
- `spread_total_bp` = file `spread_bp` + `--spread` + z-spread: the spread over the curve with **no** spread (the
  base curve). With a curve file without spread and no `--spread` both are equal. This is what `bonds-cli` called
  "Implied spread".

`mod_duration` (positive), `convexity` and `dv01` (value change for +1bp, negative for a long bond) are evaluated on
the curve shifted by the z-spread, i.e. at the market price (without a quote: on the curve). Text output, one
`label: value` per line (`zspread_bp` only with a price or yield; `spread_total_bp` is always shown):

```
settlement: 2021-03-31
maturity: 2026-05-25
years_to_maturity: 5.1534247
accrued: 1.0590278
price_dirty: 110.75903
price_clean: 109.7
price_clean_model: 109.7016
yield_cc_pct: -0.59963302
yield_annual_pct: -0.59783881
zspread_bp: 0.029061277
spread_total_bp: 0.029061277
mod_duration: 4.9821577
convexity: 25.419066
dv01: -0.05516782
```

Cash flows: for 30/360-family bonds the coupon per period is `C / freq`; for ACT/360 the coupon of a period is
`C x days/360` (and for ACT/ACT ICMA `C/freq x days/period days`), i.e. it equals the accrued interest of the
period (design of the core library, see the CHANGELOG). The cash flows of an `ACT360` bond therefore differ from
a plain `C/freq` schedule.

**`fic bond price --in`, batch.** Columns `id[, issuer], maturity, coupon_pct, freq, daycount, ttm_years, accrued,
price_source, price_clean, price_dirty, price_clean_model, yield_cc_pct, yield_annual_pct, zspread_bp,
spread_total_bp, mod_duration, convexity, dv01, status`.

- A row with a price (`price_source` = `market`) gets yields and spreads of the market price.
- A row with an empty or `N/A` price is valued on the curve (`price_source` = `model`, `status` = `ok`):
  `price_clean`/`price_dirty` are the model prices, the market-based columns (`yield_cc_pct`, `yield_annual_pct`,
  `zspread_bp`, `spread_total_bp`) are empty and duration, convexity and DV01 are on the unshifted curve.
- A row that cannot be read or priced (matured, bad date, bad number, no yield) has the error text in `status` and
  **echoes the raw input fields** (`maturity`, `coupon_pct`, `freq`, `daycount`) with empty numbers; it does not
  abort the run. The exit code is 1 only if **all** rows failed; one line on stderr gives the number of failed rows and
  the ids of the first ones.

### Data schemas

Curve file (JSON; percent and bp; `model` is `nss`, `spline` with `knots` and `discount_factors`, or `flat`
with `params: {"r": ...}`; legacy files without `model` are still read):

```json
{"model": "nss", "asof": "2021-03-31", "currency": "CHF", "source": "SNB",
 "params": {"b0": -0.266372, "b1": -0.471343, "b2": 5.68789, "b3": -5.12324, "t1": 5.74881, "t2": 4.14426},
 "spread_bp": 0}
```

Bond file (CSV, see [data/README.md](data/README.md)): `maturity,coupon_pct,price_clean[,id,freq,daycount,face,redemption,issuer]`,
coupon in percent, price in percent of face, missing price = empty field.

### Pipeline example

This runs literally from a fresh checkout without network (the `--file` and `-f` fixtures replace the downloads).
Online, step 1 is `fic curve fetch --source snb -o curve.json` (CHF) or `fic curve fetch --source ecb -o curve.json`
(EUR), and `scripts/six-bonds.zsh Glarner > bonds.csv` for the bonds. The scripts call `fic` from the `PATH` (or
`$FIC`).

```sh
go build -o fic ./cmds/fic && export PATH="$PWD:$PATH"
# 1. a curve file from a downloaded SNB CSV (latest complete date; warns on stderr that the data is old);
#    online: fic curve fetch --source snb   or   fic curve fetch --source ecb
fic curve fetch --source snb --file scripts/testdata/snb_live_sample.csv -o curve.json
# 2. bonds in the bond CSV format (missing prices stay empty)
scripts/six-bonds.zsh -f scripts/testdata/six_ref.csv > bonds.csv
# 3. fit curves to bonds (NSS and spline, written to fit.nss.json and fit.spline.json) and keep the residuals;
#    warnings and the RMSE go to stderr
fic curve fit --bonds data/bonds_2021-11-26.csv --date 2021-11-26 --onrate -0.75 --model both \
   -o fit.json --residuals residuals.csv
# 4. price the bonds on the SNB curve: yield, z-spread, duration per bond (rows without a price are valued on the curve)
fic bond price --curve curve.json --in bonds.csv --format csv > priced.csv
scripts/avg-spread.zsh -c curve.json bonds.csv        # duration-weighted z-spread per issuer
# 5. plots (base graphics, PNG)
fic curve show --curve fit.nss.json --grid 1m:30y:1m -o curve.csv
Rscript r/curve.R residuals.csv curve.png curve.csv
```

`scripts/test.zsh` tests the scripts with the fixtures in `scripts/testdata/` (no network); `FIC` selects the
binary (`FIC=./fic zsh scripts/test.zsh`).

### Regenerating the committed samples

`data/` holds the curve and bonds. `cmds/termfit/result.csv` and `cmds/termfit/termfit.png` are the output of
exactly (the `--onrate -0.75` matters):

```sh
fic curve fit --bonds data/bonds_2021-11-26.csv --date 2021-11-26 --onrate -0.75 --model both \
   -o fit.json --residuals cmds/termfit/result.csv
Rscript r/curve.R cmds/termfit/result.csv cmds/termfit/termfit.png
```

(`fit.nss.json` and `fit.spline.json` are by-products and are ignored by git.) A test
(`TestRegenerateSample` in `internal/cli`) regenerates `result.csv` in-process and compares it byte for byte.

### Legacy tools (deprecated shims)

`bonds-cli`, `swaprate-cli`, `option-cli`, `termfit`, `fwdcrv` and `holee` still work with their old flags and
print `deprecated: use "fic ..."` to stderr.

**Output format changes.** There is no legacy-output switch:

- `bonds-cli` and `swaprate-cli` print the text output of `fic bond price` / `fic swap rate` (labels and
  columns as documented above) instead of the old human-readable layout. The old "Implied spread" is
  `spread_total_bp`. Duration and convexity are now evaluated at the market price (on the curve shifted by the
  z-spread) when a quote is given (the old tool used the duration of the unshifted curve). `-spread` is added to
  the spread of the curve file (it replaced it before).
- `option-cli` keeps its old text output and raw units (Greeks per 1.00; `-vola` is a decimal, and a warning is
  printed for `-vola` > 5). `fic option price` uses `--vol` in percent and the scaled Greeks above.
- `termfit`, `fwdcrv` and `holee` keep their legacy output files and formats.

| Old | New |
|---|---|
| `bonds-cli -f F -settlement D -maturity M -coupon C -n N -quote P -redemption R -spread S -daycount X` | `fic bond price --curve F --date D --maturity M --coupon C --freq N --price P --redemption R --spread S --daycount X` (`-quote 0` = no price) |
| `swaprate-cli -f F -s S` | `fic swap rate --curve F --spread S` |
| `option-cli -type T -s S -k K -t T -q Q -vola V -r R` | `fic option price --type T --spot S --strike K --expiry T --div Q --vol V*100 --rate R` (`-vola` was a decimal, `--vol` is percent) |
| `termfit -file F -date D -settle-lag N -onrate R -f START -min-ttm Y -lambda L -huber H -objective O -model M` | `fic curve fit --bonds F --date D --settle-lag N --onrate R --start START --min-ttm Y --lambda L --huber H --objective O --model M -o curve.json --residuals residuals.csv` (the shim still writes `nss_opt.json`, `spline_opt.json`, `result.csv` to the current directory and uses `term.json` as a starting point if it exists; `fic` writes nothing unless `-o` / `--residuals` is given; `-lambda -1` = `--lambda gcv`) |
| `fwdcrv -f F -m M -t T` | `fic curve forward --curve F --tenor M --t T` (the shim still writes `forward.csv` with the columns `x,SpotRate,FwdRate,Z,F`) |
| `holee -f F -n N -m M -s S -paths P -seed X -o O` | `fic sim zcb --model holee --curve F --steps N --horizon M --sigma S*100 --paths P --seed X -o O` (`-s` was a decimal, `--sigma` is percent; the shim keeps the old CSV `t,zero_rate,fwd_rate,theta,exp_short_rate,discount,mc_discount,mc_stderr,mc_zero_rate` with decimals; `-seed 0` is time based in both) |

## Nelson-Siegel-Svensson parameters

Many central banks publish daily fitted parameters of the Nelson-Siegel-Svensson model:

- Swiss National Bank (SNB) for [CHF risk-free spot rates](https://data.snb.ch/en/topics/ziredev#!/cube/rendopar)
  (cube `rendopar`, `fic curve fetch --source snb`)
- European Central Bank (ECB) for [EUR risk-free spot rates](https://www.ecb.europa.eu/stats/financial_markets_and_interest_rates/euro_area_yield_curves/html/index.en.html)
  (AAA-rated and all euro area government bonds, `fic curve fetch --source ecb`)

`fic curve fetch` (library: `pkg/source`, the only package that uses the network) downloads these parameters; both
banks publish betas in percent and taus in years, the curve file schema. **Data freshness:** at the time of writing
(October 2026) the SNB cube `rendopar` ends on 2025-07-31 (publishing date 2025-09-01), so `--source snb` returns
that date and warns when it is older than `--max-age` days (`--strict` makes this an error, exit 1, no output); use `--source ecb` for current data. A newer date with missing parameters is skipped with a warning. The as-of date
defaults to the latest date with all six parameters; an explicit `--date` without a complete set is an error that
names the nearest earlier date (the ECB publishes on TARGET business days). Data terms of use:
[SNB data portal](https://data.snb.ch), [ECB statistics usage policy](https://www.ecb.europa.eu/stats/ecb_statistics/governance_and_quality_framework/html/usage_policy.en.html);
check them before redistributing the data. Network behaviour: one request per call, no retries, `Retry-After` is not honoured, proxies from the environment (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`) are used, redirects must be https and are followed across hosts (at most 5), responses are limited to 64 MiB. `scripts/snb.zsh` is a deprecated wrapper of `fic curve fetch --source snb`.

Further reading: [Nelson-Siegel-Svensson model at SNB](https://www.snb.ch/de/mmr/reference/quartbul_2002_2_komplett/source/quartbul_2002_2_komplett.de.pdf) (page 64).

See [CHANGELOG.md](CHANGELOG.md) for the API and CLI migration tables.
