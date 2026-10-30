# R plots

Base graphics only; the PNG is always written explicitly. Columns are read by
name from the CSV files of `fic` and the scripts stop with a message if a
column is missing.

```sh
fic curve fit --bonds data/bonds_2021-11-26.csv --date 2021-11-26 --onrate -0.75 --model both \
   -o fit.json --residuals residuals.csv
fic curve show --curve fit.nss.json --grid 1m:30y:1m -o curve.csv
Rscript r/curve.R residuals.csv curve.png curve.csv

fic sim zcb --curve data/curve_snb_2021-03-31.json --sigma 1 --horizon 10y -o sim.csv
Rscript r/sim.R sim.csv sim.png
```

| Script | Arguments (defaults) | Input |
|---|---|---|
| `r/curve.R` | `[residuals.csv [out.png [curve.csv]]]` (`residuals.csv`, `curve.png`, none) | `fic curve fit --residuals`, optionally `fic curve show/forward` |
| `r/sim.R` | `[sim.csv [out.png]]` (`sim.csv`, `sim.png`) | `fic sim zcb` |

The committed `cmds/termfit/result.csv` and `termfit.png` are produced by exactly (note `--onrate -0.75`):

```sh
fic curve fit --bonds data/bonds_2021-11-26.csv --date 2021-11-26 --onrate -0.75 --model both \
   -o fit.json --residuals cmds/termfit/result.csv
Rscript r/curve.R cmds/termfit/result.csv cmds/termfit/termfit.png
```

The scripts read and validate all inputs before the PNG is opened and write it via a temporary file, so an error
never leaves a half-written output.
