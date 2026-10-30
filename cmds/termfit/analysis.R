# Plots the termfit results: model vs market yields, clean prices and
# yield errors per bond for each fitted model.
#
# usage: Rscript analysis.R [result.csv] [termfit.png]
args <- commandArgs(trailingOnly = TRUE)
input <- if (length(args) >= 1) args[1] else "result.csv"
output <- if (length(args) >= 2) args[2] else "termfit.png"

df <- read.csv(input, header = TRUE, stringsAsFactors = FALSE)
required <- c("model", "ttm_years", "price_clean_market", "price_clean_model",
              "yield_cc_market_pct", "yield_cc_model_pct", "error_bp")
missing <- setdiff(required, names(df))
if (length(missing) > 0) {
  stop(paste("missing columns in", input, ":", paste(missing, collapse = ", ")))
}
df <- df[order(df$ttm_years), ]
models <- unique(df$model)
colors <- c(nss = "red", spline = "darkgreen")
col <- function(m) if (m %in% names(colors)) colors[[m]] else "blue"
labels <- c(nss = "Nelson-Siegel-Svensson", spline = "Cubic spline")
label <- function(m) if (m %in% names(labels)) labels[[m]] else m

# market data: one row per bond (identical for all models)
mkt <- df[df$model == models[1], ]

png(output, width = 900, height = 1200)
par(mfrow = c(3, 1), mar = c(4, 5, 3, 2))

has_zero <- "zero_cc_model_pct" %in% names(df)
ylim <- range(c(df$yield_cc_market_pct, df$yield_cc_model_pct, if (has_zero) df$zero_cc_model_pct))
plot(mkt$ttm_years, mkt$yield_cc_market_pct, type = "p", pch = 3, ylim = ylim, log = "x",
     main = "Market and model yields, model zero curves (continuously compounded)",
     xlab = "Time to maturity (years)", ylab = "Rate in %")
for (m in models) {
  d <- df[df$model == m, ]
  lines(d$ttm_years, d$yield_cc_model_pct, col = col(m), type = "o", pch = 20)
  if (has_zero) lines(d$ttm_years, d$zero_cc_model_pct, col = col(m), lty = 2)
}
leg <- c("Market yield", paste(sapply(models, label), "yield"))
lty <- c(NA, rep(1, length(models)))
pch <- c(3, rep(20, length(models)))
lcol <- c("black", sapply(models, col))
if (has_zero) {
  leg <- c(leg, paste(sapply(models, label), "zero rate"))
  lty <- c(lty, rep(2, length(models)))
  pch <- c(pch, rep(NA, length(models)))
  lcol <- c(lcol, sapply(models, col))
}
legend("topleft", legend = leg, lty = lty, pch = pch, col = lcol)

ylim <- range(c(df$price_clean_market, df$price_clean_model))
plot(mkt$ttm_years, mkt$price_clean_market, type = "p", pch = 3, ylim = ylim, log = "x",
     main = "Quoted and model clean prices",
     xlab = "Time to maturity (years)", ylab = "Clean price")
for (m in models) {
  d <- df[df$model == m, ]
  lines(d$ttm_years, d$price_clean_model, col = col(m), type = "o", pch = 20)
}
legend("topleft", legend = c("Quoted", sapply(models, label)), lty = c(NA, rep(1, length(models))),
       pch = c(3, rep(20, length(models))), col = c("black", sapply(models, col)))

ylim <- range(c(0, df$error_bp))
plot(NULL, xlim = range(df$ttm_years), ylim = ylim, log = "x",
     main = "Yield errors (model - market)",
     xlab = "Time to maturity (years)", ylab = "Error in bp")
abline(h = 0, col = "grey")
for (m in models) {
  d <- df[df$model == m, ]
  lines(d$ttm_years, d$error_bp, col = col(m), type = "o", pch = 20)
}
legend("bottomright", legend = sapply(models, function(m) sprintf("%s (RMSE %.2f bp)", label(m), sqrt(mean(df$error_bp[df$model == m]^2)))),
       lty = 1, pch = 20, col = sapply(models, col))

invisible(dev.off())
cat("wrote", output, "\n")
