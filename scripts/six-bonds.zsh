#!/bin/zsh
#
# Lists CHF bonds of SIX Swiss Exchange as a bond CSV for
#
#   fic bond price --in bonds.csv     fic curve fit --bonds bonds.csv
#
#   six-bonds.zsh [-f FILE] [ISSUER] > bonds.csv
#
#   ISSUER    issuer name (short name) filter, default: Glarner
#   -f FILE   read the SIX CSV from FILE ("-": stdin) instead of downloading
#
# Output (comma separated, header, ISO dates, coupon in percent, clean price
# in percent of face; a missing price is an EMPTY field, not 0):
#
#   maturity,coupon_pct,price_clean,id,issuer
#
# The SIX CSV (semicolon separated) is read by column name: MaturityDate
# (YYYYMMDD), CouponRate, ClosingPrice, IssuerNameShort and ISIN (optional).
#
# Source: https://www.six-group.com/fqs/ref.csv?select=MaturityDate,CouponRate,ClosingPrice,IssuerNameShort,ISIN&where=PortalSegment=BO*TradingBaseCurrency=CHF*IssuerNameShort_~GKB&orderby=MaturityDate&page=1&pagesize=99999
# SARON 3M: https://www.six-group.com/exchanges/downloads/indexdata/h_sar3mc_delayed.csv

set -u
setopt pipefail
here=${0:A:h}
prog=${0:t}

usage() { print -u2 "usage: $prog [-f FILE] [ISSUER]"; exit 2 }

input=""
while getopts "f:h" opt; do
	case $opt in
		f) input=$OPTARG ;;
		*) usage ;;
	esac
done
shift $((OPTIND - 1))
(( $# <= 1 )) || usage
issuer=${1:-Glarner}

if [[ -z $input ]]; then
	# the issuer is URL-encoded by curl (--data-urlencode)
	data=$(curl -fsS -G "https://www.six-group.com/fqs/ref.csv" \
		--data-urlencode "select=MaturityDate,CouponRate,ClosingPrice,IssuerNameShort,ISIN" \
		--data-urlencode "where=PortalSegment=BO*TradingBaseCurrency=CHF*IssuerNameShort_~${issuer}" \
		--data-urlencode "orderby=MaturityDate" --data-urlencode "page=1" --data-urlencode "pagesize=99999") \
		|| { print -u2 "$prog: download failed"; exit 1 }
elif [[ $input == - ]]; then
	data=$(cat)
else
	data=$(<"$input" 2>/dev/null) || { print -u2 "$prog: cannot read $input"; exit 1 }
fi

print -r -- "$data" | awk -f $here/lib/csv.awk -f $here/lib/six.awk
