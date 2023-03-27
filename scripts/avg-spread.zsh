#!/bin/zsh

# input is expected on stdin as comma separated values (csv) with the maturity date (YYYY-MM-DD), coupon rate in % (2.0) and the quoted (clean) price of the bond
# Example: six-bonds.zsh Kantonalbank | avg-spread.zsh

tmpfile=$(mktemp /tmp/bonds-script.XXXXXX)

while read line
do  
	data=$(echo $line | awk -F',' '{print "bonds-cli -maturity "$1" -coupon "$2" -quote "$3}' | sh)
	implied=$(echo $data | awk '/ Implied/{print $3}')
	duration=$(echo $data | awk '/duration/{print $3}')
	name=$(echo $line | awk -F',' '{print $4}' | sed 's/ /_/g')
	if [ ! -z "$implied" ]; then
		echo $name $implied $duration >> $tmpfile
	fi
done

awk '{ctr[$1]++;spread[$1]+=$2;dur[$1]+=$3}END{for (key in ctr) {printf "%15s\t%3.2f\t%3.2f\n", substr(key,1,15), spread[key]/ctr[key], dur[key]/ctr[key]}}' $tmpfile | sort -n -k2

rm $tmpfile
