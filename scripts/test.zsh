#!/bin/zsh
#
# Tests the scripts with the fixtures in testdata/ (no network).
#
#   FIC=/path/to/fic scripts/test.zsh        (default: fic from the PATH)
#
# Exit code 0 if all checks pass.

set -u
here=${0:A:h}
: ${FIC:=fic}
export FIC
curve=$here/../data/curve_snb_2021-03-31.json
tmp=$(mktemp -d) || exit 1
trap 'rm -rf $tmp' EXIT
fails=0
check() { # description, actual, expected
	if [[ "$2" == "$3" ]]; then print "ok   $1"; else print "FAIL $1: got '$2', want '$3'"; fails=$((fails + 1)); fi
}

# snb.zsh: latest complete date, header-aware, CRLF
$here/snb.zsh -f $here/testdata/snb_rendopar.csv -o $tmp/snb.json
check "snb exit" $? 0
check "snb asof" "$(grep -o '"asof": "[^"]*"' $tmp/snb.json)" '"asof": "2021-03-31"'
check "snb curve loads" "$($FIC curve show --curve $tmp/snb.json --t 10y --format csv | tail -1 | cut -d, -f1,2)" "10,-0.267259555"
check "snb equals data curve" "$($FIC curve show --curve $tmp/snb.json --t 1y,5y --format csv)" "$($FIC curve show --curve $curve --t 1y,5y --format csv)"
$here/snb.zsh -f /dev/null 2>/dev/null; check "snb empty input fails" $? 1
$here/snb.zsh -f "" 2>/dev/null; check "snb empty -f is a usage error" $? 2
FIC=/nonexistent/fic $here/snb.zsh -f $here/testdata/snb_live_sample.csv >/dev/null 2>$tmp/nofic; check "snb without fic exits 1" $? 1
check "snb without fic message" "$(grep -c 'fic not found; build it with' $tmp/nofic)" 1
check "snb deprecation note" "$($here/snb.zsh -f $here/testdata/snb_rendopar.csv 2>&1 >/dev/null | grep -c deprecated)" 1
# live format of data.snb.ch: BOM, CRLF, lowercase codes, latest complete date
$here/snb.zsh -f $here/testdata/snb_live_sample.csv -o $tmp/live.json 2>/dev/null
check "snb live sample exit" $? 0
check "snb live sample asof" "$(grep -o '"asof": "[^"]*"' $tmp/live.json)" '"asof": "2025-07-31"'
check "snb live sample b3 in percent" "$(grep -o '"b3": [^,]*' $tmp/live.json)" '"b3": 3.14724'
check "snb live sample stdin" "$($here/snb.zsh -f - < $here/testdata/snb_live_sample.csv 2>/dev/null | grep -c '"currency": "CHF"')" 1

# six-bonds.zsh (deprecated wrapper of "fic bond fetch --source six"): bond CSV,
# missing price is an empty field; the old fixture has no freq / day count /
# rating columns, those fields stay empty; the row with the coupon N/A is skipped
$here/six-bonds.zsh -f $here/testdata/six_ref.csv > $tmp/six.csv 2>$tmp/six.err
check "six exit" $? 0
check "six deprecation note" "$(grep -c 'deprecated, use: fic bond fetch --source six' $tmp/six.err)" 1
check "six invalid coupon is reported" "$(grep -c 'invalid coupon' $tmp/six.err)" 1
check "six header" "$(head -1 $tmp/six.csv)" "maturity,coupon_pct,price_clean,id,freq,daycount,issuer,rating"
check "six rows" "$(wc -l < $tmp/six.csv | tr -d ' ')" 6
check "six dates" "$(sed -n 2p $tmp/six.csv)" "2023-01-01,2,100.5,CH0005,,,Other KB,"
check "six empty price" "$(grep CH0001 $tmp/six.csv)" "2030-01-15,0.5,,CH0001,,,Glarner KB,"
check "six zero price is empty" "$(grep CH0002 $tmp/six.csv)" '2029-03-31,0,,CH0002,,,"Glarner, KB",'
check "six stdin" "$($here/six-bonds.zsh -f - < $here/testdata/six_ref.csv 2>/dev/null | wc -l | tr -d ' ')" 6
check "six issuer filter on a file" "$($here/six-bonds.zsh -f $here/testdata/six_ref.csv glarner 2>/dev/null | tail -n +2 | cut -d, -f4 | tr '\n' ' ')" "CH0224396983 CH0002 CH0001 "
$here/six-bonds.zsh -f $here/testdata/snb_rendopar.csv >/dev/null 2>&1; check "six wrong file fails" $? 1
$here/six-bonds.zsh -f "" 2>/dev/null; check "six empty -f is a usage error" $? 2
$here/six-bonds.zsh a b 2>/dev/null; check "six two issuers is a usage error" $? 2
FIC=/nonexistent/fic $here/six-bonds.zsh -f $here/testdata/six_ref.csv >/dev/null 2>$tmp/nofic; check "six without fic exits 1" $? 1
# the new fixture of the library: freq, day count and rating columns
$here/six-bonds.zsh -f $here/../pkg/source/testdata/six_ref.csv > $tmp/six2.csv 2>/dev/null
check "six full fixture" "$(sed -n 2p $tmp/six2.csv)" '2027-12-31,2.25,100.5,CH0000000008,1,BONDBASIS,"Bank ""X""; AG",AAA'
check "six full fixture rows" "$(wc -l < $tmp/six2.csv | tr -d ' ')" 7

# price-bonds.zsh: failing rows do not abort, the exit code is 0 unless all fail
out=$($here/price-bonds.zsh -c $curve -d 2021-03-31 -f csv $tmp/six.csv 2>/dev/null)
check "price-bonds exit" $? 0
check "price-bonds rows" "$(print -r -- "$out" | wc -l | tr -d ' ')" 6
check "price-bonds ok rows (missing price: valued on the curve)" "$(print -r -- "$out" | grep -c ',ok$')" 5
check "price-bonds model rows" "$(print -r -- "$out" | grep -c ',model,')" 2
check "price-bonds text format" "$($here/price-bonds.zsh -c $curve -d 2021-03-31 $tmp/six.csv 2>/dev/null | head -1 | awk '{print $1, $NF}')" "id status"

# crafted bad rows: BOM, CRLF, quoted fields (also with a comma), N/A, empty
# price, blank line, DD.MM.YYYY, matured, bad date, bad coupon
bad=$here/testdata/bonds_bad.csv
out=$($here/price-bonds.zsh -c $curve -d 2021-03-31 -f csv $bad 2>/dev/null)
check "bad rows exit" $? 0
check "bad rows: 5 ok (A, B, C, D, E)" "$(print -r -- "$out" | grep -c ',ok$')" 5
check "bad rows: 8 data rows" "$(print -r -- "$out" | tail -n +2 | wc -l | tr -d ' ')" 8
check "bad rows: model rows (C, D)" "$(print -r -- "$out" | grep -c ',model,')" 2
check "bad rows: matured" "$(print -r -- "$out" | grep -c 'is not after the settlement date 2021-03-31$')" 1
check "bad rows: bad date" "$(print -r -- "$out" | grep -c 'invalid maturity')" 1
check "bad rows: bad coupon" "$(print -r -- "$out" | grep -c 'invalid coupon')" 1

# only failing rows: exit code 1
printf 'maturity,coupon_pct,price_clean\nnot-a-date,1,100\n' | $here/price-bonds.zsh -c $curve -f csv >/dev/null 2>&1
check "all rows failed: exit 1" $? 1
$here/price-bonds.zsh -c /nonexistent.json $bad >/dev/null 2>&1; check "missing curve: exit 1" $? 1

if (( fails > 0 )); then print "$fails check(s) failed"; exit 1; fi
print "all checks passed"
