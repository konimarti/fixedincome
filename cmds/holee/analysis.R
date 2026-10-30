# Plots the output of the holee command.
#
# Usage: Rscript analysis.R [input.csv [output.png]]
#        (defaults: holee.csv, holee.png)
#
# The input is the CSV written by `holee -o holee.csv` (header with
# snake_case column names, rates as decimals).
args <- commandArgs(trailingOnly = TRUE)
input <- if (length(args) >= 1) args[1] else "holee.csv"
output <- if (length(args) >= 2) args[2] else "holee.png"

df <- read.csv(input, header = TRUE)
required <- c("t", "zero_rate", "fwd_rate", "theta", "discount", "mc_discount", "mc_stderr", "mc_zero_rate")
missing <- setdiff(required, colnames(df))
if (length(missing) > 0) {
	stop(paste("missing columns in", input, ":", paste(missing, collapse = ", ")))
}

png(output, width = 900, height = 1200)
save <- par(mfrow = c(3, 1))

rates <- 100 * df[, c("zero_rate", "fwd_rate", "mc_zero_rate")]
plot(df$t, 100 * df$zero_rate, type = "l", lwd = 2, ylim = range(rates),
	main = "Spot rate and forward rate curves", ylab = "Rates in %", xlab = "Time to maturity")
lines(df$t, 100 * df$fwd_rate, col = "green", lwd = 2)
lines(df$t, 100 * df$mc_zero_rate, col = "red", lwd = 2, lty = 2)
legend("topleft", legend = c("Spot rate curve", "Forward rate curve", "Ho-Lee spot curve (Monte Carlo)"),
	lty = c(1, 1, 2), col = c("black", "green", "red"))

lo <- 100 * (df$mc_discount - 1.96 * df$mc_stderr)
hi <- 100 * (df$mc_discount + 1.96 * df$mc_stderr)
plot(df$t, 100 * df$discount, type = "l", lwd = 2, ylim = range(c(lo, hi, 100 * df$discount)),
	main = "Zero bonds", ylab = "Prices in % of face value", xlab = "Time to maturity")
lines(df$t, 100 * df$mc_discount, col = "red", lwd = 2)
lines(df$t, lo, col = "red", lty = 3)
lines(df$t, hi, col = "red", lty = 3)
legend("topleft", legend = c("Zero bonds (curve)", "Zero bonds (Ho-Lee, Monte Carlo)", "95% confidence band"),
	lty = c(1, 1, 3), col = c("black", "red", "red"))

plot(df$t, df$theta, type = "l", lwd = 2, main = "Drift theta(t) of the Ho-Lee model",
	ylab = "theta (per year)", xlab = "Time to maturity")

par(save)
invisible(dev.off())
cat("wrote", output, "\n")
