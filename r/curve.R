# Plots the result of `fic curve fit` (and optionally a curve table).
#
# Usage: Rscript r/curve.R [residuals.csv [out.png [curve.csv]]]
#
#   residuals.csv  CSV written by `fic curve fit --residuals residuals.csv`
#                  (default: residuals.csv)
#   out.png        output file (default: curve.png)
#   curve.csv      optional CSV of `fic curve show` or `fic curve forward`
#                  (columns t_years, zero_cc_pct and inst_fwd_cc_pct or
#                  fwd_cc_pct); adds a panel with the zero and forward rates.
#                  Use - or omit to skip.
#
# Columns are read by name, rows are sorted by time to maturity. Rates are
# in percent, prices in percent of face, errors in bp. All inputs are read
# and validated before the PNG is opened; the PNG is written to a temporary
# file and renamed (no Rplots.pdf, no half-written output).
args <- commandArgs(trailingOnly = TRUE)
input <- if (length(args) >= 1) args[1] else "residuals.csv"
output <- if (length(args) >= 2) args[2] else "curve.png"
curve_file <- if (length(args) >= 3 && args[3] != "-") args[3] else NULL

read_table <- function(path, required) {
  if (!file.exists(path)) stop(paste("file not found:", path))
  df <- read.csv(path, header = TRUE, stringsAsFactors = FALSE)
  missing <- setdiff(required, names(df))
  if (length(missing) > 0) {
    stop(paste0("missing columns in ", path, ": ", paste(missing, collapse = ", "),
                " (found: ", paste(names(df), collapse = ", "), ")"))
  }
  if (nrow(df) == 0) stop(paste("no rows in", path))
  df
}

# ---- read and validate everything before drawing ----
df <- read_table(input, c("model", "ttm_years", "price_clean_market", "price_clean_model",
                          "yield_cc_market_pct", "yield_cc_model_pct", "error_bp"))
df <- df[order(df$ttm_years), ]
models <- unique(df$model)
has_zero <- "zero_cc_model_pct" %in% names(df)
# market data: one row per bond (identical for all models)
mkt <- df[df$model == models[1], ]

cv <- NULL
fwdcol <- NULL
if (!is.null(curve_file)) {
  cv <- read_table(curve_file, c("t_years", "zero_cc_pct"))
  fwdcol <- intersect(c("inst_fwd_cc_pct", "fwd_cc_pct"), names(cv))
  if (length(fwdcol) == 0) stop(paste("missing column inst_fwd_cc_pct or fwd_cc_pct in", curve_file))
  cv <- cv[order(cv$t_years), ]
}

colors <- c(nss = "red", spline = "darkgreen")
labels <- c(nss = "Nelson-Siegel-Svensson", spline = "Cubic spline")
col <- function(m) if (m %in% names(colors)) colors[[m]] else "blue"
label <- function(m) if (m %in% names(labels)) labels[[m]] else m

# ---- plot to a temporary file, rename on success ----
panels <- 3 + (!is.null(cv))
tmp <- paste0(output, ".tmp")
png(tmp, width = 900, height = 400 * panels)
ok <- FALSE
on.exit({ if (dev.cur() > 1) dev.off(); if (!ok && file.exists(tmp)) file.remove(tmp) })
par(mfrow = c(panels, 1), mar = c(4, 5, 3, 2))

ylim <- range(c(df$yield_cc_market_pct, df$yield_cc_model_pct, if (has_zero) df$zero_cc_model_pct), na.rm = TRUE)
plot(mkt$ttm_years, mkt$yield_cc_market_pct, type = "p", pch = 3, ylim = ylim, log = "x",
     main = "Market and model yields (continuously compounded)",
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

ylim <- range(c(df$price_clean_market, df$price_clean_model), na.rm = TRUE)
plot(mkt$ttm_years, mkt$price_clean_market, type = "p", pch = 3, ylim = ylim, log = "x",
     main = "Quoted and model clean prices",
     xlab = "Time to maturity (years)", ylab = "Clean price (% of face)")
for (m in models) {
  d <- df[df$model == m, ]
  lines(d$ttm_years, d$price_clean_model, col = col(m), type = "o", pch = 20)
}
legend("topleft", legend = c("Quoted", sapply(models, label)), lty = c(NA, rep(1, length(models))),
       pch = c(3, rep(20, length(models))), col = c("black", sapply(models, col)))

plot(NULL, xlim = range(df$ttm_years, na.rm = TRUE), ylim = range(c(0, df$error_bp), na.rm = TRUE), log = "x",
     main = "Yield errors (model - market)",
     xlab = "Time to maturity (years)", ylab = "Error in bp")
abline(h = 0, col = "grey")
for (m in models) {
  d <- df[df$model == m, ]
  lines(d$ttm_years, d$error_bp, col = col(m), type = "o", pch = 20)
}
legend("bottomright",
       legend = sapply(models, function(m) sprintf("%s (RMSE %.2f bp)", label(m), sqrt(mean(df$error_bp[df$model == m]^2, na.rm = TRUE)))),
       lty = 1, pch = 20, col = sapply(models, col))

if (!is.null(cv)) {
  plot(cv$t_years, cv$zero_cc_pct, type = "l", lwd = 2, ylim = range(c(cv$zero_cc_pct, cv[[fwdcol[1]]]), na.rm = TRUE),
       main = "Zero and forward rates of the curve", xlab = "Time (years)", ylab = "Rate in %")
  lines(cv$t_years, cv[[fwdcol[1]]], col = "green", lwd = 2)
  legend("topleft", legend = c("Zero rate (cc)", paste("Forward rate", fwdcol[1])), lty = 1, lwd = 2, col = c("black", "green"))
}

invisible(dev.off())
if (!file.rename(tmp, output)) stop(paste("cannot write", output))
ok <- TRUE
cat("wrote", output, "\n")
