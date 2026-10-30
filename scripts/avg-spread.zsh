#!/bin/zsh
#
# Duration-weighted average z-spread (bp) of bonds per issuer.
#
#   avg-spread.zsh [-c curve.json] [-d YYYY-MM-DD] [bonds.csv]
#
#   bonds.csv  bond CSV (default: stdin) with the columns maturity,
#              coupon_pct, price_clean and optionally issuer (bonds are
#              grouped by the issuer column; without it all bonds form the
#              group "all"), e.g. the output of six-bonds.zsh
#   -c FILE    curve file (default: $FIC_CURVE or ./curve.json)
#   -d DATE    valuation date (default: asof date of the curve, else today)
#
# The bonds are valued with "fic bond price --format csv". The aggregation
# below reads the output columns by name (issuer, zspread_bp, mod_duration,
# status, price_source) and skips rows whose status is not "ok" and rows
# valued on the curve without a market price (price_source is not "market");
# the number of skipped rows is reported on stderr:
#
#   avg_zspread_bp = sum(zspread_bp * mod_duration) / sum(mod_duration)
#   avg_duration   = sum(mod_duration) / n
#
# Output: CSV with the columns issuer,n,avg_zspread_bp,avg_mod_duration,
# sorted by the spread. Example: six-bonds.zsh GKB | avg-spread.zsh -c curve.json

set -u
setopt pipefail
here=${0:A:h}
prog=${0:t}
usage() { print -u2 "usage: $prog [-c curve.json] [-d date] [bonds.csv]"; exit 2 }

: ${FIC:=fic}
args=()
while getopts "c:d:h" opt; do
	case $opt in
		c) args+=(--curve "$OPTARG") ;;
		d) args+=(--date "$OPTARG") ;;
		*) usage ;;
	esac
done
shift $((OPTIND - 1))
(( $# <= 1 )) || usage

$FIC bond price $args --in "${1:--}" --format csv | awk -f $here/lib/csv.awk -f $here/lib/avg-spread.awk
