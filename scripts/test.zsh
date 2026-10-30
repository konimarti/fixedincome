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

# six-bonds.zsh: bond CSV, missing price is an empty field
$here/six-bonds.zsh -f $here/testdata/six_ref.csv > $tmp/six.csv
check "six header" "$(head -1 $tmp/six.csv)" "maturity,coupon_pct,price_clean,id,issuer"
check "six dates" "$(sed -n 2p $tmp/six.csv)" "2026-05-25,1.25,109.70,CH0224396983,Glarner KB"
check "six empty price" "$(sed -n 3p $tmp/six.csv)" "2030-01-15,0.5,,CH0001,Glarner KB"
check "six zero price is empty" "$(sed -n 4p $tmp/six.csv)" '2029-03-31,0,,CH0002,"Glarner, KB"'
check "six N/A coupon is empty" "$(sed -n 5p $tmp/six.csv)" "2031-06-30,,101.5,CH0003,Other KB"

# price-bonds.zsh: failing rows do not abort, the exit code is 0 unless all fail
out=$($here/price-bonds.zsh -c $curve -d 2021-03-31 -f csv $tmp/six.csv 2>/dev/null)
check "price-bonds exit" $? 0
check "price-bonds rows" "$(print -r -- "$out" | wc -l | tr -d ' ')" 7
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

# avg-spread.zsh: duration-weighted, grouped by issuer, skips rows with status != ok
out=$($here/avg-spread.zsh -c $curve -d 2021-03-31 $bad 2>$tmp/err)
check "avg-spread exit" $? 0
check "avg-spread header" "$(print -r -- "$out" | head -1)" "issuer,n,avg_zspread_bp,avg_mod_duration"
check "avg-spread group with a comma" "$(print -r -- "$out" | grep -c '^"Bank, One",2,')" 1
check "avg-spread second group" "$(print -r -- "$out" | grep -c '^Bank Two,1,')" 1
check "avg-spread skipped" "$(grep -c 'rows skipped' $tmp/err)" 1
# the duration-weighted average of the two-bond group "Bank, One" (A: 0.0290612769bp, duration 4.982157727;
# B: 176.6090327bp, duration 6.853723509), computed by hand:
# (0.0290612769*4.982157727 + 176.6090327*6.853723509) / (4.982157727 + 6.853723509) = 102.2800
cat > $tmp/pick.awk <<'AWK'
NR > 1 { split_csv($0, ",", f); if (f[1] == "Bank, One") printf "%.4f", f[3] }
AWK
got=$(print -r -- "$out" | awk -f $here/lib/csv.awk -f $tmp/pick.awk)
check "avg-spread weighted value (hard-coded)" "$got" "102.2800"
check "avg-spread second group excludes the bond without a market price" "$(print -r -- "$out" | grep -c '^Bank Two,1,')" 1
# without an issuer column all bonds form one group
out=$($here/avg-spread.zsh -c $curve -d 2021-03-31 $here/../data/bonds_2021-11-26.csv 2>/dev/null)
check "avg-spread no issuer column" "$(print -r -- "$out" | tail -1 | cut -d, -f1,2)" "all,39"

if (( fails > 0 )); then print "$fails check(s) failed"; exit 1; fi
print "all checks passed"
