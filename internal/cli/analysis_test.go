package cli

import (
	"encoding/xml"
	"html"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const badBondsFile = "../../scripts/testdata/bonds_bad.csv"

// flatCurve writes a flat 1% curve file and returns its path.
func flatCurve(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "flat.json")
	if err := os.WriteFile(p, []byte(mustOK(t, "curve", "template", "--model", "flat")), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func parseF(t *testing.T, s string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("number %q: %v", s, err)
	}
	return v
}

func TestCompare(t *testing.T) {
	dir := t.TempDir()
	flat := flatCurve(t, dir)
	args := []string{"curve", "compare", curveFile, flat, "--t", "1y,2y,5y,10y"}
	code, out, errOut := run(t, args...)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	golden(t, "curve_compare_csv", out)
	// the asof dates differ (the flat curve has none): no warning; the summary is on stderr
	if !strings.Contains(errOut, "diff_zero_bp (B minus A) at 4 maturities") || strings.Contains(errOut, "warning") {
		t.Errorf("stderr: %q", errOut)
	}

	// independent check against curve show of A and B (percent and bp)
	rows := tableRows(t, out)
	sa := tableRows(t, mustOK(t, "curve", "show", "--curve", curveFile, "--t", "1y,2y,5y,10y"))
	sb := tableRows(t, mustOK(t, "curve", "show", "--curve", flat, "--t", "1y,2y,5y,10y"))
	var sum, sumSq float64
	for i, r := range rows {
		za, zb := parseF(t, sa[i]["zero_cc_pct"]), parseF(t, sb[i]["zero_cc_pct"])
		fa, fb := parseF(t, sa[i]["inst_fwd_cc_pct"]), parseF(t, sb[i]["inst_fwd_cc_pct"])
		for col, want := range map[string]float64{"a_zero_cc_pct": za, "b_zero_cc_pct": zb, "diff_zero_bp": (zb - za) * 100,
			"a_fwd_cc_pct": fa, "b_fwd_cc_pct": fb, "diff_fwd_bp": (fb - fa) * 100} {
			if got := parseF(t, r[col]); math.Abs(got-want) > 1e-7*math.Max(1, math.Abs(want)) {
				t.Errorf("row %d %s = %v, want %v", i, col, got, want)
			}
		}
		d := (zb - za) * 100
		sum += d
		sumSq += d * d
	}
	// the summary of the differences
	want := "mean " + strconv.FormatFloat(sum/4, 'g', 4, 64) + ","
	if !strings.Contains(errOut, want) || !strings.Contains(errOut, "rms "+strconv.FormatFloat(math.Sqrt(sumSq/4), 'g', 4, 64)) {
		t.Errorf("summary %q, want %q and the rms", errOut, want)
	}

	// B = A plus a spread of 50bp in the file: the difference is 50bp everywhere
	b := filepath.Join(dir, "spread.json")
	data := strings.Replace(string(mustRead(t, curveFile)), `"spread_bp": 0`, `"spread_bp": 50`, 1)
	if err := os.WriteFile(b, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	// flags before, between and after the files
	for _, a := range [][]string{{"--t", "1y,7y", curveFile, b}, {curveFile, "--t", "1y,7y", b}, {curveFile, b, "--t", "1y,7y"}} {
		for _, r := range tableRows(t, mustOK(t, append([]string{"curve", "compare"}, a...)...)) {
			if d := parseF(t, r["diff_zero_bp"]); math.Abs(d-50) > 1e-6 {
				t.Errorf("diff_zero_bp %v, want 50", d)
			}
			if d := parseF(t, r["diff_fwd_bp"]); math.Abs(d-50) > 1e-6 {
				t.Errorf("diff_fwd_bp %v, want 50", d)
			}
		}
	}

	// the default grid 1m:10y:1m
	if n := len(tableRows(t, mustOK(t, "curve", "compare", curveFile, b))); n != 120 {
		t.Errorf("default grid has %d rows, want 120", n)
	}

	// differing asof dates warn
	fit := filepath.Join(dir, "fit.json")
	mustOK(t, "curve", "fit", "--bonds", bondsFile, "--date", "2021-11-26", "-o", fit)
	if _, _, errOut := run(t, "curve", "compare", curveFile, fit, "--t", "1y"); !strings.Contains(errOut, "asof dates differ") {
		t.Errorf("no asof warning: %q", errOut)
	}

	// usage errors
	for _, a := range [][]string{{"curve", "compare"}, {"curve", "compare", curveFile}, {"curve", "compare", curveFile, b, flat},
		{"curve", "compare", curveFile, b, "--grid", "1y:2y:1y", "--t", "1y"}, {"curve", "compare", "-", "-"},
		{"curve", "compare", curveFile, b, "-o", b}, {"curve", "compare", curveFile, b, "--format", "x"}} {
		if code, _, _ := run(t, a...); code != 2 {
			t.Errorf("%v: exit %d, want 2", a, code)
		}
	}
	if code, _, _ := run(t, "curve", "compare", curveFile, filepath.Join(dir, "missing.json")); code != 1 {
		t.Errorf("missing file: exit %d, want 1", code)
	}
	// -o is atomic and stdin works for one of the files
	o := filepath.Join(dir, "cmp.csv")
	code, _, errOut = runIn(t, string(mustRead(t, curveFile)), "curve", "compare", "-", b, "--t", "1y", "-o", o)
	if code != 0 || !strings.HasPrefix(string(mustRead(t, o)), "t_years,") {
		t.Errorf("-o: %d %s", code, errOut)
	}
}

func TestGroupBy(t *testing.T) {
	base := []string{"bond", "price", "--curve", curveFile, "--date", "2021-03-31", "--in", badBondsFile}
	grouped := func(col string, extra ...string) (string, string) {
		code, out, errOut := run(t, append(append(append([]string{}, base...), "--group-by", col), extra...)...)
		if code != 0 {
			t.Fatalf("exit %d: %s", code, errOut)
		}
		return out, errOut
	}
	out, errOut := grouped("issuer", "--format", "csv")
	golden(t, "bond_group_issuer_csv", out)
	o2, _ := grouped("daycount", "--format", "json")
	golden(t, "bond_group_daycount_json", o2)
	if !strings.Contains(errOut, "5 of 8 rows not in the groups") {
		t.Errorf("stderr: %q", errOut)
	}

	// hand-computed: A (0.0290612769bp, duration 4.982157727) and B (176.6090327bp, duration 6.853723509)
	// of "Bank, One": (0.0290612769*4.982157727 + 176.6090327*6.853723509) / (4.982157727 + 6.853723509) = 102.2800
	rows := tableRows(t, out)
	if len(rows) != 2 || rows[0]["issuer"] != "Bank, One" || rows[1]["issuer"] != "Bank Two" {
		t.Fatalf("rows (sorted by spread): %v", rows)
	}
	if got := parseF(t, rows[0]["avg_zspread_bp"]); math.Abs(got-102.2800) > 5e-5 {
		t.Errorf("avg_zspread_bp %v, want 102.2800", got)
	}
	if got := parseF(t, rows[0]["avg_mod_duration"]); math.Abs(got-(4.982157727+6.853723509)/2) > 1e-8 {
		t.Errorf("avg_mod_duration %v", got)
	}
	if rows[0]["n"] != "2" || rows[1]["n"] != "1" {
		t.Errorf("n: %v", rows)
	}

	// independent: aggregate the per-bond batch output (rows with a market price and status ok)
	var zd, d, dv, n float64
	for _, r := range tableRows(t, mustOK(t, append(append([]string{}, base...), "--format", "csv")...)) {
		if r["status"] != "ok" || r["price_source"] != "market" || r["issuer"] != "Bank Two" {
			continue
		}
		z, dur := parseF(t, r["zspread_bp"]), parseF(t, r["mod_duration"])
		zd, d, dv, n = zd+z*dur, d+dur, dv+parseF(t, r["dv01"]), n+1
	}
	for col, want := range map[string]float64{"avg_zspread_bp": zd / d, "avg_mod_duration": d / n, "dv01_sum": dv, "n": n} {
		if got := parseF(t, rows[1][col]); math.Abs(got-want) > 1e-8*math.Max(1, math.Abs(want)) {
			t.Errorf("Bank Two %s = %v, want %v", col, got, want)
		}
	}

	// freq groups: all bonds of the batch have the default frequency 1
	for _, r := range tableRows(t, mustOK(t, append(append([]string{}, base...), "--group-by", "freq", "--format", "csv")...)) {
		if r["freq"] != "1" || r["n"] != "3" {
			t.Errorf("freq group %v", r)
		}
	}
	code, text, _ := run(t, append(append([]string{}, base...), "--group-by", "issuer", "--format", "text")...)
	if code != 0 || !strings.HasPrefix(text, "issuer") || !strings.Contains(text, "avg_zspread_bp") {
		t.Errorf("text: %q", text)
	}

	// errors: invalid column and no --in (usage), no issuer column (runtime)
	for _, a := range [][]string{append(append([]string{}, base...), "--group-by", "maturity"),
		{"bond", "price", "--curve", curveFile, "--maturity", "5y", "--coupon", "1", "--group-by", "issuer"}} {
		if code, _, _ := run(t, a...); code != 2 {
			t.Errorf("%v: exit %d, want 2", a, code)
		}
	}
	code, _, errOut = run(t, "bond", "price", "--curve", curveFile, "--in", bondsFile, "--group-by", "issuer")
	if code != 1 || !strings.Contains(errOut, "needs an issuer column") {
		t.Errorf("no issuer column: %d %q", code, errOut)
	}
}

var (
	svgRe = regexp.MustCompile(`(?s)<svg.*?</svg>`)
	rowRe = regexp.MustCompile(`<tr><td class="l">(nss|spline)</td><td class="l">([^<]*)</td><td>[^<]*</td><td>([^<]*)</td><td>([^<]*)</td><td>([^<]*)</td><td>([^<]*)</td></tr>`)
)

func TestFitReport(t *testing.T) {
	dir := t.TempDir()
	evil := `<script>alert("x")</script>`
	bonds := filepath.Join(dir, "bonds<&>.csv")
	data := string(mustRead(t, bondsFile)) + "2030-01-15,1,100.5,\"" + strings.ReplaceAll(evil, `"`, `""`) + "\"\n"
	if err := os.WriteFile(bonds, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	fitArgs := func(rep, resid string) []string {
		return []string{"curve", "fit", "--bonds", bonds, "--date", "2021-11-26", "--onrate", "-0.75", "--model", "both",
			"-o", filepath.Join(dir, "fit.json"), "--report", rep, "--residuals", resid}
	}
	rep1, rep2, resid := filepath.Join(dir, "r1.html"), filepath.Join(dir, "r2.html"), filepath.Join(dir, "res.csv")
	if code, _, errOut := run(t, fitArgs(rep1, resid)...); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if code, _, errOut := run(t, fitArgs(rep2, filepath.Join(dir, "res2.csv"))...); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	h := string(mustRead(t, rep1))
	if h != string(mustRead(t, rep2)) {
		t.Error("the report is not deterministic")
	}

	// structure: two SVG plots that are well-formed XML, no script, no external assets
	svgs := svgRe.FindAllString(h, -1)
	if strings.Count(h, "<svg") != 2 || len(svgs) != 2 {
		t.Fatalf("%d svg elements", strings.Count(h, "<svg"))
	}
	for _, s := range svgs {
		d := xml.NewDecoder(strings.NewReader(s))
		for {
			if _, err := d.Token(); err != nil {
				if err.Error() != "EOF" {
					t.Errorf("svg is not well-formed: %v", err)
				}
				break
			}
		}
	}
	if !strings.HasPrefix(h, "<!DOCTYPE html>") || !strings.HasSuffix(strings.TrimSpace(h), "</html>") {
		t.Error("not a complete HTML document")
	}
	for _, bad := range []string{"<script", "src=", "href=", "http://www.w3.org/1999/xlink", "@import", "url(http"} {
		if strings.Contains(h, bad) {
			t.Errorf("report contains %q", bad)
		}
	}
	// untrusted text is escaped (bond id, file name)
	if !strings.Contains(h, "&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;") || strings.Contains(h, evil) {
		t.Error("bond id is not escaped")
	}
	if !strings.Contains(h, "bonds&lt;&amp;&gt;.csv") {
		t.Error("file name is not escaped")
	}
	// summary: as-of date, both models, warnings
	for _, s := range []string{"2021-11-26", "nss", "spline", "yield error RMSE", "Fit warnings", "dropped", "prefers-color-scheme"} {
		if !strings.Contains(h, s) {
			t.Errorf("report lacks %q", s)
		}
	}

	// the table agrees with the residuals file
	want := map[string]map[string]string{}
	for _, r := range tableRows(t, string(mustRead(t, resid))) {
		want[r["model"]+"/"+r["id"]] = r
	}
	got := rowRe.FindAllStringSubmatch(h, -1)
	if len(got) != len(want) || len(got) < 60 {
		t.Fatalf("%d table rows, %d residuals", len(got), len(want))
	}
	for _, m := range got {
		r, ok := want[m[1]+"/"+html.UnescapeString(m[2])]
		if !ok {
			t.Errorf("unknown row %v", m[1:3])
			continue
		}
		for i, col := range []string{"ttm_years", "yield_cc_market_pct", "yield_cc_model_pct", "error_bp"} {
			if a, b := parseF(t, m[3+i]), parseF(t, r[col]); math.Abs(a-b) > 6e-4 {
				t.Errorf("%s/%s %s: report %v, csv %v", m[1], m[2], col, a, b)
			}
		}
	}

	// the plot points are inside the plot area
	for _, pt := range regexp.MustCompile(`class="pt" cx="([\d.]+)" cy="([\d.]+)"`).FindAllStringSubmatch(svgs[1], -1) {
		x, y := parseF(t, pt[1]), parseF(t, pt[2])
		if (x < 56 || x > 704 || y < 30 || y > 296) && y != 14 {
			t.Errorf("point (%v, %v) outside the plot area", x, y)
		}
	}
}

func TestFitReportOptions(t *testing.T) {
	dir := t.TempDir()
	rep := filepath.Join(dir, "r.html")
	// a single model, to stdout with the curve in a file
	code, out, errOut := run(t, "curve", "fit", "--bonds", bondsFile, "--date", "2021-11-26", "--model", "spline", "-o", filepath.Join(dir, "c.json"), "--report", "-")
	if code != 0 || !strings.HasPrefix(out, "<!DOCTYPE html>") || strings.Contains(out, ">Model<") {
		t.Errorf("--report -: %d %.60q %s", code, out, errOut)
	}
	// usage errors: stdout clash, the report is an input or another output
	for _, a := range [][]string{
		{"--report", "-"},
		{"-o", rep, "--report", rep},
		{"-o", filepath.Join(dir, "c.json"), "--residuals", rep, "--report", rep},
		{"-o", filepath.Join(dir, "c.json"), "--report", bondsFile},
	} {
		args := append([]string{"curve", "fit", "--bonds", bondsFile, "--date", "2021-11-26"}, a...)
		if code, _, _ := run(t, args...); code != 2 {
			t.Errorf("%v: exit %d, want 2", a, code)
		}
	}
	// an existing report survives a failing run; no temporary files remain
	if err := os.WriteFile(rep, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, _ = run(t, "curve", "fit", "--bonds", "/nonexistent.csv", "--report", rep, "-o", filepath.Join(dir, "c2.json"))
	if code != 1 || string(mustRead(t, rep)) != "old" {
		t.Errorf("failing run: exit %d, report %q", code, mustRead(t, rep))
	}
	if m, _ := filepath.Glob(filepath.Join(dir, ".*tmp*")); len(m) > 0 {
		t.Errorf("temporary files: %v", m)
	}
	// a bad report path fails before the fit
	if code, _, _ := run(t, "curve", "fit", "--bonds", bondsFile, "--report", filepath.Join(dir, "no", "r.html")); code != 1 {
		t.Errorf("bad report path: exit %d", code)
	}
}
