#!/bin/zsh

# input is expected on stdin as comma separated values (csv) with the maturity date (YYYY-MM-DD), coupon rate in % (2.0), the quoted (clean) price of the bond and the issuer name
# Example: six-bonds.zsh Kantonalbank | avg-spread.zsh
#
# Header rows and rows with an invalid, missing or zero price are skipped
# (bonds-cli would use the model price for -quote 0 and report an implied
# spread of ~0).

setopt extendedglob

# clean removes carriage returns, surrounding whitespace and quotes
clean() {
	local v=${1//$'\r'/}
	v=${v##[[:space:]]#}
	v=${v%%[[:space:]]#}
	v=${v#\"}
	v=${v%\"}
	print -r -- "$v"
}

tmpfile=$(mktemp /tmp/bonds-script.XXXXXX)
errfile=$(mktemp /tmp/bonds-script-err.XXXXXX)

while IFS=',' read -r maturity coupon price name
do
	maturity=$(clean "$maturity")
	coupon=$(clean "$coupon")
	price=$(clean "$price")
	name=$(clean "$name")
	if [[ ! "$maturity" =~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' ]] || [[ ! "$coupon" =~ '^-?[0-9]*\.?[0-9]+$' ]]; then
		echo "skipping row: $maturity,$coupon,$price,$name" >&2
		continue
	fi
	if [[ ! "$price" =~ '^[0-9]*\.?[0-9]+$' ]] || (( price == 0 )); then
		echo "skipping bond maturing $maturity ($name): no price" >&2
		continue
	fi
	if ! data=$(bonds-cli -maturity "$maturity" -coupon "$coupon" -quote "$price" 2>$errfile); then
		echo "skipping bond maturing $maturity ($name): $(grep -v -m 1 "^Term model read" $errfile)" >&2
		continue
	fi
	implied=$(echo $data | awk '/ Implied/{print $3}')
	duration=$(echo $data | awk '/duration/{print $3}')
	name=${name// /_}
	if [ ! -z "$implied" ]; then
		echo ${name:-unknown} $implied $duration >> $tmpfile
	fi
done

awk '{ctr[$1]++;spread[$1]+=$2;dur[$1]+=$3}END{for (key in ctr) {printf "%15s\t%3.2f\t%3.2f\n", substr(key,1,15), spread[key]/ctr[key], dur[key]/ctr[key]}}' $tmpfile | sort -n -k2

rm $tmpfile $errfile
