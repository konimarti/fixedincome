#!/bin/zsh

# input is expected on stdin as comma separated values (csv) with the maturity date (YYYY-MM-DD), coupon rate in % (2.0) and the quoted (clean) price of the bond
# Example: six-bonds.zsh Glarner | price-bonds.zsh

awk -F',' '{print "bonds-cli -maturity "$1" -coupon "$2" -quote "$3}' | while read line
do  
	echo "####"
	echo $line | sh | sed -n '/Yields/,${p};/duration/p;/Coupon/p'
done
