# Extracts the latest complete NSS parameter set from the SNB CSV (see snb.zsh).
# Prints: date b0 b1 b2 b3 t1 t2. Requires csv.awk.
BEGIN { hdr = 0 }
{
	line = trim($0)
	if (line == "") next
	n = split_csv(line, ";", f)
	if (!hdr) {
		# the header row names the columns Date, D0 and Value
		for (i = 1; i <= n; i++) {
			name = tolower(trim(f[i]))
			if (name == "date") cd = i
			else if (name == "d0") cn = i
			else if (name == "value") cv = i
		}
		if (cd && cn && cv) hdr = 1
		next
	}
	key = tolower(trim(f[cn]))
	sub(/^beta/, "b", key); sub(/^tau/, "t", key)
	if (key !~ /^(b[0-3]|t[12])$/) next
	val = trim(f[cv])
	if (val !~ /^[-+]?[0-9]*\.?[0-9]+([eE][-+]?[0-9]+)?$/) next
	d = trim(f[cd])
	if (d !~ /^[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]/) next
	d = substr(d, 1, 10)
	if (!((d SUBSEP key) in v)) cnt[d]++
	v[d, key] = val
}
END {
	if (!hdr) { print "snb: no header row with the columns Date, D0, Value" > "/dev/stderr"; exit 1 }
	best = ""
	for (d in cnt) if (cnt[d] == 6 && d > best) best = d
	if (best == "") { print "snb: no date with all parameters B0..B3, T1, T2" > "/dev/stderr"; exit 1 }
	printf "%s %s %s %s %s %s %s\n", best, v[best, "b0"], v[best, "b1"], v[best, "b2"], v[best, "b3"], v[best, "t1"], v[best, "t2"]
}
