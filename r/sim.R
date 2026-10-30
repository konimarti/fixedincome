# Plots the result of `fic sim zcb`: Monte Carlo zero bond prices vs. the
# model/curve.
#
# Usage: Rscript r/sim.R [sim.csv [out.png]]
#        (defaults: sim.csv, sim.png)
#
#   sim.csv  CSV written by `fic sim zcb -o sim.csv` (columns t_years,
#            curve_zero_cc_pct, curve_fwd_cc_pct, model_discount, mc_discount,
#            mc_stderr, mc_zero_cc_pct, z_score; rates in percent)
#
# Columns are read by name, rows are sorted by time. The PNG is written
# via a temporary file that is renamed (no Rplots.pdf, no half-written output).
args <- commandArgs(trailingOnly = TRUE)
input <- if (length(args) >= 1) args[1] else "sim.csv"
output <- if (length(args) >= 2) args[2] else "sim.png"

if (!file.exists(input)) stop(paste("file not found:", input))
df <- read.csv(input, header = TRUE, stringsAsFactors = FALSE)
required <- c("t_years", "curve_zero_cc_pct", "curve_fwd_cc_pct", "model_discount",
              "mc_discount", "mc_stderr", "mc_zero_cc_pct", "z_score")
missing <- setdiff(required, names(df))
if (length(missing) > 0) {
  stop(paste0("missing columns in ", input, ": ", paste(missing, collapse = ", "),
              " (found: ", paste(names(df), collapse = ", "), ")"))
}
if (nrow(df) == 0) stop(paste("no rows in", input))
df <- df[order(df$t_years), ]

tmp <- paste0(output, ".tmp")
png(tmp, width = 900, height = 1200)
ok <- FALSE
on.exit({ if (dev.cur() > 1) dev.off(); if (!ok && file.exists(tmp)) file.remove(tmp) })
par(mfrow = c(3, 1), mar = c(4, 5, 3, 2))

rates <- df[, c("curve_zero_cc_pct", "curve_fwd_cc_pct", "mc_zero_cc_pct")]
plot(df$t_years, df$curve_zero_cc_pct, type = "l", lwd = 2, ylim = range(rates, na.rm = TRUE),
     main = "Zero rate and forward rate curves", ylab = "Rate in %", xlab = "Time (years)")
lines(df$t_years, df$curve_fwd_cc_pct, col = "green", lwd = 2)
lines(df$t_years, df$mc_zero_cc_pct, col = "red", lwd = 2, lty = 2)
legend("topleft", legend = c("Zero rate (curve)", "Forward rate (curve)", "Zero rate (Monte Carlo)"),
       lty = c(1, 1, 2), col = c("black", "green", "red"), lwd = 2)

lo <- 100 * (df$mc_discount - 1.96 * df$mc_stderr)
hi <- 100 * (df$mc_discount + 1.96 * df$mc_stderr)
plot(df$t_years, 100 * df$model_discount, type = "l", lwd = 2,
     ylim = range(c(lo, hi, 100 * df$model_discount), na.rm = TRUE),
     main = "Zero bond prices", ylab = "Price in % of face value", xlab = "Time (years)")
lines(df$t_years, 100 * df$mc_discount, col = "red", lwd = 2)
lines(df$t_years, lo, col = "red", lty = 3)
lines(df$t_years, hi, col = "red", lty = 3)
legend("topleft", legend = c("Model (analytic)", "Monte Carlo", "95% confidence band"),
       lty = c(1, 1, 3), col = c("black", "red", "red"), lwd = c(2, 2, 1))

plot(df$t_years, df$z_score, type = "h", lwd = 2, ylim = range(c(df$z_score, -2, 2), na.rm = TRUE), main = "Deviation of Monte Carlo from model",
     ylab = "(MC - model) / standard error", xlab = "Time (years)")
abline(h = c(-2, 0, 2), col = c("grey", "black", "grey"), lty = c(2, 1, 2))

invisible(dev.off())
if (!file.rename(tmp, output)) stop(paste("cannot write", output))
ok <- TRUE
cat("wrote", output, "\n")
