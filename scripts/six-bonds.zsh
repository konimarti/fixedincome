#!/bin/zsh
#
# DEPRECATED: use "fic bond fetch --source six" (this script is a thin
# wrapper around it).
#
# Lists CHF bonds of SIX Swiss Exchange as a bond CSV for
#
#   fic bond price --in bonds.csv     fic curve fit --bonds bonds.csv
#
#   six-bonds.zsh [-f FILE] [ISSUER] > bonds.csv
#
#   ISSUER    issuer name (short name) filter, default: Glarner (not with -f:
#             then all rows of FILE, unless ISSUER is given)
#   -f FILE   read the SIX CSV from FILE ("-": stdin) instead of downloading
#
# Output (comma separated, header, ISO dates, coupon in percent, clean price
# in percent of face; a missing price is an EMPTY field, not 0):
#
#   maturity,coupon_pct,price_clean,id,freq,daycount,issuer,rating
#
# The old output had the columns maturity,coupon_pct,price_clean,id,issuer;
# fic adds freq, daycount and rating, skips perpetuals and bonds without
# accrued interest or with an unknown frequency / day count (see "fic bond
# fetch --help" and docs/cli.md). The fic binary is taken from $FIC (default:
# fic from the PATH).

set -u
prog=${0:t}
: ${FIC:=fic}

usage() { print -u2 "usage: $prog [-f FILE] [ISSUER]"; exit 2 }

input="" have_f=0
while getopts "f:h" opt; do
	case $opt in
		f) input=$OPTARG; have_f=1 ;;
		*) usage ;;
	esac
done
shift $((OPTIND - 1))
(( $# <= 1 )) || usage
(( have_f == 0 )) || [[ -n $input ]] || usage
if ! command -v $FIC >/dev/null 2>&1; then
	print -u2 "$prog: fic not found; build it with: go build ./cmds/fic and put it on PATH or set FIC"
	exit 1
fi

args=(bond fetch --source six)
if (( have_f )); then
	args+=(--file $input)
	(( $# == 0 )) || args+=(--issuer $1)
else
	args+=(--issuer ${1:-Glarner})
fi
print -u2 "$prog: deprecated, use: fic ${args[*]}"
exec $FIC $args
