# Sample data

Small fixtures for the examples, the tests (`internal/cli`) and the scripts.

| File | Content | Provenance |
|---|---|---|
| `curve_snb_2021-03-31.json` | Nelson-Siegel-Svensson curve of the Swiss National Bank (CHF risk-free spot rates) as of 2021-03-31 | parameters published by the SNB ([data.snb.ch](https://data.snb.ch/en/topics/ziredev#!/cube/rendopar)), written with `fic curve template` |
| `bonds_2021-11-26.csv` | 39 CHF bonds (money market bills and Swiss Confederation bonds) with the clean prices of 2021-11-26 | the former `bonddata.csv` of the removed `termfit` tool in the new format (ISINs/valor numbers in `id`); dates normalised to ISO, columns renamed |
| `fit_residuals_2021-11-26.csv` | per-bond residuals of the NSS and spline fits of `bonds_2021-11-26.csv` | `fic curve fit`, see "Regenerating the samples" |
| `fit_2021-11-26.png` | plot of the fits and residuals | `r/curve.R` on the residuals file |

Test fixtures of the downloads (SNB cube `rendopar` as of 2025-07-29..31, ECB AAA parameters and spot rates of
2026-10-27..29, as downloaded on 2026-10-30) are in `pkg/source/testdata/` and `scripts/testdata/`. Fresh curve
files: `fic curve fetch --source snb|ecb` (see the README, including the note that the SNB cube is no longer
updated).

## Curve file (JSON)

Rates and betas in percent, spread in basis points, maturities and decay
parameters in years. `model` is `nss`, `spline` (`knots`, `discount_factors`
instead of `params`) or `flat` (`params: {"r": 1.0}`); `asof`, `currency`,
`source` and `version` are optional. `fic curve fit` and `fic curve fetch` write this format
(`source` is the bond file), `fic curve template` prints examples.

```json
{"model": "nss", "asof": "2021-03-31", "currency": "CHF", "source": "SNB",
 "params": {"b0": -0.266372, "b1": -0.471343, "b2": 5.68789, "b3": -5.12324, "t1": 5.74881, "t2": 4.14426},
 "spread_bp": 0}
```

## Bond file (CSV)

Comma separated with a header; columns are identified by name (case
insensitive, any order, unknown columns ignored):

| Column | Meaning | Required |
|---|---|---|
| `maturity` | maturity date `YYYY-MM-DD` or `DD.MM.YYYY` | yes |
| `coupon_pct` | annual coupon in percent of face (alias `coupon`) | yes |
| `price_clean` | quoted clean price in percent of face (alias `price`); empty, `N/A` or `-` = no price | yes |
| `id` | identifier (ISIN), default `row<line>` | no |
| `freq` | coupons per year (1, 2, 3, 4, 6, 12), default `--freq` | no |
| `daycount` | `30E360`, `EUROBOND`, `BONDBASIS`, `ACT360`, `ACTACT`, default `--daycount` | no |
| `face`, `redemption` | face value (default 100) and redemption (default face) | no |
| `issuer` | echoed to the output; `fic bond price --in --group-by issuer` aggregates per issuer | no |

Without a header the columns are `maturity,coupon,price,id`. CRLF line endings,
a byte order mark, quoted fields and blank lines are accepted.
`fic bond price --in` reports a problem of a row in the `status` column,
`fic curve fit` skips such rows with a warning.

## Regenerating the samples

`data/fit_residuals_2021-11-26.csv` (per-bond residuals of the fit of `bonds_2021-11-26.csv`) and `data/fit_2021-11-26.png`
are the output of exactly:

```sh
fic curve fit --bonds data/bonds_2021-11-26.csv --date 2021-11-26 --onrate -0.75 --model both \
   -o fit.json --residuals data/fit_residuals_2021-11-26.csv
Rscript r/curve.R data/fit_residuals_2021-11-26.csv data/fit_2021-11-26.png
```

(`--onrate -0.75` imposes an overnight rate of -0.75%; without it the result differs.)
