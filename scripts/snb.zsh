#!/bin/zsh
#
# Writes the Nelson-Siegel-Svensson curve of the Swiss National Bank (SNB)
# as a curve file (schema of fic, see README: betas in percent) for
# "fic curve show/fit/..." and "fic bond price --curve".
#
#   snb.zsh [-f FILE] [-o curve.json]
#
#   -f FILE   read the SNB CSV from FILE ("-": stdin) instead of downloading
#             https://data.snb.ch/api/cube/rendopar/data/csv/en
#   -o FILE   output file (default: stdout)
#
# The CSV (semicolon separated, a few header lines, then a header row with
# the columns Date, D0 and Value) is parsed by column name; the as-of date is
# the latest date for which all six parameters (B0..B3, T1, T2) are
# available. Exit code 1 if the parameters cannot be found.

set -u
setopt pipefail
here=${0:A:h}
prog=${0:t}

usage() { print -u2 "usage: $prog [-f FILE] [-o curve.json]"; exit 2 }

input="" output="-"
while getopts "f:o:h" opt; do
	case $opt in
		f) input=$OPTARG ;;
		o) output=$OPTARG ;;
		*) usage ;;
	esac
done
shift $((OPTIND - 1))
(( $# == 0 )) || usage

if [[ -z $input ]]; then
	data=$(curl -fsS "https://data.snb.ch/api/cube/rendopar/data/csv/en") || { print -u2 "snb: download failed"; exit 1 }
elif [[ $input == - ]]; then
	data=$(cat)
else
	data=$(<"$input" 2>/dev/null) || { print -u2 "snb: cannot read $input"; exit 1 }
fi

# prints "date b0 b1 b2 b3 t1 t2" of the latest date with all six parameters
params=$(print -r -- "$data" | awk -f $here/lib/csv.awk -f $here/lib/snb.awk) || exit 1

set -- ${=params}
json=$(printf '{"model": "nss", "asof": "%s", "currency": "CHF", "source": "SNB",\n "params": {"b0": %s, "b1": %s, "b2": %s, "b3": %s, "t1": %s, "t2": %s},\n "spread_bp": 0}\n' "$@")
if [[ $output == - ]]; then
	print -r -- "$json"
else
	print -r -- "$json" > "$output" || exit 1
fi
