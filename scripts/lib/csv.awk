# Helpers for the scripts: CSV parsing by column name.
#
# split_csv(line, sep, out): splits a CSV line into out[1..n], honouring
#   double quotes ("" is a quote inside a quoted field); returns n.
# trim(s): removes surrounding blanks and carriage returns.
# csv_quote(s): quotes a field if it contains a comma, quote or line break.
function split_csv(line, sep, out,    n, i, c, field, inq, len) {
	split("", out)
	n = 0; field = ""; inq = 0; len = length(line)
	for (i = 1; i <= len; i++) {
		c = substr(line, i, 1)
		if (inq) {
			if (c == "\"") {
				if (substr(line, i + 1, 1) == "\"") { field = field "\""; i++ }
				else inq = 0
			} else field = field c
		} else if (c == "\"") inq = 1
		else if (c == sep) { out[++n] = field; field = "" }
		else field = field c
	}
	out[++n] = field
	return n
}
function trim(s) {
	gsub(/^[ \t\r]+|[ \t\r]+$/, "", s)
	return s
}
function csv_quote(s) {
	if (s ~ /[,"\r\n]/) { gsub(/"/, "\"\"", s); return "\"" s "\"" }
	return s
}
