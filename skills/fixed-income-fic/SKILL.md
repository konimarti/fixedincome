---
name: fixed-income-fic
description: Fetch yield curves (SNB, ECB), fit Nelson-Siegel-Svensson or spline curves to bond prices, and price bonds, swaps, options and short-rate simulations with the fic command line tool
version: 1.0.0
author: konimarti
license: MIT
metadata:
  hermes:
    tags: [Finance, Fixed Income, Bonds, Yield Curve, Valuation]
---

# Fixed income valuation with `fic`

`fic` is a single-binary tool (github.com/konimarti/fixedincome) for yield curves and fixed income valuation.
Use it instead of doing bond or curve arithmetic by hand: it handles day counts, schedules, accrued interest,
yield and z-spread solving, and curve fitting.

## When to Use

- The user wants a current CHF or EUR yield curve, zero or forward rates, or a comparison of two curves.
- The user wants a curve fitted to a list of bonds (prices), or the fit quality (residuals).
- The user wants a bond's yield, z-spread, duration, convexity or DV01, from a quoted price or from a curve.
- The user wants par swap rates, a Black-Scholes option price with Greeks or implied volatility, or a Ho-Lee,
  Hull-White or Vasicek zero-bond simulation.

## Setup

Check `command -v fic`. If it is missing, install it (needs Go 1.21+):

```sh
go install github.com/konimarti/fixedincome/cmds/fic@latest   # puts fic in $(go env GOPATH)/bin
```

`fic help` lists the commands, `fic help <command>` lists the flags of one command. Prefer that over guessing a flag.

## Conventions (get these right)

- **Rates, yields, volatilities and dividend yields are in percent** (`2` means 2%). **Spreads are in basis points.**
- Yields and zero rates are continuously compounded (`_cc_pct`); the annual-compounded value is shown next to them.
- Dates are `YYYY-MM-DD` or `DD.MM.YYYY`. Maturities, tenors and horizons may be `6m`, `2y`, `10d`.
- Bond prices are **clean prices in percent of face**. Duration is positive; DV01 is negative for a long bond.
- Use `--format json` or `csv` for results you will process, and keep stdout (data) apart from stderr
  (warnings, summaries). Exit codes: `0` ok, `1` runtime or data error, `2` usage error.
- Work in a scratch directory and pass `-o FILE`; `fic` writes files atomically and refuses `-o` equal to an input.

## Quick Reference

| Goal | Command |
|---|---|
| Latest ECB curve (EUR) | `fic curve fetch --source ecb -o curve.json` |
| SNB curve (CHF) | `fic curve fetch --source snb -o curve.json` |
| Zero and forward rates | `fic curve show --curve curve.json --t 1y,2y,5y,10y --format json` |
| Compare two curves (B minus A, bp) | `fic curve compare a.json b.json --t 1y,5y,10y` |
| CHF bonds from SIX by issuer/rating | `fic bond fetch --source six --issuer Glarner --rating AA,AAA -o bonds.csv` |
| Fit curve to bonds | `fic curve fit --bonds bonds.csv --date YYYY-MM-DD --model nss -o fit.json --residuals res.csv --report fit.html` |
| Price one bond from a quote | `fic bond price --curve curve.json --coupon 1.25 --maturity 2031-05-25 --price 101.2 --format json` |
| Bond price from a yield | `fic bond price --curve curve.json --coupon 1.25 --maturity 2031-05-25 --yield 1.1` |
| Callable bond: OAS, effective duration | `fic bond price --curve curve.json --coupon 2 --maturity 2031-03-31 --price 101 --call-from 2026-03-31@100 --vol 1.0 --format json` (or `--call DATE@PRICE,...`, `--put`) |
| Price a bond list | `fic bond price --curve curve.json --in bonds.csv --format csv` |
| Average z-spread per issuer or rating | `fic bond price --curve curve.json --in bonds.csv --group-by issuer` (or `rating`) |
| Portfolio risk (long/short, nominal column) | `fic portfolio risk --curve curve.json --in positions.csv [--vol 1.0] [--scenarios parallel,irrbb] [--group-by rating] --format json` (`--section summary\|krd\|scenarios\|positions`) |
| Par swap rates | `fic swap rate --curve curve.json --tenors 2y,5y,10y` |
| Option price and Greeks | `fic option price --type call --spot 110 --strike 100 --expiry 2y --vol 30 --rate 2` |
| Implied volatility | `fic option price --type call --spot 110 --strike 100 --expiry 2y --implied-from 28.5 --rate 2` |
| Zero-bond simulation | `fic sim zcb --model holee --curve curve.json --sigma 1 --paths 10000 --antithetic` |

Bond file columns: `maturity,coupon_pct,price_clean[,id,freq,daycount,face,redemption,issuer]`. Coupon is in percent,
price in percent of face, a missing price is an empty field. `freq` is 1, 2, 3, 4, 6 or 12 (default 1); `daycount` is
30E360, EUROBOND, BONDBASIS, ACT360 or ACTACT (default 30E360).

## Procedure

1. **Get or build a curve.** Fetch one, or fit one to the user's bonds. Look at the warnings on stderr and report them.
2. **Check it** with `fic curve show` before using it. Zero rates should look plausible for the currency and date.
3. **Run the valuation** with `--format json` and read the named fields (`yield_cc_pct`, `zspread_bp`,
   `mod_duration`, `convexity`, `dv01`, `price_clean_model`, ...) rather than parsing text output.
4. **Report** the numbers with units, the curve date and source, and any warning that affects their meaning.

## Pitfalls

- **Stale SNB data.** The SNB parameter cube currently ends on 2025-07-31; `fic` warns about this. Use
  `--source ecb` for current EUR data, and tell the user when a curve is old. Add `--strict` to fail instead of warn.
- **Valuation date.** The default is the `asof` date of the curve file; `fic` warns if `--date` differs. State the
  date you used.
- **Fit warnings.** A poorly determined NSS long end, dropped bonds (matured, `--min-ttm`) or large residuals are
  reported on stderr. Check the residuals (`--residuals`, or `--report`, a self-contained HTML page) and say when the
  fit is poor. Do not present a fitted curve as reliable beyond the longest bond maturity.
- **Overnight rate.** `--onrate 0` pins the short end to 0%; leave the flag out for no constraint. Negative values
  (for example SARON) are valid.
- **Units.** Passing `0.02` where `2` is meant gives a 0.02% rate. Convert before calling.
- **Batch rows.** In `bond price --in`, a row that fails has the reason in `status`; the run only exits 1 if every row
  failed. Report failed rows to the user.
- **Callable bonds.** Always supply `--vol` (percent per year, absolute; 1.0 = 100bp): it is not observable from the
  bond and `fic` exits 2 without it. The OAS depends on the vol and `--mean-reversion` (default 0.03), so state the
  assumed values. `mod_duration`/`dv01` are effective ones for these bonds, `zspread_bp` is the straight bond's. Call
  schedules must come from the user; puts are ignored in the yield to worst.
- **Portfolio risk.** `positions.csv` is a bond file with a required `nominal` column (currency units of face,
  negative = short or liability; exit 2 without it). Spreads (z-spread, OAS) are solved from the prices and held
  constant; scenarios are full revaluations. Duration and KRD are ratios to the *net* market value: for a hedged book
  quote `dv01` and `key_dv01` (currency per bp) instead. The IRRBB shock sizes (`--irrbb-parallel/-short/-long`, bp)
  are illustrative defaults and no floor on shocked rates is applied: ask the user for the current BCBS d368 / FINMA values and say which were used. `--strict` prints nothing if a row is excluded or unpriced. Excluded
  rows are listed in `--section positions` (`status`) and on stderr.
- **Spreads.** `zspread_bp` is the spread over the curve actually used (curve file plus `--spread`);
  `spread_total_bp` is relative to the curve without any spread.
- **Not covered:** notional exchange and calendars in swaps, compounded-in-arrears floaters (SARON, SOFR, ESTR),
  and credit-risk modelling beyond a z-spread. Say so rather than approximating.
- **SIX bonds.** Prices are delayed closing prices; perpetuals and bonds without a supported accrual code are skipped
  (counted on stderr). Mixed pools (all AAA issuers: government-guaranteed, Pfandbriefe, ...) fit poorly with one
  curve: fit one issuer or one homogeneous group, and look at the outlier warnings.
- Never invent market data. Prices, curves and ratings must come from the user, a file, or `fic curve fetch`.

## Verification

- Round trip: price a bond with `--price`, then price it again with the returned `yield_cc_pct` as `--yield`;
  the clean prices must agree.
- After fitting, the RMSE and per-bond residuals (bp) should be small relative to bid-ask spreads; look at the largest.
- `fic curve compare` of a curve with itself must show zero differences.
