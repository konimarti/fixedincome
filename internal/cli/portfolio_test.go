package cli

import (
	"encoding/csv"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const positionsFile = "../../data/positions_example.csv"

func riskArgs(extra ...string) []string {
	return append([]string{"portfolio", "risk", "--curve", curveFile, "--in", positionsFile, "--vol", "1.0"}, extra...)
}

func TestPortfolioRiskGolden(t *testing.T) {
	golden(t, "portfolio_risk_text", mustOK(t, riskArgs("--format", "text", "--scenarios", "parallel,irrbb")...))
	golden(t, "portfolio_risk_group_text", mustOK(t, riskArgs("--format", "text", "--group-by", "rating", "--scenarios", "none")...))
	golden(t, "portfolio_risk_positions_csv", mustOK(t, riskArgs("--format", "csv")...))
	golden(t, "portfolio_risk_summary_csv", mustOK(t, riskArgs("--format", "csv", "--section", "summary")...))
	golden(t, "portfolio_risk_krd_csv", mustOK(t, riskArgs("--format", "csv", "--section", "krd", "--krd-tenors", "1y,5y,10y")...))
	golden(t, "portfolio_risk_scenarios_csv", mustOK(t, riskArgs("--format", "csv", "--section", "scenarios", "--scenarios", "irrbb")...))
	golden(t, "portfolio_risk_json", mustOK(t, riskArgs("--format", "json", "--group-by", "issuer", "--shocks", "100")...))
}

func parseCSV(t *testing.T, s string) []map[string]string {
	t.Helper()
	recs, err := csv.NewReader(strings.NewReader(s)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]string
	for _, r := range recs[1:] {
		m := map[string]string{}
		for i, c := range recs[0] {
			m[c] = r[i]
		}
		out = append(out, m)
	}
	return out
}

func f64(t *testing.T, s string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("%q: %v", s, err)
	}
	return v
}

func TestPortfolioRiskConsistency(t *testing.T) {
	pos := parseCSV(t, mustOK(t, riskArgs("--format", "csv")...))
	if len(pos) != 6 {
		t.Fatalf("%d positions", len(pos))
	}
	var dirty, dv01 float64
	krd := map[string]float64{}
	for _, p := range pos {
		v := f64(t, p["mv_dirty"])
		dirty += v
		dv01 += f64(t, p["dv01"])
		sum := 0.0
		for c, x := range p {
			if strings.HasPrefix(c, "krd_") {
				sum += f64(t, x)
				krd[c] += v * f64(t, x)
			}
		}
		d := f64(t, p["mod_duration"])
		if math.Abs(sum-d) > 0.01*d {
			t.Errorf("%s: sum KRD %v, duration %v", p["id"], sum, d)
		}
		if p["id"] == "FUNDING7Y" && !(v < 0 && f64(t, p["dv01"]) > 0 && d > 0) {
			t.Errorf("short position: %v", p)
		}
	}
	sum := parseCSV(t, mustOK(t, riskArgs("--format", "csv", "--section", "summary")...))[0]
	if math.Abs(f64(t, sum["mv_dirty"])-dirty) > 1e-6 || math.Abs(f64(t, sum["dv01"])-dv01) > 1e-6 {
		t.Errorf("summary %v vs sums %v %v", sum, dirty, dv01)
	}
	// groups sum to the total and the portfolio KRD is the money weighted sum
	groups := parseCSV(t, mustOK(t, riskArgs("--format", "csv", "--section", "summary", "--group-by", "issuer")...))
	var gd, gv float64
	for _, g := range groups {
		if g["issuer"] == "TOTAL" {
			continue
		}
		gd += f64(t, g["mv_dirty"])
		gv += f64(t, g["dv01"])
	}
	if math.Abs(gd-dirty) > 1e-6 || math.Abs(gv-dv01) > 1e-6 {
		t.Errorf("groups %v %v vs %v %v", gd, gv, dirty, dv01)
	}
	tot := groups[len(groups)-1]
	if tot["issuer"] != "TOTAL" {
		t.Fatalf("last group row %v", tot)
	}
	for c, w := range krd {
		if math.Abs(f64(t, tot[c])-w/dirty) > 1e-8 {
			t.Errorf("%s: total %v, weighted %v", c, tot[c], w/dirty)
		}
	}
	// the json report is valid json with all sections
	var rep map[string]any
	if err := json.Unmarshal([]byte(mustOK(t, riskArgs("--format", "json", "--group-by", "rating")...)), &rep); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"settlement", "summary", "groups", "krd", "scenarios", "positions"} {
		if _, ok := rep[k]; !ok {
			t.Errorf("json: missing %q", k)
		}
	}
}

func TestPortfolioRiskErrors(t *testing.T) {
	dir := t.TempDir()
	write := func(name, s string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	noNominal := write("nonominal.csv", "maturity,coupon_pct,price_clean,id\n2026-03-31,1,100,A\n")
	mixed := write("mixed.csv", "maturity,coupon_pct,price_clean,id,nominal\n2026-03-31,1,100,GOOD,1000\n"+
		"2026-03-31,1,100,ZERO,0\n2026-03-31,1,100,EMPTY,\n2020-03-31,1,100,MATURED,10\nbad,1,100,BADDATE,10\n")
	allBad := write("allbad.csv", "maturity,coupon_pct,price_clean,id,nominal\n2020-03-31,1,100,MATURED,10\n")
	base := []string{"portfolio", "risk", "--curve", curveFile}
	with := func(extra ...string) []string { return append(append([]string{}, base...), extra...) }
	tests := []struct {
		name string
		args []string
		code int
		msg  string
	}{
		{"no in", with(), 2, "--in FILE is required"},
		{"missing nominal column", with("--in", noNominal), 2, "column nominal is required"},
		{"callable without vol", with("--in", positionsFile), 2, "--vol is required"},
		{"bad section", with("--in", positionsFile, "--vol", "1", "--section", "x"), 2, "invalid --section"},
		{"bad group", with("--in", positionsFile, "--vol", "1", "--group-by", "x"), 2, "invalid --group-by"},
		{"bad scenarios", with("--in", positionsFile, "--vol", "1", "--scenarios", "x"), 2, "invalid --scenarios"},
		{"bad shocks", with("--in", positionsFile, "--vol", "1", "--shocks", "0"), 2, "invalid --shocks"},
		{"bad tenors", with("--in", positionsFile, "--vol", "1", "--krd-tenors", "5y,2y"), 2, "strictly increasing"},
		{"bad irrbb", with("--in", positionsFile, "--vol", "1", "--irrbb-short", "-1"), 2, "--irrbb-short"},
		{"all rows fail", with("--in", allBad, "--date", "2021-03-31"), 1, "all rows failed"},
		{"strict", with("--in", mixed, "--date", "2021-03-31", "--strict"), 1, "4 of 5 positions excluded"},
	}
	for _, tt := range tests {
		code, _, errOut := run(t, tt.args...)
		if code != tt.code || !strings.Contains(errOut, tt.msg) {
			t.Errorf("%s: exit %d, stderr %q", tt.name, code, errOut)
		}
	}
	// default: warn and continue, the status says why
	code, out, errOut := run(t, with("--in", mixed, "--date", "2021-03-31", "--format", "csv")...)
	if code != 0 || !strings.Contains(errOut, "4 of 5 positions excluded") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	st := map[string]string{}
	for _, p := range parseCSV(t, out) {
		st[p["id"]] = p["status"]
	}
	if st["GOOD"] != "ok" || !strings.Contains(st["ZERO"], "invalid nominal") || !strings.Contains(st["EMPTY"], "invalid nominal") ||
		!strings.Contains(st["MATURED"], "not after the settlement") || !strings.Contains(st["BADDATE"], "invalid maturity") {
		t.Errorf("status %v", st)
	}
	// atomic output file
	outFile := filepath.Join(dir, "risk.csv")
	if _, err := os.Stat(outFile); err == nil {
		t.Fatal("exists")
	}
	mustOK(t, riskArgs("-o", outFile)...)
	if b, err := os.ReadFile(outFile); err != nil || !strings.HasPrefix(string(b), "id,issuer,rating,nominal") {
		t.Errorf("output file: %v %q", err, b)
	}
	if code, _, errOut := run(t, riskArgs("-o", positionsFile)...); code != 2 {
		t.Errorf("-o = input: exit %d %s", code, errOut)
	}
}

func TestPortfolioRiskScaling(t *testing.T) {
	// doubling every nominal doubles values and DV01, not the durations
	dir := t.TempDir()
	b, _ := os.ReadFile(positionsFile)
	var sb strings.Builder
	for i, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		f := strings.Split(l, ",")
		if i > 0 {
			n, _ := strconv.ParseFloat(f[6], 64)
			f[6] = strconv.FormatFloat(2*n, 'f', -1, 64)
		}
		sb.WriteString(strings.Join(f, ",") + "\n")
	}
	p := filepath.Join(dir, "x2.csv")
	os.WriteFile(p, []byte(sb.String()), 0o644)
	a := parseCSV(t, mustOK(t, riskArgs("--format", "csv", "--section", "summary")...))[0]
	c := parseCSV(t, mustOK(t, "portfolio", "risk", "--curve", curveFile, "--in", p, "--vol", "1.0", "--format", "csv", "--section", "summary"))[0]
	for _, k := range []string{"mv_dirty", "dv01"} {
		if math.Abs(f64(t, c[k])-2*f64(t, a[k])) > 1e-6*math.Abs(f64(t, c[k])) {
			t.Errorf("%s: %v vs 2*%v", k, c[k], a[k])
		}
	}
	if c["mod_duration"] != a["mod_duration"] {
		t.Errorf("duration %v vs %v", c["mod_duration"], a["mod_duration"])
	}
}

func TestPortfolioRiskHedgedBook(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "hedged.csv")
	os.WriteFile(p, []byte("maturity,coupon_pct,price_clean,id,issuer,nominal\n"+
		"2031-03-31,1.5,106.5,L,X,1000000\n2028-03-31,1.0,100.5,S,Y,-1059701.4925373135\n"), 0o644)
	args := func(extra ...string) []string {
		return append([]string{"portfolio", "risk", "--curve", curveFile, "--in", p}, extra...)
	}
	code, out, errOut := run(t, args("--format", "csv", "--section", "summary")...)
	if code != 0 || !strings.Contains(errOut, "net value ~0; use dv01 / key_dv01") {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	s := parseCSV(t, out)[0]
	if s["mod_duration"] != "" || s["convexity"] != "" || f64(t, s["mv_dirty"]) != 0 || f64(t, s["gross_mv"]) < 2e6 {
		t.Errorf("summary %v", s)
	}
	g := parseCSV(t, mustOK(t, args("--format", "csv", "--section", "summary", "--group-by", "issuer")...))
	for _, r := range g {
		if r["weight_pct"] != "" || r["weight_gross_pct"] == "" {
			t.Errorf("group %v", r)
		}
	}
	sc := parseCSV(t, mustOK(t, args("--format", "csv", "--section", "scenarios")...))
	if sc[0]["pv_change_pct"] != "" || sc[0]["pv_change"] == "" || sc[0]["approx_change"] == "" {
		t.Errorf("scenario %v", sc[0])
	}
	if !strings.Contains(mustOK(t, args("--format", "json", "--section", "krd")...), `"krd": null`) {
		t.Error("json krd not null")
	}
	if !strings.Contains(mustOK(t, args("--format", "text", "--section", "krd")...), "n/a") {
		t.Error("text krd not n/a")
	}
}

func TestPortfolioRiskStrictAndWarnings(t *testing.T) {
	dir := t.TempDir()
	write := func(name, s string) string {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte(s), 0o644)
		return p
	}
	hdr := "maturity,coupon_pct,price_clean,id,nominal\n"
	noPrice := write("noprice.csv", hdr+"2026-03-31,1,100,A,1000\n2027-03-31,1,,M,1000\n2026-03-31,1,100,A,1000\n")
	bad := write("bad.csv", hdr+"2026-03-31,1,100,A,1000\n2026-03-31,1,100,B,x\n")
	outFile := write("out.csv", "keep\n")
	base := []string{"portfolio", "risk", "--curve", curveFile, "--format", "csv", "-o", outFile}
	// model-valued and duplicate rows: warnings only
	code, _, errOut := run(t, append(base, "--in", noPrice)...)
	if code != 0 || !strings.Contains(errOut, "1 positions valued on the curve, not from a market price (M)") || !strings.Contains(errOut, "duplicate ids: A") {
		t.Errorf("exit %d: %s", code, errOut)
	}
	// strict: failure, output untouched
	for _, in := range []string{noPrice, bad} {
		write("out.csv", "keep\n")
		code, stdout, errOut := run(t, append(base, "--in", in, "--strict")...)
		b, _ := os.ReadFile(outFile)
		if code != 1 || string(b) != "keep\n" || stdout != "" || errOut == "" {
			t.Errorf("%s: exit %d, file %q, stderr %q", in, code, b, errOut)
		}
	}
	code, stdout, _ := run(t, "portfolio", "risk", "--curve", curveFile, "--in", bad, "--strict", "--format", "csv")
	if code != 1 || stdout != "" {
		t.Errorf("strict stdout %q", stdout)
	}
	// all rows failed names the reason
	code, _, errOut = run(t, "portfolio", "risk", "--curve", curveFile, "--in", write("all.csv", hdr+"2020-03-31,1,100,Z,10\n"), "--date", "2021-03-31")
	if code != 1 || !strings.Contains(errOut, "Z: maturity") {
		t.Errorf("exit %d: %s", code, errOut)
	}
	// stdin only once
	for _, args := range [][]string{{"portfolio", "risk", "--curve", "-", "--in", "-"}, {"bond", "price", "--curve", "-", "--in", "-"}} {
		if code, _, errOut := run(t, args...); code != 2 || !strings.Contains(errOut, "stdin can be used only once") {
			t.Errorf("%v: exit %d: %s", args, code, errOut)
		}
	}
	// price above the call price: hint
	code, _, errOut = run(t, "bond", "price", "--curve", curveFile, "--coupon", "2", "--maturity", "2031-03-31", "--price", "1e9", "--call-from", "2026-03-31@100", "--vol", "1")
	if code != 1 || !strings.Contains(errOut, "outside the range reachable by the call/put terms") {
		t.Errorf("oas hint: exit %d: %s", code, errOut)
	}
}
