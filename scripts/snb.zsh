#!/bin/zsh
#
# DEPRECATED: use "fic curve fetch --source snb" (this script is a thin
# wrapper around it).
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
# The as-of date is the latest date for which all six parameters are
# available. Exit code 1 if the parameters cannot be found. The fic binary
# is taken from $FIC (default: fic from the PATH).

set -u
prog=${0:t}
: ${FIC:=fic}

usage() { print -u2 "usage: $prog [-f FILE] [-o curve.json]"; exit 2 }

input="" output="-" have_f=0
while getopts "f:o:h" opt; do
	case $opt in
		f) input=$OPTARG; have_f=1 ;;
		o) output=$OPTARG ;;
		*) usage ;;
	esac
done
shift $((OPTIND - 1))
(( $# == 0 )) || usage
(( have_f == 0 )) || [[ -n $input ]] || usage
if ! command -v $FIC >/dev/null 2>&1; then
	print -u2 "$prog: fic not found; build it with: go build ./cmds/fic and put it on PATH or set FIC"
	exit 1
fi

print -u2 "$prog: deprecated, use: fic curve fetch --source snb [--file FILE] [-o FILE]"
args=(curve fetch --source snb -o $output)
(( have_f == 0 )) || args+=(--file $input)
exec $FIC $args
