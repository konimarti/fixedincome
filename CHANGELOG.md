# Changelog

## Unreleased

### Added

- **`fic portfolio risk --curve C --in positions.csv`**: interest rate risk of a bond portfolio with long and short
  positions (bond CSV plus a required `nominal` column, negative for shorts): market value, money-weighted
  duration and convexity, DV01, key rate durations (triangular bumps, `--krd-tenors`; effective ones by lattice
  recalibration for callable rows) as years and as currency per bp, full-revaluation scenarios with the spreads held
  (`--scenarios parallel|irrbb|none`, `--shocks`, `--irrbb-parallel/short/long`; the IRRBB magnitudes are illustrative
  defaults and no floor on the shocked rates is applied) next to the duration/convexity approximation, `--group-by issuer|rating|freq|daycount`,
  `--section summary|krd|scenarios|positions`, `--strict` (no output if a row is excluded or unpriced), `gross_mv`;
  duration, KRD and weights are empty for a hedged book (net value ~0). See docs/cli.md. Sample: `data/positions_example.csv`.
- Library: package `portfolio` (`Evaluate`, `Aggregate`, `GroupBy`, `Shift`, `Scenario`, `IRRBB`) and
  `cashflow.KeyRateWeight`.
- **Callable and putable bonds in `fic bond price`**: `--call DATE@PRICE[,...]`, `--put`, `--call-from DATE@PRICE`,
  `--vol PCT` (required), `--mean-reversion` (default 0.03), `--steps` (default 50). Hull-White lattice value, OAS,
  yield to worst, effective duration and convexity, DV01, option value; `--in` accepts the CSV columns `call`,
  `call_from` and `put` and adds the callable columns only if a row has terms. `--group-by` uses the OAS for callable
  rows. See docs/cli.md. Sample: `data/bonds_callable_example.csv`.
- Library: package `lattice` (Hull-White trinomial tree) and `bond.Callable`.

## fic bond fetch --source six

- **`fic bond fetch --source six [--issuer NAME] [--rating AAA,AA,...] [--currency CHF] [--file FILE|-]
  [--timeout 30s] [--format csv] [-o FILE]`**: bonds of SIX Swiss Exchange (segment BO) with delayed closing prices as
  a bond CSV `maturity,coupon_pct,price_clean,id,freq,daycount,issuer,rating`, sorted by maturity, then ISIN. `freq`
  comes from `InterestFrequencyCode` (1, 2, 4), `daycount` from `AccruedInterestCalcCode` (`A011`, `A013` to
  `EUROBOND`, `A001` to `BONDBASIS`, `A004` to `ACT360`); perpetuals, flat bonds (`A000`) and unknown codes are
  skipped and counted on stderr. ISO 8859-1 responses are converted, quoting is strict, the HTML error page of the
  service is reported with an excerpt. Library: `source.SIXQuery`, `source.ParseSIX`, `Client.FetchSIX`.
- **`rating` column** in bond files: `fic bond price --in` carries it through (next to `issuer`) and
  `--group-by` accepts `rating`; `fic curve fit` ignores it.
- **`scripts/six-bonds.zsh` is a deprecated wrapper** of the new command (`-f FILE`, `ISSUER` kept; prints a note).
  The output columns changed from `maturity,coupon_pct,price_clean,id,issuer` to
  `maturity,coupon_pct,price_clean,id,freq,daycount,issuer,rating`; rows with an invalid coupon (the old script
  wrote an empty coupon) are skipped with a warning, and with `-f` a given ISSUER now filters the file. Removed:
  `scripts/lib/six.awk` and `scripts/lib/csv.awk`.

## Analysis in fic

Three analysis features move into the `fic` binary.

- **`fic curve fit --report FILE`**: a self-contained HTML report of the fit (inline CSS and SVG, no scripts or
  external assets, escaped text, light and dark scheme, deterministic output): as-of date, bonds, RMSE, fit warnings,
  fitted zero curve(s) with the market yields (`--model both`: both models), yield errors in bp per bond, parameter
  and residual tables. Atomic output, same-file refusal as for the other outputs. Sample:
  `docs/fit_report_sample.html`. New package `internal/report`.
- **`fic bond price --in FILE --group-by issuer|freq|daycount`**: one row per group with `n`, the duration-weighted
  `avg_zspread_bp`, `avg_mod_duration` and `dv01_sum`, sorted by spread; only rows with status `ok` and a market
  price count, the others are reported on stderr. Formats `text`, `csv`, `json`.
- **`fic curve compare A.json B.json`**: zero rates and instantaneous forwards of two curves on a grid or a list of
  maturities (`--grid`, `--t` as in `curve show`), with the differences (B minus A) in bp; stderr gets a warning if the
  `asof` dates differ and the mean, maximum and RMS of the zero-rate difference. Commands can now take positional
  arguments (mixed with flags).
- **Removed: `scripts/avg-spread.zsh` and `scripts/lib/avg-spread.awk`** (and their cases in `scripts/test.zsh`).
  Migration: `avg-spread.zsh -c curve.json -d DATE bonds.csv` is
  `fic bond price --curve curve.json --date DATE --in bonds.csv --group-by issuer --format csv`. Differences: without
  an `issuer` column the new command fails (exit 1) instead of printing one group `all`; the output has the extra
  column `dv01_sum`; the skipped-rows message is a `fic bond price` warning. `scripts/lib/csv.awk` was removed later
  with `six-bonds.zsh`'s awk code. The R scripts in `r/` are now optional, for custom plots.

## Removed (cleanup)

The deprecated code of WP5 is gone. Nothing else changed; `fic` and the library behave as before.

- **The six legacy binaries** `bonds-cli`, `swaprate-cli`, `option-cli`, `termfit`, `fwdcrv` and `holee`
  (`cmds/*`, the shims in `internal/cli/legacy.go` and their tests). `cmds/` contains only `cmds/fic`.
- **The `FI_CURVE` environment variable.** Use `FIC_CURVE`.
- **The legacy curve file format** (no `"model"` field, model sniffed from the keys `b0`..`t2`, `maturities` +
  `discountfactors`, `r`, and `spread`). `term.Parse` returns the error
  `curve file has no "model" field (legacy format is no longer supported; see "fic curve template")`.
  Migration: add `"model": "nss"`, `"spline"` or `"flat"`, move the values into `"params"` (NSS: `b0`..`b3`, `t1`,
  `t2`; flat: `r`) or `"knots"` / `"discount_factors"` (spline; now strictly increasing knots), and rename
  `spread` to `spread_bp`; `fic curve template` prints examples. The root `term.json` and `cmds/holee/term.json`
  are deleted.
- Moved: `cmds/termfit/result.csv` is `data/fit_residuals_2021-11-26.csv`, `cmds/termfit/termfit.png` is
  `data/fit_2021-11-26.png`; `cmds/termfit/bonddata.csv` (duplicate of `data/bonds_2021-11-26.csv`) is deleted and
  the `pkg/fit` tests read the data file.

### Migration: old command to `fic`

| Old | New |
|---|---|
| `bonds-cli -f F -settlement D -maturity M -coupon C -n N -quote P -redemption R -spread S -daycount X` | `fic bond price --curve F --date D --maturity M --coupon C --freq N --price P --redemption R --spread S --daycount X` (`-quote 0` = no price) |
| `swaprate-cli -f F -s S` | `fic swap rate --curve F --spread S` |
| `option-cli -type T -s S -k K -t T -q Q -vola V -r R` | `fic option price --type T --spot S --strike K --expiry T --div Q --vol V*100 --rate R` (`-vola` was a decimal, `--vol` is percent) |
| `termfit -file F -date D -settle-lag N -onrate R -f START -min-ttm Y -lambda L -huber H -objective O -model M` | `fic curve fit --bonds F --date D --settle-lag N --onrate R --start START --min-ttm Y --lambda L --huber H --objective O --model M -o curve.json --residuals residuals.csv` (`termfit` wrote `nss_opt.json`, `spline_opt.json`, `result.csv` to the current directory and read `term.json` as a starting point if it existed; `fic` writes nothing unless `-o` / `--residuals` is given; `-lambda -1` = `--lambda gcv`) |
| `fwdcrv -f F -m M -t T` | `fic curve forward --curve F --tenor M --t T` (`fwdcrv` wrote `forward.csv` with the columns `x,SpotRate,FwdRate,Z,F`) |
| `holee -f F -n N -m M -s S -paths P -seed X -o O` | `fic sim zcb --model holee --curve F --steps N --horizon M --sigma S*100 --paths P --seed X -o O` (`-s` was a decimal, `--sigma` is percent; the old CSV was `t,zero_rate,fwd_rate,theta,exp_short_rate,discount,mc_discount,mc_stderr,mc_zero_rate` with decimals; `-seed 0` is time based in both) |
| `FI_CURVE=F` | `FIC_CURVE=F` |

### Output format changes (relative to the removed tools)

- `fic bond price` / `fic swap rate` print labels and columns as documented in the README instead of the old
  human-readable layout of `bonds-cli` and `swaprate-cli`. The old "Implied spread" is `spread_total_bp`. Duration
  and convexity are evaluated at the market price (on the curve shifted by the z-spread) when a quote is given
  (the old tool used the duration of the unshifted curve). `--spread` is added to the spread of the curve file (it
  replaced it before).
- `fic option price` uses `--vol` in percent and reports `vega_per_volpt`, `theta_per_day`, `rho_per_pct` and
  `phi_per_pct`; `option-cli` printed the raw Greeks per 1.00 and took `-vola` as a decimal.
- `fic curve fit`, `fic curve forward` and `fic sim zcb` write the CSV formats described in the README, not the
  legacy files and columns of `termfit`, `fwdcrv` and `holee`.

## WP6: fetch yield-curve parameters from central banks

- New command `fic curve fetch --source snb|ecb [--date D|latest] [--file FILE|-] [--ecb-issuers aaa|all]
  [--max-age DAYS] [--timeout 30s] [--spread BP] [-o FILE]`: writes the published NSS parameters as a curve file
  (SNB: CHF, `SNB rendopar`; ECB: EUR, `ECB AAA` / `ECB all issuers`). A warning on stderr if the latest available
  as-of date is older than `--max-age` days (default 14, 0 disables): the SNB cube `rendopar` ends on 2025-07-31.
- New public package `pkg/source`: `ParseSNB`, `ParseECB` (offline, any `io.Reader`) and `Client` with `FetchSNB`,
  `FetchECB`. It is the only package with network access. Parsers find columns by name, accept BOM, CRLF,
  lower and upper case codes and a preamble, reject duplicates and non-numeric values with the line number.
  The client has a timeout, a size limit, https-only redirects and a User-Agent. SNB downloads are restricted with
  `fromDate`/`toDate` (supported by the SNB API), ECB with `startPeriod`/`endPeriod`/`lastNObservations`.
  The ECB parameters are validated against the ECB spot rates SR_1Y/SR_5Y/SR_10Y (agreement 1e-10 percent).
- `scripts/snb.zsh` is now a deprecated wrapper of `fic curve fetch --source snb` (same options `-f`, `-o`);
  `scripts/lib/snb.awk` is removed. The SNB codes are lower case in the live file (`b0`..`t2`); both cases work.
- `term.Marshal` rounds all NSS numbers (betas in percent and the decay parameters) to 14 significant digits
  (3.14724 instead of 3.1472399999999996). This affects curve files written by `fic curve fit` as well;
  differences are <= 5e-14 relative.
- Robustness: `ParseECB` rejects files that mix several values of INSTRUMENT_FM, CURRENCY or REF_AREA (and
  non-EUR data); numbers must be plain decimals (no hex floats, underscores, Inf); |beta| > 1000 percent or
  tau > 1000 years is an error, |beta| > 100 or tau > 100 a warning. `Curve.Notes` reports a skipped newer
  incomplete date (`skipped newer incomplete date X (missing t2)`). Errors show host and path, not the query.
- `fic curve fetch --strict`: with `latest`, exit 1 and no output if the as-of is older than `--max-age` days
  (default: warn only). The stderr summary carries `[STALE: N days]` then. Warnings and the summary are printed
  only after the output was written. `--spread` is limited to +-100000 bp. With `--file` the stale warning says
  `data in FILE is from DATE (N days old)`.
- `scripts/snb.zsh`: the `source` of the written curve changed from `SNB` to `SNB rendopar`, and the file has
  `"version": 1`; the script exits 1 if `fic` is not found and 2 for `-f ""`.
- Network notes: no retries, `Retry-After` is not honoured, proxies from the environment are used, https
  redirects to other hosts are followed (at most 5).

## WP5: unified CLI, scripts, R

One binary `fic` (`cmds/fic`, implementation in `internal/cli`) replaces the
six apps with consistent flags, units, formats and exit codes. The old
apps are deprecated shims (`internal/cli/legacy.go`, tiny `main.go`) that
keep their old flags and print `deprecated: use "fic ..."` to stderr.

The binary is called `fic` (not `fi`, a reserved word in bash and zsh); the
environment variable is `FIC_CURVE` (`FI_CURVE` is still read, with a
deprecation warning on stderr, when it is the only one set).

### Output format changes (legacy shims)

There is no `--legacy-output` switch.

- `bonds-cli` and `swaprate-cli` print the text output of `fic bond price` and
  `fic swap rate` instead of the old layout. The old "Implied spread" is
  `spread_total_bp` (file spread + `-spread` + z-spread, e.g. 155.4 for
  `-spread 50 -quote 101 -n 2` of the old tool). Duration and convexity are
  evaluated at the market price (on the curve shifted by the z-spread) when a
  quote is given, not on the curve.
- `option-cli` keeps its old text output and the raw units of the Greeks;
  `-vola` stays a decimal (warning for `-vola` > 5). `fic option price` reports
  `vega_per_volpt`, `theta_per_day`, `rho_per_pct` and `phi_per_pct`.
- `termfit`, `fwdcrv` and `holee` keep their legacy files and formats.

### Safety and robustness

- `-o FILE` is atomic (temporary file in the same directory, renamed on
  success); a failure never damages or deletes an existing file; `-o` equal to
  an input file is a usage error (exit 2).
- `fic bond price --in`: a row with an empty or `N/A` price is valued on the
  curve (`price_source` = `model`); failed rows echo the raw input fields;
  stderr names the first failed ids. Output columns `price_source` and
  `spread_total_bp` were added.
- CSV output guards text fields starting with `= + - @`, tab or CR with an
  apostrophe; negative zero is printed as 0; a `--spread` that makes the
  discount factors under- or overflow is an error.
- `swap rate` has no `--date` flag (swaps start at the curve date).
- A terminal is a character device other than `/dev/null`.

### Migration (old flag -> new flag)

| Old | New |
|---|---|
| `bonds-cli -f F` | `fic bond price --curve F` (env `FIC_CURVE`, default `./curve.json`) |
| `-settlement D` / `-maturity M` / `-coupon C` (%) | `--date D` / `--maturity M` (date or `5y`) / `--coupon C` (%) |
| `-n N` | `--freq N` (1, 2, 3, 4, 6, 12; invalid values exit 2 with the valid list) |
| `-quote P` (0 = none) | `--price P` (clean, percent of face) or `--yield PCT` (cc) |
| `-redemption R`, `-daycount X`, `-spread S` | `--redemption R`, `--daycount X`, `--spread S` |
| `swaprate-cli -f F -s S` | `fic swap rate --curve F --spread S [--tenors ...] [--freq 2] [--fwd-curve F2]` |
| `option-cli -type -s -k -t -q -vola -r` | `fic option price --type --spot --strike --expiry --div --vol --rate` (`-vola 0.3` is `--vol 30`; `-t 2` is `--expiry 2y`; Greeks in `vega_per_volpt`, `theta_per_day`, `rho_per_pct`, `phi_per_pct`) |
| `termfit -file -date -settle-lag -onrate -f -min-ttm -lambda -huber -objective -model` | `fic curve fit --bonds --date --settle-lag --onrate --start --min-ttm --lambda (gcv\|X) --huber --objective --model -o --residuals` |
| `fwdcrv -f -m -t` | `fic curve forward --curve --tenor --t` |
| `holee -f -n -m -s -paths -seed -o` | `fic sim zcb --model holee --curve --steps --horizon --sigma --paths --seed -o` (`-s 0.02` is `--sigma 2`) |
| `analysis.R` (termfit, fwdcrv, holee) | `r/curve.R`, `r/sim.R` (removed from `cmds/`) |

### Behaviour changes

- Units at the command line: all rates, yields, vols and dividend yields are
  percent, spreads bp, dates ISO (CSV inputs also `DD.MM.YYYY`); `fic` has no
  decimal flags (the shims keep `holee -s` and `option-cli -vola` decimal).
- `--spread` is **added** to the `spread_bp` of the curve file (`bonds-cli`
  and `swaprate-cli` replaced it).
- `--date` defaults to the `asof` of the curve file (else today); a
  differing `--date` gives a warning.
- `bond price`: reports `yield_cc_pct` and `yield_annual_pct`; the z-spread is
  relative to the effective curve (file spread plus `--spread`) and
  duration, convexity and DV01 are evaluated on the curve shifted by the
  z-spread (at the market price); all prices per 100 of face; a bond that is
  matured on the settlement date exits 1 (`bonds-cli` exited 1 via
  `log.Fatal`, too); batch mode with a per-row `status`.
- `curve fit`: stdout is the curve JSON (not a summary), the summary and
  warnings go to stderr; nothing is written to the current directory;
  `--model both` writes `<stem>.nss.json` and `<stem>.spline.json`; the
  curve file given as `--start` must exist and be an NSS curve (the old
  `-f` implicitly read `term.json` and ignored errors); rows with errors are
  skipped with a warning; bond files accept `coupon_pct`/`price_clean`
  headers (and the old `coupon`/`price`), CRLF, BOM, quoted fields and
  `N/A` prices; the residual file has the old `result.csv` columns.
- `sim zcb`: `--seed` defaults to 1 (deterministic); the output has the
  columns `t_years, curve_zero_cc_pct, curve_fwd_cc_pct, curve_discount,
  model_discount, mc_discount, mc_stderr, mc_zero_cc_pct, z_score` (rates in
  percent); the `theta` and `exp_short_rate` columns are only written by the
  `holee` shim; Hull-White and Vasicek are available (`--model`).
- `swap rate`: tenor list, fixed frequency and dual curve (`--fwd-curve`);
  `par_rate_pct` is the annualised fixed-leg rate, the annuity is
  `sum Z(t_i)/freq`.
- `option price`: expiry as years, tenor or date; all Greeks including theta
  and phi; implied volatility with `--implied-from`; curve (`--curve`) or flat
  rate (`--rate`); invalid input exits 2.
- Shims print the output of `fic` (text format) for `bonds-cli`,
  `swaprate-cli` and `option-cli`; `termfit` (`nss_opt.json`,
  `spline_opt.json`, `result.csv`), `fwdcrv` (`forward.csv`) and `holee`
  (decimal CSV) keep their legacy files.
- Scripts: `snb.zsh` writes the new curve schema (header-aware, latest
  complete date), `six-bonds.zsh` writes the bond CSV (missing price = empty
  field), `price-bonds.zsh` and `avg-spread.zsh` (removed again, see "Analysis in fic") are thin wrappers of
  `fic bond price` (duration-weighted average z-spread per issuer by column
  name, rows with `status != ok` skipped); `scripts/test.zsh` tests them with
  fixtures.
- Samples: `data/` (SNB curve 2021-03-31, bonds 2021-11-26), regenerated
  `cmds/termfit/result.csv` (new format) and `termfit.png`;
  `data/bonds_2021-11-26.csv` is `cmds/termfit/bonddata.csv` in the new bond CSV format (the original stays for the
  termfit shim and the `pkg/fit` tests).

## WP4: Monte Carlo (breaking)

`pkg/mc` has been rewritten: a model is discretised on an explicit time
grid, paths are simulated exactly (no Euler bias), the engine is parallel,
seedable and allocation-free per path, and the estimates come with honest
standard errors.

### Design

- `mc.Grid` has N+1 nodes `t_0 = 0, ..., t_N = T` (`NewUniformGrid(T, n)`,
  `NewGrid(times)`); previously the paths had N points and stopped at
  `T - dt`.
- `mc.Model.Discretize(grid) (Simulator, error)` precomputes the grid
  coefficients; `Simulator.Simulate(z, path)` maps `Dim()` standard
  normals to a `mc.Path{Grid, State [][]float64, Df []float64}` with the
  pathwise discount factor `Df[i] = exp(-∫_0^{t_i} r ds)`. Negating `z`
  gives the antithetic path.
- `mc.Price(model, payoff, grid, opts)`, `mc.PriceMulti` (vector payoffs,
  e.g. `mc.ZeroBonds` for the whole model curve) and `mc.PriceAmerican`
  (Longstaff-Schwartz) return `mc.Result{Mean, StdErr, N, Paths, Beta}`
  with `CI95()`.
- `mc.Options{Paths, Workers, Seed, Antithetic, Control, TrainingPaths}`:
  the samples are split into blocks of `mc.BlockSize` with one xoshiro256**
  stream per block derived from `(Seed, block)`; results are identical for
  any number of workers (default `GOMAXPROCS`). Welford/Chan accumulation,
  variance with n-1. The block statistics are merged in block order as
  soon as all earlier blocks are done (reorder window of 2×Workers
  blocks), so the memory does not grow with the number of paths
  (`PriceMulti` with `ZeroBonds`, N = 1000: 1.8 / 15.7 / 58.7 MB for
  2^14 / 2^18 / 2^20 paths before, 1.8 / 4.6 / 4.5 MB now).
- Control variates: β is estimated on a pilot sample (the first
  min(256, samples/10) samples, excluded from the estimate), so the
  estimator is unbiased and its standard error honest for small samples;
  at least `mc.MinControlSamples` (30) samples are required.
- Panics in simulators, payoffs, controls and exercise functions are
  returned as errors (`mc.ErrPanic`); `PriceMulti` rejects a `ZeroBonds`
  on another grid.
- Short rate models (`holee`, `hullwhite`, `vasicek`) share an exact
  simulation of `(x, ∫x)` for the OU factor (2 normals per step); the
  deterministic part of `∫r` is computed exactly on the nodes, so
  `E[Df[i]]` equals the model bond price for every grid (Ho-Lee and
  Hull-White: the curve; σ = 0: to machine precision).
- `stock.GBM` simulates `ln S` exactly with the forward rates of a curve
  (`Df[i] S_i` is a martingale on the grid) and a dividend yield `Q`.

### Migration

| Old API | New API |
|---|---|
| `mc.New(model, nsim)`, `e.Run()`, `e.Estimate()`, `e.StdError()`, `e.CI()` | `res, err := mc.Price(model, payoff, grid, mc.Options{Paths: nsim, Seed: s})`; `res.Mean`, `res.StdErr` (n-1), `res.CI95()` |
| `mc.Model{Measurement() float64}` | `mc.Model{Discretize(*Grid) (Simulator, error)}` + `mc.Payoff{Value(*Path) float64}` (`mc.PayoffFunc`) |
| `Engine.Estimates` (all samples stored) | not stored (streaming moments, memory independent of `Paths`) |
| `model.Rng = rand.New(...)` (time-seeded by default) | `Options.Seed` (deterministic, worker-independent); `mc.NewRand(seed, stream)` |
| payoff `func(rates []float64) float64` with N points `0..(N-1)dt`, discounting by summing `rates[i]*dt` | `func(p *mc.Path) float64` on N+1 nodes `0..T`, discount with `p.Df[i]` |
| `holee.New(ts, sigma, T, N, payoff)`; fields `R0, Theta, Rng, Payoff`; `holee.Calibrate` | `holee.New(curve, sigma) (*HoLee, error)` (calibration implicit and exact); `R0()`, `Theta(t)`, `Phi(t)`, `Z(r, t, T)`, `Discretize(grid)` |
| — | `hullwhite.New(curve, a, sigma)`: arbitrage-free Hull-White with `Z(r, t, T)`, `Theta`, `Phi`, `IntPhi` |
| `vasicek.New(ts, sigma, T, N, payoff)` (calibrates r̄, γ silently) | `vasicek.New(r0, rbar, gamma, sigma)`; `vasicek.Calibrate(curve, sigma, maturities) (*Vasicek, Calibration{RMSE, Evaluations, AtBound}, error)` (R0 from the curve) |
| `vasicek.Calibrate(v, ts)` | `vasicek.Calibrate(curve, sigma, maturities)` |
| `v.Z(r, t, T)` (singular for γ -> 0) | `v.Z(r, t, T)` (γ >= 0, no singularity), `v.ZeroRate(T)`, `v.Mean(t)` |
| Vasicek Euler scheme | exact OU transition and exact joint `∫r` |
| `stock.New(r, s0, sigma, T, N, payoff)` (R: "return") | `stock.New(s0, sigma, q, curve)`; the drift is the risk-free forward rate of the curve minus the dividend yield `q` |
| payoff closures for options | `stock.European{Type, Strike}`, `stock.Asian{Type, Strike, Geometric}`, `stock.American{Type, Strike, Degree, Model}` (`mc.Exercisable`; use `m.American(type, K)`) |
| — | `m.European(type, K, T)` (Black-Scholes), `m.GeometricAsian(type, K, grid)` (closed form), `m.TerminalControl(grid)`, `m.EuropeanControl(type, K, grid)`, `m.GeometricAsianControl(type, K, grid)` |
| — | `mc.PriceAmerican(model, exercisable, grid, opts)` → `AmericanResult{Result (out-of-sample), InSample, InSampleStdErr, TrainingPaths}`; optional `mc.ContinuationBounder`, `mc.Validator` |
| `cmds/holee` writes `./result.csv` without header (committed sample `cmds/holee/result.csv`, removed); the Monte Carlo part integrated the rates only up to `T - dt` | `holee -f term.json -n 1000 -m 10 -s 0.02 -paths 10000 -seed 0 -o holee.csv` (CSV with header to stdout or `-o`; summary and warnings on stderr); `Rscript analysis.R holee.csv holee.png` |

### Intentional behaviour changes

- Ho-Lee: the old calibration used forwards lagged by one step and
  integrated the rates up to `T - dt`; the zero bond price was biased by
  O(dt) even for σ = 0. Exact expectation of the old scheme (NSS curve of
  the tests): T = 30, N = 50, σ = 0.02: 100.48 instead of 106.09 (-5.61);
  T = 10, N = 1000, σ = 0.02: -0.030; σ = 0, T = 30, N = 50: -0.37. New:
  exact for σ = 0 (error < 1e-12), unbiased for σ > 0 (2^20 antithetic
  paths: 106.131 ± 0.592 vs 106.088).
- Vasicek: the old calibration (NSS curve of the tests, σ = 0, T = 5,
  N = 1260, fit up to 1.5 T) ran away to r̄ = 122.5 with γ ≈ 0 (singular
  parametrisation; σ = 0.01: r̄ = 16.05). Now (κ = γ r̄, bounded γ) with
  the RMSE reported; Hull-White is the arbitrage-free alternative.
- American options (Longstaff-Schwartz): the exercise rule only exercises
  if the intrinsic value is at least the regression estimate and at least
  a known lower bound of the continuation value (`mc.ContinuationBounder`;
  for `stock.American` with a model: the European value of the remaining
  option, which is also a basis function). Without it a call without
  dividends was sometimes exercised early (LSM below the European price by
  more than 4 SE for 3 of 60 seeds); now it equals the European Monte
  Carlo price exactly. Default basis degree 3.
- Units: all model parameters are decimals; the initial short rate is set
  from the curve (`f(0,0)`), not passed in.
- `example/range_floater` reports four variants (flat or model LIBOR ×
  LIBOR roll-up or pathwise discounting); 200000 paths, seed 1: flat+roll
  (Veronesi) 99.8116, flat+Df 99.5759, model+roll 99.6022, model+Df
  (consistent) 99.6070 (book 99.8385 ± 0.0505). The gap comes from
  discounting at the flat-converted LIBOR, see the example.

## Unreleased: core redesign (breaking)

The core of the library (curves, schedules, cash flows, instrument
valuation) has been redesigned. The API is not backwards compatible.

### Conventions

- All rates, yields, spreads, betas, volatilities and dividend yields are
  **decimals** inside the library (`0.02` = 2%, `0.005` = 50bp). Percent and
  basis points are only used at the I/O boundary (curve files, command line
  flags).
- Rates are **continuously compounded** everywhere. `pkg/rate` converts to and
  from n-times compounded and simple rates (`cashflow.Yield` reports the
  annually compounded yield).
- The time axis for discounting is **ACT/365 Fixed** (calendar days / 365 from
  the settlement date) for all instruments, independent of the day count
  convention of accrued interest. Dates are normalised to UTC midnight.
- Curves are immutable and safe for concurrent use. Spreads are applied with
  `term.WithSpread`, which returns a new curve.
- Every fixed-income instrument reduces to cash flows (`cashflow.CashFlow`);
  analytics operate on cash flows.
- Invalid user input yields errors instead of panics.

### Migration

| Old API | New API |
|---|---|
| `term.Structure` | `term.Curve` (`Discount(t)`, `ZeroRate(t)`) |
| `ts.Z(t)` | `c.Discount(t)` |
| `ts.Rate(t)` (cc, percent) | `c.ZeroRate(t)` (cc, decimal) |
| `ts.SetSpread(bp)` / `GetSpread()` | `term.WithSpread(c, bp/1e4)` / `term.Spread(c)` |
| `term.Flat{R: 2.0, Spread: 10}` | `term.WithSpread(term.Flat{Rate: 0.02}, 0.001)` |
| `term.NelsonSiegelSvensson{B0..B3 (%), T1, T2, Spread}` | `term.NewNSS(b0..b3 (decimal), tau1, tau2)` or `term.NSS{B0, B1, B2, B3, Tau1, Tau2}` + `Validate()` |
| `term.NewSpline(t, z, spread)` (panics; cubic spline on DFs) | `term.NewSpline(t, z) (*Spline, error)` (natural cubic spline on log-DFs, anchored at (0,1)) |
| `term.Spline{...}.Init()`, `term.Initer` | removed; use `term.NewSpline` |
| — | `term.Forward(c, t1, t2)`, `term.InstForward(c, t)` |
| `term.Parse(data) (Structure, error)` | `term.Parse(data) (Curve, Meta, error)`; new schema with `"model"` (case-insensitive), optional `"version": 1`, `"comment"` and `"x-..."` extension keys; legacy format still parsed |
| `json.Marshal(ts)` | `term.Marshal(c, meta)` |
| `rate.Continuous(i%, n)` | `rate.ToContinuous(i, n) (float64, error)` |
| `rate.Annual(r%, n)` | `rate.FromContinuous(rc, n) (float64, error)` |
| `rate.EffectiveAnnual(i%, n)` (growth factor) | `rate.EffectiveAnnual(i, n)` (effective rate, `(1+i/n)^n - 1`) |
| — | `rate.ToSimple`, `rate.FromSimple` |
| `maturity.Schedule{Settlement, Maturity, Frequency int, Basis string}` | `maturity.Schedule{Issue, FirstCoupon, Maturity, Frequency maturity.Frequency, DayCount maturity.DayCount, EOM maturity.EOMRule}`; the settlement date is passed to the methods |
| `Basis: "30E360"` (string) | `DayCount: maturity.Thirty360E` (`Eurobond`, `BondBasis`, `Act360`, `ActAct`); `maturity.ParseDayCount(name)` accepts the daycount names and aliases such as `"30E/360"`, `"ACT/360"` |
| — | `maturity.ParseFrequency`, `maturity.DayCountNames`, `maturity.EOMAuto/EOMOn/EOMOff` |
| `Frequency: 0` (= annual) | `maturity.Annual` (0 is invalid, see `Frequency.Validate`) |
| `s.M()` (descending, basis-dependent years) | `s.CouponDates(settle) ([]time.Time, error)` (ascending dates), `s.Periods(settle) ([]Period, error)`, `s.PeriodAt(settle) (Period, error)`, `maturity.YearFraction(settle, d)` |
| `s.Last()`, `s.Next()` | `maturity.YearFraction(settle, s.Maturity)`, `s.NextCoupon(settle) (time.Time, error)` (`maturity.ErrMatured`) |
| `s.DayCountFraction()` | `p, _ := s.PeriodAt(settle); s.AccrualFraction(p.Start, settle, p.End)` (year fraction, with error) |
| `s.Compounding()`, `s.EffectiveCoupon(c)` | `int(s.Frequency)`, `s.CouponFraction(period) (float64, error)` |
| `maturity.DaysInYear`, `LastDay`, `FirstDay`, `DifferenceInYears` | removed (`maturity.YearFraction`, `maturity.Date`) |
| `fixedincome.Security`, `TermSecurity`, `Option` | `cashflow.Instrument` (`CashFlows(settle)`: Straight) and `cashflow.Projected` (`CashFlows(settle, forecast)`: Floating, InterestRateSwap, FRA) |
| `b.PresentValue(ts)` (bond) | `b.PresentValue(settle, c)` = `b.DirtyPrice(settle, c)`, or `cashflow.PV(cfs, c)` |
| `b.PresentValue(ts) - b.Accrued()` | `b.CleanPrice(settle, c)` |
| `b.Accrued()` | `b.Accrued(settle) (float64, error)` |
| `b.Duration(ts)` (negative) | `cashflow.Duration(cfs, c)` (**positive** modified duration) |
| `b.Convexity(ts)` | `cashflow.Convexity(cfs, c)` |
| `fixedincome.Irr(dirty, s)` (cc, percent) | `cashflow.YieldCC(dirty, cfs)` (cc, decimal); `cashflow.Yield(dirty, cfs, n)` |
| `fixedincome.Spread(dirty, s, ts)` (bp, mutates ts) | `cashflow.ZSpread(dirty, cfs, c)` (decimal, no mutation) |
| `fixedincome.PVBP(s, ts)` | `cashflow.DV01(cfs, c)` (full revaluation, negative for a long bond) |
| `fixedincome.InterestSensitivity(dr, s, ts)` | `cashflow.Sensitivity(cfs, c, dr)` (`-D dr + 1/2 C dr²`) |
| — | `cashflow.KeyRateDurations(cfs, c, keys, bump)` |
| — | sentinel errors `cashflow.ErrNoYield`, `cashflow.ErrNoCashFlows`, `maturity.ErrMatured`, `maturity.ErrSettlement`, `bond.ErrNoFixing`, `swap.ErrNoFixing` |
| — | `term.Parametric` (NSS: `Params`, `WithParams`, `ZeroRateGrad`, `DiscountGrad`), `term.DiscountGrid`, `term.MinKnotSpacing` |
| `bond.Straight{Schedule, Coupon (%), Redemption}` | `bond.NewStraight(schedule, coupon (decimal), bond.WithFace(f), bond.WithRedemption(r))` (Face 100 and Redemption = Face by default, only via the constructor); struct literals must set `Face > 0` and `Redemption >= 0` explicitly |
| `bond.Floating{Schedule, Rate (%), Redemption}`; `PresentValue(ts)` | `bond.NewFloating(schedule, margin, bond.WithFixing(r), ...)` / `bond.Floating{Schedule, Margin, CurrentFixing *float64, Face, Redemption}`; `PresentValue`/`DirtyPrice`/`CleanPrice(settle, discount, forecast)`, `CashFlows(settle, forecast)`, `Accrued(settle)` |
| `f.Duration(ts)`, `f.Convexity(ts)` (FRN) | `bond.FloatingDuration(f, settle, disc, fwd)`, `bond.FloatingConvexity(...)` (both curves shifted) |
| `swap.InterestRateSwap{Floating, Fixed}` (bond legs) | `swap.InterestRateSwap{Notional (> 0), FixedRate, FixedSchedule, FloatSchedule, Margin, CurrentFixing *float64, Payer}`; `PresentValue`, `CashFlows` (net, signed), `ParRate`, `Annuity`, `FloatLeg` |
| `swap.InterestRate(times, n, ts)` (percent) | `swap.ParRateFromTimes(times, n, c)` (decimal) |
| `swap.FxRate(fx, cfs, times, tsLong, tsShort)` | `swap.FxRate(fx, cfs, times, domestic, foreign)` |
| `forward.RateAgreement{N, M, T1, T2}` | `forward.NewFRA(notional, fixed, start, end, payer)` (Act360) / `forward.FRA{Notional (> 0), FixedRate, Start, End, DayCount maturity.DayCount, Payer}`; `PresentValue(settle, disc, fwd)`, `CashFlows(settle, fwd)`, `ForwardRate`, `AccrualFraction`; ActAct is rejected |
| `forward.ZeroBondPrice(t, m, ts) (float64, error)` | `forward.ZeroBondPrice(t, m, c) float64` |
| — | `forward.FxPrice`, `forward.AssetPrice` |
| `option.European{Type int, S, K, T, Q (%), Vola}` | `option.European{Type option.Type, S, K, T, Vol, Q (decimal)}` |
| `option.Call = 0`, `option.Put = 1` (int) | `option.Call = 1`, `option.Put = 2` (typed `option.Type`; the zero value is invalid); `option.ParseType("call")` |
| `opt.PresentValue(ts)` (panics for invalid type) | `opt.Price(c) (float64, error)` |
| `opt.Delta/Gamma/Rho/Vega(ts)` | `opt.Greeks(c) (Greeks, error)` with `Delta, Gamma, Vega, Theta, Rho, Phi` |
| `fixedincome.ImpliedVola(price, o, ts)` (mutates o) | `option.ImpliedVol(price, opt, c)` (no mutation, bracket [1e-6, 5]; `option.ErrNoImpliedVol`, `option.ErrNoVega`) |
| `option.D1(S, K, T, Q%, vol, ts)` | `option.D1(S, K, T, r, q, vol)` |
| `holee.New(ts, ...)`, `vasicek.New(ts, ...)` | redesigned in WP4, see "WP4: Monte Carlo" below |

### Intentional behaviour changes

- **Time axis**: discounting times are ACT/365F for all day count conventions
  (previously the day count convention of the bond, e.g. 30E/360 years). For
  the README bond (1.25% CH0224396983, settle 2021-04-17) the dirty price
  changes by +0.0003, the yield by +0.00015% and the z-spread by +0.005bp; for
  CH0193265995 (settle 2021-04-15) the z-spread changes from 24.82bp to 24.90bp.
- **Accrued interest**: for the 30/360 family and ACT/360 it is
  `coupon × daycount.Days(prev, settle)/360` (previously
  `days/periodDays/frequency`, which is wrong for EUROBOND/BONDBASIS periods
  ending at the end of February and for irregular ACT/360 periods). ACT/ACT
  ("ACTACT") is ACT/ACT ICMA (`coupon/freq × days/periodDays`), with the
  notional period for a short first coupon. No change for regular 30E360
  periods (README bond: identical).
- **Duration sign**: modified duration is positive (`-1/P dP/dy`).
- **Spline**: natural cubic spline on the log-discount factors, anchored at
  (0, 1), with flat instantaneous forward extrapolation beyond the last knot
  (previously a cubic spline on the discount factors with flat zero-rate
  extrapolation below the first knot). On the same knots as the termfit
  sample fit the zero rates differ by at most 1bp up to 23y.
- **Settlement on a coupon date**: the coupon is not received (cash flows
  strictly after the settlement date) and nothing is accrued; a bond settled
  on its maturity date has no cash flows and a value of 0.
- **FRN / swap**: coupons are projected from a forecast curve and discounted
  on the discount curve (previously "par at reset" was assumed, even with a
  z-spread). `CurrentFixing` (now `*float64`) is used for the period
  containing the settlement date, including a period starting on it; if it
  is nil, a period starting on the settlement date is projected (so the FRN
  is at par with no rate duration) and a period in progress is an error
  (`ErrNoFixing`). The index is an IBOR-style term rate; compounded-in-arrears
  overnight rates (SARON, SOFR, €STR) are not modelled.
- **ACT/360 bonds**: the coupon is `coupon × days/360` of the period (equal
  to the accrual, no clean price jump on coupon dates; a 5% semiannual bond
  with a 182-day period jumped by 0.0092 before). 30/360 and ACT/ACT ICMA
  regular coupons stay `coupon/frequency`.
- **Long first coupon**: `Schedule.FirstCoupon` (with `Issue`) defines a
  short or long first period; ACT/ACT ICMA accrual runs over the notional
  periods.
- **Yield / z-spread**: a non-positive price for non-negative cash flows (or
  vice versa) returns `ErrNoYield` instead of a spurious root (e.g.
  `YieldCC(0, {30y: 100})` returned 50).
- **Yield(n)** is the n-compounded equivalent of the continuously compounded
  ACT/365F yield, not the ICMA street yield.
- **Validation**: invalid schedules, zero settlement dates (or more than 200
  years before maturity), non-positive face values or notionals, negative
  redemptions and invalid NSS parameters return errors (or NaN for curve
  evaluation) instead of panics or silent defaults. Zero-value and nil
  splines return NaN; spline knots must be at least 1e-8 years apart.
- **EOM rule**: inferred if the maturity is the last day of its month,
  including 28 February in a non-leap year (use `maturity.EOMOff` to keep
  the 28th).
- **Root finding**: own Brent solver (`internal/solve`); roots at bracket
  endpoints are returned, NaN and non-bracketing intervals are skipped. Yields
  use Newton's method with a bracketed fallback up to very high yields.
- **Implied volatility** is searched in [1e-6, 5] (previously [1e-6, 1000]).

### Upstream github.com/konimarti/daycount (not patched here)

- 30E360: the last day of February is converted to 30 even if it is the
  termination date (FIXME).
- No ACT/365F or ACT/ACT ISDA conventions.
- `Implemented()` returns the names in random order.
- `Fraction()` is wrong for EUROBOND/BONDBASIS February-end periods and uses
  a 30E/360 denominator for ACT360 (not used by this library any more).

### Curve fitting (`pkg/fit`, `termfit`)

- New package `pkg/fit`: `ReadBondsCSV`, `PrepareBonds` (cash flows,
  accrued interest and market yields computed once; matured bonds and bonds
  below `Options.MinTTM`, default 0.1y, are dropped with a warning),
  `FitNSS` and `FitSpline` returning a `Result` with per-bond residuals
  (clean/dirty prices in percent of face, continuously compounded and annual
  yields, error in bp), RMSE, convergence flag and warnings. The zero values
  of `Options` are documented in a table; `Options.Lambda` is a `*float64`
  (nil = generalized cross-validation, `fit.Lambda(0)` = no smoothing).
- Quoted prices are percent of face for any `Bond.Face` (the dirty price is
  `CleanPrice*Face/100 + accrued`). The settlement date is the curve date
  (no settlement lag is applied).
- The objective remains the squared **yield** errors (continuously
  compounded). Model yields are solved by Newton's method on the log price
  with a warm start (always converges, no penalty discontinuity); the
  Jacobian uses the implicit function theorem. `Objective: ObjectivePrice`
  is available for comparison; `WeightInvDuration` weights by 1/(D*P).
  `Options.Huber` (threshold in bp) gives a robust Huber fit by IRLS.
- NSS: deterministic multi-start over a grid of decay parameters (betas
  solved per grid point), the best starts of each tau ordering polished by a
  bound-constrained (active-set) Levenberg-Marquardt with the analytic
  Jacobian; tau in [0.1, 30] years, soft separation of tau1 and tau2 (ratio
  >= 1.25), light ridge on B2 and B3; the overnight constraint `B0 + B1 = on`
  is exact (B1 eliminated). Warnings for taus at a bound, an active
  separation constraint, beta blow-ups, a poorly determined long end (B0),
  alternative local solutions within 5% of the objective, outliers (> 4 x
  RMSE) and implausible market yields (> 25%).
- Spline: zero rates at 5-8 knots between the shortest and longest maturity
  (a blend of maturity quantiles and a logarithmic grid) plus a short knot,
  smoothing penalty on the second differences of the zero rates, selected by
  generalized cross-validation by default.
- `fit.OvernightToCC` converts a simple ACT/360 overnight rate to the
  continuously compounded rate on the ACT/365F axis (365 ln(1 + r/360)).
- `termfit` is a thin wrapper over `pkg/fit` with the new flags `-min-ttm`,
  `-lambda`, `-huber`, `-settle-lag`, `-objective yield|price` and
  `-model nss|spline|both` (models are fitted independently).
  `result.csv` now has a header and named columns (`model, id, maturity,
  ttm_years, price_clean_market, price_clean_model, yield_cc_market_pct,
  yield_cc_model_pct, yield_annual_market_pct, yield_annual_model_pct,
  error_bp, zero_cc_model_pct, discount_factor_model`), sorted by time to
  maturity; `analysis.R` takes the input and output paths as arguments.
- Behaviour changes: bonds with less than 0.1 years to maturity are no
  longer used by default (`-min-ttm 0` keeps all); `-onrate` is converted
  with `365 ln(1 + r/360)` (previously daily compounding on a 360-day
  year), and an unset `-onrate` means no constraint while `-onrate 0`
  imposes 0%; a missing explicit `-f` file is logged and ignored.

### Dependencies

- Removed `github.com/cnkei/gospline` (not goroutine-safe) and
  `github.com/khezen/rootfinding` (returned 0 for roots at bracket endpoints).
