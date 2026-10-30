#!/bin/zsh

# input is expected on stdin as comma separated values (csv) with the maturity date (YYYY-MM-DD), coupon rate in % (2.0) and the quoted (clean) price of the bond
# Example: six-bonds.zsh Glarner | price-bonds.zsh
#
# Header rows and rows with an invalid, missing or zero price are skipped
# (bonds-cli would use the model price for -quote 0).

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

errfile=$(mktemp /tmp/bonds-script-err.XXXXXX)

while IFS=',' read -r maturity coupon price rest
do
	maturity=$(clean "$maturity")
	coupon=$(clean "$coupon")
	price=$(clean "$price")
	if [[ ! "$maturity" =~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' ]] || [[ ! "$coupon" =~ '^-?[0-9]*\.?[0-9]+$' ]]; then
		echo "skipping row: $maturity,$coupon,$price" >&2
		continue
	fi
	if [[ ! "$price" =~ '^[0-9]*\.?[0-9]+$' ]] || (( price == 0 )); then
		echo "skipping bond maturing $maturity: no price" >&2
		continue
	fi
	if ! data=$(bonds-cli -maturity "$maturity" -coupon "$coupon" -quote "$price" 2>$errfile); then
		echo "skipping bond maturing $maturity: $(grep -v -m 1 "^Term model read" $errfile)" >&2
		continue
	fi
	echo "####"
	echo "$data" | sed -n '/Yields/,${p};/duration/p;/Coupon/p'
done

rm $errfile
