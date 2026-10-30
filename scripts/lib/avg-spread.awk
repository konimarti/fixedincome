# Aggregates the CSV of "fic bond price --format csv" per issuer (see
# avg-spread.zsh). Requires csv.awk.
BEGIN { hdr = 0 }
{
	line = trim($0)
	if (line == "") next
	n = split_csv(line, ",", f)
	if (!hdr) {
		for (i = 1; i <= n; i++) col[tolower(trim(f[i]))] = i
		if (!(("zspread_bp" in col) && ("mod_duration" in col) && ("status" in col))) {
			print "avg-spread: missing columns zspread_bp, mod_duration, status in the output of fic" > "/dev/stderr"
			exit 1
		}
		hdr = 1
		next
	}
	if (trim(f[col["status"]]) != "ok") { skipped++; next }
	# bonds without a market price have no z-spread
	if (("price_source" in col) && trim(f[col["price_source"]]) != "market") { skipped++; next }
	g = ("issuer" in col) ? trim(f[col["issuer"]]) : "all"
	if (g == "") g = "unknown"
	z = f[col["zspread_bp"]] + 0
	d = f[col["mod_duration"]] + 0
	cnt[g]++; wsum[g] += z * d; dsum[g] += d
}
END {
	if (!hdr) exit 1
	print "issuer,n,avg_zspread_bp,avg_mod_duration"
	if (skipped) printf "avg-spread: %d rows skipped (status not ok or no market price)\n", skipped > "/dev/stderr"
	m = 0
	for (g in cnt) {
		avg[g] = (dsum[g] > 0) ? wsum[g] / dsum[g] : 0
		# insertion sort by the spread
		i = ++m
		while (i > 1 && avg[order[i - 1]] > avg[g]) { order[i] = order[i - 1]; i-- }
		order[i] = g
	}
	for (i = 1; i <= m; i++) {
		g = order[i]
		printf "%s,%d,%.10g,%.10g\n", csv_quote(g), cnt[g], avg[g], dsum[g] / cnt[g]
	}
}
