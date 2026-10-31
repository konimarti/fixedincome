# `fic` reference

Full reference of the command line tool. For a short introduction see the [README](../README.md).


```
fic curve fetch | fit | compare | show | forward | template     fic bond price     fic portfolio risk     fic swap rate     fic option price     fic sim zcb
fic version        fic help [command]
```

Flags work with one or two dashes (`--curve` is documented). `fic help <command>` lists the flags.

## Global conventions

| | |
|---|---|
| Units | rates, yields, vols, dividend yields in **percent**; spreads in **bp**; dates `YYYY-MM-DD` or `DD.MM.YYYY` (everywhere a date is read, output dates are ISO); maturities, tenors and horizons as dates or `6m`, `2y`, `10d`, `3w` |
| Curve | `--curve FILE` (env `FIC_CURVE`, default `./curve.json`; `-` is stdin). The curve date is the `asof` of the file. A `--spread` so large that the discount factors under- or overflow is an error (exit 1) |
| Date | `--date`: default the `asof` of the curve file, else today; a warning on stderr if `--date` and `asof` differ |
| Spread | the `spread_bp` of the curve file is kept, `--spread BP` is **added** to it |
| Formats | `--format text\|csv\|json`, `-o FILE` (default stdout). **Output files are atomic and non-destructive:** the data is written to a temporary file next to `-o` and renamed on success, an existing file is never damaged or deleted by a failing run, and `-o` equal to an input file (`--curve`, `--in`, `--bonds`, `--start`, `--fwd-curve`; compared with `os.SameFile`) is a usage error (exit 2). Default format for tables (`curve show/forward`, batch `bond price`, `swap rate`, `sim zcb`): text on a terminal, **csv** when stdout is not a terminal or `-o` is given (a terminal is a character device other than `/dev/null`; this is not a full `isatty` check); for single values (`bond price` of one bond, `option price`): **text** |
| CSV / JSON | header always, snake_case names with unit suffix (`ttm_years`, `price_clean`, `yield_cc_pct`, `zspread_bp`, ...), comma separated, ISO dates, numbers `%.10g` (negative zero is printed as `0`); text fields that start with `=`, `+`, `-`, `@`, tab or CR get a leading apostrophe in CSV (spreadsheet formula guard; numbers are untouched); JSON is one object (single value) or an array of objects (tables) with the same names; non-finite numbers are empty (csv) or `null` |
| Streams | stdout is data only; warnings, summaries and progress go to stderr; nothing is written to the working directory unless `-o` (or `--residuals`, `--report`) is given |
| Exit codes | `0` ok, `1` runtime or numerical error (unreadable file, matured bond, no yield found, all batch rows failed), `2` usage error (unknown command or flag, invalid value, missing required flag). Errors are one line `fic <command>: message` |

## Commands and flags

| Command | Flags (units) |
|---|---|
| `fic curve fetch` | `--source snb\|ecb` (required), `--date D\|latest` (default latest = the latest date with all six parameters), `--file FILE\|-` (parse a downloaded file, no network), `--ecb-issuers aaa\|all` (default aaa), `--max-age DAYS` (default 14; warn if the latest parameters are older, 0 = never), `--strict` (with `latest`: exit 1 and write no output if the data is older than `--max-age`), `--timeout 30s`, `--spread BP` (written as `spread_bp`, at most 100000 in absolute value), `-o FILE` (curve JSON, default stdout). Downloads the published NSS parameters (SNB: CHF, source `SNB rendopar`; ECB: EUR, `ECB AAA` or `ECB all issuers`) and writes a curve file with `asof`, `currency` and `source`; after the output was written, stderr gets warnings and a summary with the 1y/5y/10y zero rates (tagged `[STALE: N days]` when the data is older than `--max-age`). `--ecb-issuers` given explicitly must match the variant of a `--file`; files that mix several instruments, currencies or areas are rejected. Exit 1 for network or data errors (also a `--date` without a complete parameter set: the error names the nearest earlier date) |
| `fic curve fit` | `--bonds FILE\|-` (bond CSV), `--date D` (`YYYY-MM-DD` or `DD.MM.YYYY`, default today), `--model nss\|spline\|both` (default nss; `both` needs `-o` and writes `<stem>.nss.json`, `<stem>.spline.json`), `--onrate PCT` (overnight rate, simple ACT/360; constrains the short end), `--start CURVE` (NSS starting point), `--min-ttm Y` (default 0.1; <= 0 keeps all), `--lambda X\|gcv`, `--objective yield\|price`, `--huber BP`, `--settle-lag DAYS`, `-o FILE` (curve JSON, default stdout), `--residuals FILE` (per-bond CSV), `--report FILE` (HTML report, see below). The `asof` of the curve is the settlement date; `source` the bond file. Warnings of the fit go to stderr |
| `fic curve compare` | `A.json B.json` (two curve files, flags may come before, between or after them), `--grid start:end:step` (default `1m:10y:1m`) or `--t 1y,2y,5y`, `--format`, `-o` (not equal to A or B). The curves are used as stored (including their `spread_bp`), there is no `--spread`. Columns `t_years, a_zero_cc_pct, b_zero_cc_pct, diff_zero_bp, a_fwd_cc_pct, b_fwd_cc_pct, diff_fwd_bp`: the instantaneous forward rate `fwd_cc`, differences are **B minus A** in bp. stderr gets a warning if the `asof` dates differ and a summary of `diff_zero_bp` (mean, maximum absolute value with its maturity, RMS) |
| `fic curve show` | `--curve`, `--grid start:end:step` (default `1m:10y:1m`) or `--t 1y,2y,5y`, `--spread BP`, `--format`, `-o`. Columns `t_years, zero_cc_pct, zero_annual_pct, discount_factor, inst_fwd_cc_pct` |
| `fic curve forward` | as `show` plus `--tenor 1y`. Columns `t_years, tenor_years, zero_cc_pct, discount_factor, fwd_cc_pct, fwd_annual_pct, fwd_discount_factor` |
| `fic curve template` | `--model nss\|spline\|flat`, `-o`: a valid curve file |
| `fic bond fetch` | `--source six` (required), `--issuer NAME` (case-insensitive substring of the short issuer name; `*`, `\|` and control characters are a usage error), `--rating AAA,AA,...` (exact composite ratings, default all incl. unrated), `--currency CHF` (trading currency, three upper-case letters), `--file FILE\|-` (parse a downloaded SIX CSV, no network; the filters are applied to it), `--timeout 30s`, `--format csv`, `-o FILE` (default stdout, atomic). Writes the bond CSV `maturity,coupon_pct,price_clean,id,freq,daycount,issuer,rating` sorted by maturity, then ISIN; see "SIX bond data" below. Exit 2 for usage errors, 1 for network or format errors and when no usable bond is left (the message names the filters) |
| `fic bond price` | `--curve`, `--date`, `--spread BP`, `--maturity D\|5y`, `--coupon PCT`, `--freq 1\|2\|3\|4\|6\|12` (default 1), `--daycount 30E360\|EUROBOND\|BONDBASIS\|ACT360\|ACTACT`, `--face 100`, `--redemption` (default: the face value), one of `--price CLEAN` (percent of face) or `--yield PCT` (cc), or `--in FILE\|-` for a bond CSV (batch), with `--in`: `--group-by issuer\|rating\|freq\|daycount` (one row per group, see below); callable and putable bonds: `--call DATE@PRICE[,DATE@PRICE...]`, `--put DATE@PRICE[,...]`, `--call-from DATE@PRICE` (exclusive with `--call`), `--vol PCT` (required with these), `--mean-reversion A` (default 0.03), `--steps N` (default 50), see "Callable and putable bonds" below |
| `fic portfolio risk` | `--curve`, `--date`, `--spread BP`, `--in FILE\|-` (required: positions CSV, see below), `--freq`, `--daycount`, `--face` (defaults of missing columns), `--vol PCT`, `--mean-reversion A`, `--steps N` (as `bond price`, needed for call/put rows), `--krd-tenors 3m,6m,1y,...` (default `3m,6m,1y,2y,3y,5y,7y,10y,15y,20y,30y`), `--shocks 50,100,200` (bp), `--scenarios parallel\|irrbb\|none` (comma-separated list allowed, default parallel), `--irrbb-parallel BP`, `--irrbb-short BP`, `--irrbb-long BP`, `--group-by issuer\|rating\|freq\|daycount`, `--section summary\|krd\|scenarios\|positions`, `--strict`, `--format`, `-o`. See "Portfolio risk" below |
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
  base curve). With a curve file without spread and no `--spread` both are equal.

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

**`fic bond price --in FILE --group-by COLUMN`, groups.** Instead of one row per bond the output has one row per
value of `COLUMN` (`issuer`, `rating`, `freq` or `daycount`; anything else is a usage error, exit 2; `issuer` and
`rating` need such a column in the file, else exit 1; an empty issuer or rating is the group `unknown`). Only rows with `status` `ok` and a
market price (`price_source` = `market`) are used; the number of the other rows is reported on stderr. Columns
`<COLUMN>, n, avg_zspread_bp, avg_mod_duration, dv01_sum`, sorted by `avg_zspread_bp`:

- `avg_zspread_bp` = sum(`zspread_bp` x `mod_duration`) / sum(`mod_duration`) (duration-weighted),
- `avg_mod_duration` = sum(`mod_duration`) / `n`, `dv01_sum` = sum of the `dv01` of the group.

There is no total row.

**`fic curve fit --report FILE`, HTML report.** One self-contained file (inline CSS and SVG, no scripts, no external
assets; text is escaped; light and dark colour scheme; no timestamp, so the same fit gives the same bytes). It shows
the as-of date, the number of bonds, the RMSE and the fit warnings, the fitted zero curve(s) (`--model both`: both)
with the market yields of the bonds, the yield errors in bp against the time to maturity, the parameters and the
residuals (the numbers of `--residuals`, rounded). `--report -` writes the HTML to stdout (then the curve needs
`-o FILE`); like `-o` the file is written atomically and must not be an input file or another output. Sample of
`data/bonds_2021-11-26.csv`: [fit_report_sample.html](fit_report_sample.html).

## Callable and putable bonds

`fic bond price --call 2026-03-31@100,2028-03-31@100 --vol 1.0 ...` values a bond with issuer call rights on a
Hull-White trinomial lattice (library: `bond.Callable`, package `lattice`) calibrated to the curve. `DATE@PRICE` is a
date (`YYYY-MM-DD` or `DD.MM.YYYY`) and the **clean** call price in percent of face; `--put` is the same for
investor puts; `--call-from DATE@PRICE` means callable on every coupon date from `DATE` (before maturity) at `PRICE`.
Each date is a Bermudan exercise date; a call on a coupon date pays the clean price and the coupon, between coupons
the clean price plus accrued. Exercise dates before the valuation date are ignored.

- `--vol` (the Hull-White volatility of the short rate in **percent per year**, absolute: 1.0 = 100bp) is required
  (exit 2): it cannot be observed from the bond. `--mean-reversion` (per year, a decimal, default 0.03) and
  `--steps` (lattice steps per year, default 50; raise it for large mean reversion or to check convergence) are
  model inputs too.
- With `--price`: `oas_bp` is the option-adjusted spread over the effective curve (the spread that makes the
  callable value equal the market price); `zspread_bp` stays the z-spread of the straight bond to maturity and
  `yield_cc_pct` the cash-flow yield to maturity. `spread_total_bp` is the file spread plus `--spread` plus the OAS.
  `ytw_cc_pct`, `ytw_date`: yield to worst over the maturity and the call dates (puts are ignored).
- `mod_duration`, `convexity` and `dv01` are the **effective** ones (parallel +-50bp shift of the curve, lattice
  recalibrated, OAS constant); `straight_duration` is the cash-flow duration of the straight bond to maturity.
- `option_value` = `value_straight` - `value_callable`, clean values at the OAS (without a price: on the curve),
  positive when the calls dominate. Without a price the callable is valued on the curve: `price_clean` is the model
  price and `price_clean_model` the same; the OAS columns are empty. `--yield` is not supported for these bonds.
- Batch: the CSV columns `call` (`2028-05-25@101;2029-05-25@100.5`, semicolon-separated), `call_from` and `put`
  are optional; rows without them are priced as before. If any row has terms, `--vol` is required, the
  columns `oas_bp, ytw_cc_pct, ytw_date, straight_duration, option_value, value_straight, value_callable` are added
  (empty for other rows) and `--group-by` averages the OAS of callable and the z-spread of the other bonds (column
  `avg_spread_bp` instead of `avg_zspread_bp`). `--call`, `--put` and `--call-from` cannot be combined with `--in`.

Caveats: the OAS depends on the volatility and the mean reversion (more vol means a more valuable call and a lower
OAS), which are inputs and not observable from the bond; there is no free data source of call schedules (SIX data
has none), so the terms come from the prospectus; the lattice assumes the curve is dated at the valuation date; puts
are valued but ignored in the yield to worst. Sample: `data/bonds_callable_example.csv`.

## Portfolio risk

`fic portfolio risk --curve C --in positions.csv [--vol V]` computes the interest rate risk of a portfolio of bonds
with long and short positions (library: package `portfolio`).

**Input.** The bond CSV of `bond price --in` (`maturity, coupon_pct, price_clean[, id, freq, daycount, face,
redemption, issuer, rating, call, call_from, put]`) plus the **required column `nominal`**: the position size in
currency units of face; **negative for a short position or a liability** (the position factor is `nominal / face`).
A file without a `nominal` column is a usage error (exit 2), a row with an empty, non-numeric or zero nominal is
excluded. Rows with call or put terms need `--vol` (exit 2 without it). Sample: `data/positions_example.csv`.

**Valuation.** Each position is priced as in the batch `bond price`: the z-spread (OAS for rows with call or put
terms, on the Hull-White lattice) is solved from the clean market price against the curve and **held constant** in
every calculation; only the zero curve is bumped or shocked. A row without a price is valued on the curve (spread 0,
`price_source` = `model`). Rows that fail (bad date, matured, bad nominal, no solution) get the reason in the `status`
column of `--section positions`, are excluded from every total, and are reported on stderr; the run continues (exit
0). All rows failing is exit 1 (the first reasons are in the message). With `--strict` the run exits 1 and **prints
nothing** (an existing `-o` file is left untouched) if a row was excluded or a row was valued on the curve without a
market price. Without `--strict` a stderr warning names the ids valued on the curve and the duplicate ids (no
failure). `--curve -` together with `--in -` is a usage error (stdin can be read once). A price that the call/put terms
cannot reach (e.g. far above the call price) fails the row with an OAS "not bracketed" message and a hint.

**Measures** (per position, group and portfolio; amounts include the factor, in currency units):

- `mv_dirty`, `mv_clean`, `accrued`: market value with and without accrued interest (net of shorts).
- `mod_duration` (years, positive), `convexity` (years^2): **money-weighted**, `sum(V_i D_i) / sum(V_i)` with the dirty
  values `V_i` (negative for a short). They are ratios to the *net* value. `gross_mv` = `sum(|V_i|)`. A net value below
  1e-9 of the gross is set to 0; below 1e-6 of the gross (a hedged book) duration, convexity, `krd_*`, `weight_pct` and
  `pv_change_pct` are empty (csv) / null (json) / n/a (text) and a stderr warning says to use `dv01` and `key_dv01`.
  The parallel approximation of the scenarios still works (it uses `sum(V_i D_i)` and `sum(V_i C_i)`). A single short position has the same duration as the
  long one.
- `dv01`: change of the value for +1bp parallel (full revaluation, the sign of a long position is negative, a short
  position is positive).
- **Key rate durations** `krd_<tenor>` (years) and `key_dv01` (currency per 1bp at the key, `-krd * mv_dirty * 1bp`,
  first order). The zero rate is bumped by a triangular (hat) function: 1 at the key tenor, linear down to 0 at the
  neighbouring keys, **flat beyond the first and last key** (cash flows before the first key or after the last key
  load fully on that key), so the hats sum to 1 and the key rate durations sum to the parallel duration. Central
  difference of +-1bp (the conventions of `cashflow.KeyRateDurations`). Portfolio `krd_k = -sum(dV_i/dr_k) / sum(V_i)`.
- **Callable and putable rows** use effective measures: parallel duration, convexity and DV01 as in `bond price`; key
  rate durations by recalibrating the lattice to the curve bumped with the same hat (central difference of +-10bp,
  `portfolio.DefaultCallableBump`), the OAS fixed. The lattice value is only piecewise smooth in the bump: in tests
  the sum of the key rate durations of a 10y bond callable from year 5 is within 0.5% of the effective duration for
  bumps of 1-10bp and 1% (25bp) to 4% (50bp) above; 10bp is the default.

**Scenarios** are full revaluations of every position (callables: lattice recalibrated to the shocked curve, OAS
fixed). `--scenarios parallel` shifts the zero curve by `+-s` bp for each `s` of `--shocks`; `--scenarios irrbb` gives
the six standardised interest rate shock scenarios of the Basel Committee (BCBS d368, 2016), without the floor on the
shocked rates, with `short(t) = R_short exp(-t/4)` and `long(t) = R_long (1 - exp(-t/4))` (t in years):
`parallel_up/down = +-R_parallel`, `steepener = -0.65 short(t) + 0.9 long(t)`, `flattener = 0.8 short(t) - 0.6 long(t)`,
`short_up/down = +-short(t)`. `R_parallel`, `R_short` and `R_long` are currency specific: `--irrbb-parallel`,
`--irrbb-short`, `--irrbb-long` in bp (defaults 100, 150, 100 are illustrative; **check the current BCBS d368 / FINMA
values** and the coefficients before using the results for reporting). Per scenario the output has `pv_change` (net
dirty value, currency), `pv_change_pct` (of the absolute base value, so the sign is the sign of the profit or loss),
and the approximation from the risk measures next to it: for parallel shifts `dr`, `approx_change = V (-D dr + C dr^2 / 2)`
(`approx_basis` = `duration_convexity`); for the other shapes the first-order sum over the key rates of
`key_dv01_k / 1bp * shift(key_k)` (`krd`, no convexity). `approx_error = approx_change - pv_change`.

**Output.** Text (default on a terminal): a summary block, the table by `--group-by` (if given), the key rate table
(`tenor, years, krd, key_dv01`) and the scenario table (`scenario, pv_change, pv_change_pct, approx_change,
approx_error, approx_basis`). `--section` selects one table: `summary` (a record: `n_positions, n_excluded, mv_dirty,
mv_clean, accrued, gross_mv, mod_duration, convexity, dv01`; with `--group-by` one row per group and a final `TOTAL` row with the
columns `<COLUMN>, n, mv_dirty, mv_clean, accrued, weight_pct, weight_gross_pct, mod_duration, convexity, dv01, krd_<tenor>...`;
`weight_pct` is the group's share of the net market value, `weight_gross_pct` its net value as a share of `gross_mv`), `krd`, `scenarios` or `positions` (one row per row of the
file: `id[, issuer][, rating], nominal, price_source, mv_dirty, mv_clean, accrued, spread_bp, mod_duration, convexity,
dv01, krd_<tenor>..., status`; `spread_bp` is the z-spread of a straight bond and the OAS of a callable one). CSV (the default when not on a terminal) prints one rectangular table, by default
`positions`. JSON is the whole report (`settlement, summary, groups, krd, scenarios, positions`), or the one section
with `--section`.

## Data schemas

Curve file (JSON; percent and bp; `model` is `nss`, `spline` with `knots` and `discount_factors`, or `flat`
with `params: {"r": ...}`; a file without `model` is an error, `fic curve template` prints examples):

```json
{"model": "nss", "asof": "2021-03-31", "currency": "CHF", "source": "SNB",
 "params": {"b0": -0.266372, "b1": -0.471343, "b2": 5.68789, "b3": -5.12324, "t1": 5.74881, "t2": 4.14426},
 "spread_bp": 0}
```

Bond file (CSV, see [data/README.md](../data/README.md)): `maturity,coupon_pct,price_clean[,id,freq,daycount,face,redemption,issuer,rating]`,
coupon in percent, price in percent of face, missing price = empty field. Optional call terms: `call`, `call_from`, `put` (see above).

## SIX bond data

`fic bond fetch --source six` queries the public reference data service of SIX Swiss Exchange
(`https://www.six-group.com/fqs/ref.csv`, no login) for the bonds of the segment BO (exchange traded bonds) in the
trading currency `--currency`, optionally restricted to an issuer and to ratings, and writes a bond CSV for
`fic curve fit --bonds` and `fic bond price --in`.

- **Query.** `where` is `PortalSegment=BO*TradingBaseCurrency=CCY[*IssuerNameShort_~NAME][*CompositeRating=R1|R2]`
  (`_~` is a case-insensitive substring match, `=` an exact match, `|` separates alternative values, `*` joins terms,
  which is why `*` and `|` are refused in `--issuer`). The service expects ISO 8859-1: an umlaut in `--issuer` is
  sent as the Latin-1 byte (characters outside Latin-1 are a usage error). Without `--issuer` and `--rating` the
  query returns all bonds of the currency (about 2500 for CHF).
- **Response.** `text/csv` in ISO 8859-1, separated by `;`, with a header; it is converted to UTF-8 (a `--file`
  that is valid UTF-8 is used as is). Quoting is strict: a quote inside an unquoted field, an unterminated quote or a
  row with a different number of fields is an error. Anything that is not a CSV with the columns `MaturityDate`,
  `CouponRate` and `ClosingPrice` (for example the HTML page "Bad request ... Invalid name 'X'") is an error that
  shows a short excerpt. Limits as for the other downloads: `--timeout` (default 30 s), 64 MiB, redirects to https only.
- **Columns written.** `maturity` (ISO date), `coupon_pct`, `price_clean` (the delayed closing price in percent of
  face; empty if the price is missing or 0), `id` (ISIN), `freq`, `daycount`, `issuer`, `rating` (empty if unrated).
  Columns of a `--file` that are absent leave the fields empty (`freq`, `daycount` then fall back to the `fic bond
  price` defaults); this is how files with only `MaturityDate`, `CouponRate` and `ClosingPrice` work.
- **Mappings.** `freq` from `InterestFrequencyCode`: 1, 2 and 4 are coupons per year; other codes (observed `Y`, `Z`)
  are unsupported. `daycount` from `AccruedInterestCalcCode`: `A011` (30/360 ICMA) and `A013` (Eurobond basis
  model 3) are `EUROBOND`, `A001` (30/360 ISDA) is `BONDBASIS`, `A004` (Actual/360) is `ACT360`. `A000` (no accrued
  interest, flat) and unknown codes are unsupported.
- **Skipped rows.** Rows with an unsupported frequency or day count code, perpetual bonds (`MaturityDate`
  `99991231`) and rows with an invalid maturity, coupon or price are not written; stderr gets one warning per reason
  with the count. With `--file` the filters `--issuer`, `--rating` and `--currency` are applied to the rows first.
- **Summary** on stderr: bonds written, skipped rows, count by rating, price coverage. The prices are **delayed
  closing prices**, not tradable quotes, and many bonds trade rarely; check the fit warnings for outliers.
- **Terms of use.** The data is subject to the terms of SIX
  (<https://www.six-group.com/en/products-services/the-swiss-stock-exchange/market-data/news-tools/terms-conditions.html>,
  not verified here); check them before redistributing the data.
- **Migration from `scripts/six-bonds.zsh`.** The script is a deprecated wrapper of this command (`-f FILE` and the
  `ISSUER` argument, default `Glarner`, are kept). The columns changed from `maturity,coupon_pct,price_clean,id,issuer`
  to the list above.

## Pipeline example

This runs literally from a fresh checkout without network (the `--file` and `-f` fixtures replace the downloads).
Online, step 1 is `fic curve fetch --source snb -o curve.json` (CHF) or `fic curve fetch --source ecb -o curve.json`
(EUR), and `fic bond fetch --source six --issuer Glarner -o bonds.csv` for the bonds. The scripts call `fic` from the `PATH` (or
`$FIC`).

```sh
go build -o fic ./cmds/fic && export PATH="$PWD:$PATH"
# 1. a curve file from a downloaded SNB CSV (latest complete date; warns on stderr that the data is old);
#    online: fic curve fetch --source snb   or   fic curve fetch --source ecb
fic curve fetch --source snb --file scripts/testdata/snb_live_sample.csv -o curve.json
# 2. bonds in the bond CSV format (missing prices stay empty)
fic bond fetch --source six --file pkg/source/testdata/six_ref.csv -o bonds.csv
# 3. fit curves to bonds (NSS and spline, written to fit.nss.json and fit.spline.json) and keep the residuals;
#    warnings and the RMSE go to stderr
fic curve fit --bonds data/bonds_2021-11-26.csv --date 2021-11-26 --onrate -0.75 --model both \
   -o fit.json --residuals residuals.csv
# 4. price the bonds on the SNB curve: yield, z-spread, duration per bond (rows without a price are valued on the curve)
fic bond price --curve curve.json --in bonds.csv --format csv > priced.csv
fic bond price --curve curve.json --in bonds.csv --group-by issuer   # duration-weighted z-spread per issuer
# 5. a report of the fit and a comparison of the two models
fic curve fit --bonds data/bonds_2021-11-26.csv --date 2021-11-26 --onrate -0.75 --model both -o fit.json --report fit.html
fic curve compare fit.nss.json fit.spline.json --grid 1m:30y:1m -o compare.csv
# 6. optional: custom plots in R (base graphics, PNG)
fic curve show --curve fit.nss.json --grid 1m:30y:1m -o curve.csv
Rscript r/curve.R residuals.csv curve.png curve.csv
```

`scripts/test.zsh` tests the scripts with the fixtures in `scripts/testdata/` (no network); `FIC` selects the
binary (`FIC=./fic zsh scripts/test.zsh`).

## Regenerating the committed samples

`data/` holds the curve and bonds. `data/fit_residuals_2021-11-26.csv` and `data/fit_2021-11-26.png` are the output of
exactly (the `--onrate -0.75` matters; `docs/fit_report_sample.html` is the `--report` of the same command):

```sh
fic curve fit --bonds data/bonds_2021-11-26.csv --date 2021-11-26 --onrate -0.75 --model both \
   -o fit.json --residuals data/fit_residuals_2021-11-26.csv --report docs/fit_report_sample.html
Rscript r/curve.R data/fit_residuals_2021-11-26.csv data/fit_2021-11-26.png
```

(`fit.nss.json` and `fit.spline.json` are by-products and are ignored by git.) A test
(`TestRegenerateSample` in `internal/cli`) regenerates the residuals and the report in-process and compares them byte for byte.


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

See [CHANGELOG.md](CHANGELOG.md) for the API and CLI migration tables (including the removed legacy tools).
