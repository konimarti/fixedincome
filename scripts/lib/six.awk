# Converts the SIX CSV (semicolon separated, header with column names) to the
# bond CSV of fic (see six-bonds.zsh). Requires csv.awk.
function number(s) {
	s = trim(s)
	return (s ~ /^[-+]?[0-9]*\.?[0-9]+([eE][-+]?[0-9]+)?$/) ? s : ""
}
BEGIN { hdr = 0; rows = 0 }
{
	line = trim($0)
	if (line == "") next
	n = split_csv(line, ";", f)
	if (!hdr) {
		for (i = 1; i <= n; i++) {
			name = tolower(trim(f[i]))
			if (name == "maturitydate") cm = i
			else if (name == "couponrate") cc = i
			else if (name == "closingprice") cp = i
			else if (name == "issuernameshort") ci = i
			else if (name == "isin") cn = i
		}
		if (!(cm && cc && cp)) {
			print "six-bonds: the header must have the columns MaturityDate, CouponRate, ClosingPrice" > "/dev/stderr"
			exit 1
		}
		hdr = 1
		print "maturity,coupon_pct,price_clean,id,issuer"
		next
	}
	mat = trim(f[cm])
	if (mat ~ /^[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]$/) mat = substr(mat, 1, 4) "-" substr(mat, 5, 2) "-" substr(mat, 7, 2)
	price = number(f[cp])
	if (price != "" && price + 0 == 0) price = ""
	rows++
	printf "%s,%s,%s,%s,%s\n", csv_quote(mat), number(f[cc]), price, csv_quote(cn ? trim(f[cn]) : ""), csv_quote(ci ? trim(f[ci]) : "")
}
END { if (!hdr) { print "six-bonds: no header row" > "/dev/stderr"; exit 1 } }
