#!/bin/zsh
#
# Prices bonds on a curve: price, yield, z-spread, duration, convexity and
# DV01 per bond (a table; rows that cannot be priced have an error text in
# the status column).
#
#   price-bonds.zsh [-c curve.json] [-d YYYY-MM-DD] [-s SPREAD_BP] [bonds.csv]
#
#   bonds.csv  bond CSV (see "fic help bond price"; default: stdin), e.g. the
#              output of "fic bond fetch --source six"
#   -c FILE    curve file (default: $FIC_CURVE or ./curve.json)
#   -d DATE    valuation date (default: asof date of the curve, else today)
#   -s BP      z-spread in bp added to the curve
#   -f FORMAT  text (default), csv or json
#
# Example: fic bond fetch --source six --issuer Glarner | price-bonds.zsh -c curve.json
# The exit code is that of fic (1 only if all rows failed).

set -u
prog=${0:t}
usage() { print -u2 "usage: $prog [-c curve.json] [-d date] [-s bp] [-f text|csv|json] [bonds.csv]"; exit 2 }

: ${FIC:=fic}
args=()
fmt=text
while getopts "c:d:s:f:h" opt; do
	case $opt in
		c) args+=(--curve "$OPTARG") ;;
		d) args+=(--date "$OPTARG") ;;
		s) args+=(--spread "$OPTARG") ;;
		f) fmt=$OPTARG ;;
		*) usage ;;
	esac
done
shift $((OPTIND - 1))
(( $# <= 1 )) || usage

exec $FIC bond price $args --in "${1:--}" --format $fmt
