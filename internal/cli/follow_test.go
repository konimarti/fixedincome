package cli

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestAtomicOutput: RC1.
func TestAtomicOutput(t *testing.T) {
	dir := t.TempDir()
	// failure keeps an existing file
	f := filepath.Join(dir, "x.json")
	if err := os.WriteFile(f, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"curve", "fit", "--bonds", "/nonexistent.csv", "-o", f},
		{"bond", "price", "--curve", curveFile, "--maturity", "2000-01-01", "--coupon", "1", "--date", "2021-03-31", "-o", f},
	} {
		if code, _, _ := run(t, args...); code != 1 {
			t.Errorf("%v: exit %d", args, code)
		}
		if b := mustRead(t, f); string(b) != "old" {
			t.Errorf("%v: existing file damaged: %q", args, b)
		}
	}
	// a usage error keeps it, too
	if code, _, _ := run(t, "curve", "show", "--curve", curveFile, "--format", "x", "-o", f); code != 2 || string(mustRead(t, f)) != "old" {
		t.Errorf("usage error damaged the file")
	}
	// success replaces it and leaves no temporary files
	if code, _, errOut := run(t, "curve", "show", "--curve", curveFile, "--t", "1y", "-o", f); code != 0 {
		t.Fatal(errOut)
	}
	if b := mustRead(t, f); !strings.HasPrefix(string(b), "t_years,") {
		t.Errorf("not replaced: %q", b)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("leftover files: %v", entries)
	}
	// -o equal to an input is refused with exit 2 and the input is untouched
	c := filepath.Join(dir, "c.json")
	if err := os.WriteFile(c, mustRead(t, curveFile), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := mustRead(t, c)
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(c, link); err != nil {
		t.Fatal(err)
	}
	bonds := filepath.Join(dir, "b.csv")
	if err := os.WriteFile(bonds, mustRead(t, bondsFile), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"curve", "show", "--curve", c, "-o", c},
		{"curve", "show", "--curve", c, "-o", filepath.Join(dir, ".", "c.json")},
		{"curve", "show", "--curve", c, "-o", link},
		{"bond", "price", "--curve", curveFile, "--in", bonds, "-o", bonds},
		{"curve", "fit", "--bonds", bonds, "--date", "2021-11-26", "-o", bonds},
		{"curve", "fit", "--bonds", bonds, "--date", "2021-11-26", "--start", c, "-o", c},
		{"curve", "fit", "--bonds", bonds, "--date", "2021-11-26", "--residuals", bonds},
		{"swap", "rate", "--curve", curveFile, "--fwd-curve", c, "-o", c},
		{"sim", "zcb", "--curve", c, "--sigma", "1", "-o", c},
	} {
		code, _, errOut := run(t, args...)
		if code != 2 || !strings.Contains(errOut, "also an input file") {
			t.Errorf("%v: exit %d %q", args, code, errOut)
		}
	}
	if string(mustRead(t, c)) != string(orig) || string(mustRead(t, bonds)) != string(mustRead(t, bondsFile)) {
		t.Error("an input file was modified")
	}
}

// TestSpreadTotal: RC3.
func TestSpreadTotal(t *testing.T) {
	args := []string{"bond", "price", "--curve", curveFile, "--maturity", "2026-05-25", "--coupon", "1.25", "--price", "109.7", "--format", "csv", "--spread", "50"}
	out := mustOK(t, args...)
	z, tot := field(t, out, "zspread_bp"), field(t, out, "spread_total_bp")
	if math.Abs(tot-(50+z)) > 1e-7 {
		t.Errorf("spread_total_bp %v, want 50 + %v", tot, z)
	}
	// without --spread the total is the z-spread
	out = mustOK(t, args[:len(args)-2]...)
	if z, tot := field(t, out, "zspread_bp"), field(t, out, "spread_total_bp"); z != tot {
		t.Errorf("%v vs %v", z, tot)
	}
	// the total is always shown in text, the z-spread only with a quote
	txt := mustOK(t, "bond", "price", "--curve", curveFile, "--maturity", "2026-05-25", "--coupon", "1.25", "--spread", "50")
	if !strings.Contains(txt, "spread_total_bp: 50\n") || strings.Contains(txt, "zspread_bp") {
		t.Errorf("text: %s", txt)
	}
}

// TestGreekUnits: RC4.
func TestGreekUnits(t *testing.T) {
	base := []string{"option", "price", "--type", "call", "--spot", "100", "--strike", "100", "--expiry", "1y", "--rate", "2", "--format", "csv"}
	a := mustOK(t, append(base, "--vol", "20")...)
	b := mustOK(t, append(base, "--vol", "21")...)
	// vega_per_volpt is the price change for one volatility point (first order)
	dp := field(t, b, "price") - field(t, a, "price")
	if v := field(t, a, "vega_per_volpt"); math.Abs(dp-v) > 0.01*v {
		t.Errorf("vega_per_volpt %v, price change %v", v, dp)
	}
	r2 := mustOK(t, "option", "price", "--type", "call", "--spot", "100", "--strike", "100", "--expiry", "1y", "--rate", "3", "--vol", "20", "--format", "csv")
	if dp, v := field(t, r2, "price")-field(t, a, "price"), field(t, a, "rho_per_pct"); math.Abs(dp-v) > 0.02*v {
		t.Errorf("rho_per_pct %v, price change %v", v, dp)
	}
	d2 := mustOK(t, "option", "price", "--type", "call", "--spot", "100", "--strike", "100", "--expiry", "364d", "--rate", "2", "--vol", "20", "--format", "csv")
	if dp, v := field(t, d2, "price")-field(t, a, "price"), field(t, a, "theta_per_day"); math.Abs(dp-v) > 0.05*math.Abs(v) {
		t.Errorf("theta_per_day %v, price change over one day %v", v, dp)
	}
	if !strings.Contains(a, "delta,gamma,vega_per_volpt,theta_per_day,rho_per_pct,phi_per_pct") {
		t.Errorf("header %q", a)
	}
}

// TestBatchModelRows: RC9 and RC10.
func TestBatchModelRows(t *testing.T) {
	in := "maturity,coupon_pct,price_clean,id,freq,daycount\n2026-05-25,1.25,,M,2,ACT360\n2026-05-25,1.25,109.7,K,,\nbad,\"-1\",100,F,x,y\n"
	code, out, errOut := runIn(t, in, "bond", "price", "--curve", curveFile, "--in", "-", "--date", "2021-03-31")
	if code != 0 {
		t.Fatalf("%d %s", code, errOut)
	}
	rows := tableRows(t, out)
	m, k, f := rows[0], rows[1], rows[2]
	if m["status"] != "ok" || m["price_source"] != "model" || k["price_source"] != "market" {
		t.Errorf("rows: %v %v", m, k)
	}
	for _, c := range []string{"yield_cc_pct", "yield_annual_pct", "zspread_bp", "spread_total_bp"} {
		if m[c] != "" || k[c] == "" {
			t.Errorf("%s: model %q market %q", c, m[c], k[c])
		}
	}
	if m["price_clean"] != m["price_clean_model"] || m["mod_duration"] == "" {
		t.Errorf("model row: %v", m)
	}
	// model duration is on the unshifted curve: equals single mode without a price
	single := mustOK(t, "bond", "price", "--curve", curveFile, "--maturity", "2026-05-25", "--coupon", "1.25", "--freq", "2", "--daycount", "ACT360", "--format", "csv", "--date", "2021-03-31")
	if d, s := m["mod_duration"], strconv.FormatFloat(field(t, single, "mod_duration"), 'g', 10, 64); d != s {
		t.Errorf("duration %s vs %s", d, s)
	}
	// a failed row echoes the raw fields
	if f["status"] == "ok" || f["maturity"] != "bad" || f["coupon_pct"] != "-1" || f["freq"] != "x" || f["daycount"] != "y" || f["id"] != "F" {
		t.Errorf("failed row: %v", f)
	}
	if !strings.Contains(errOut, "1 of 3 rows failed") || !strings.Contains(errOut, "first: F") {
		t.Errorf("stderr %q", errOut)
	}
}

// TestHelpForms: RC13.
func TestHelpForms(t *testing.T) {
	a := mustOK(t, "help", "curve fit")
	b := mustOK(t, "help", "curve", "fit")
	if a != b {
		t.Error("quoted and unquoted help differ")
	}
	if strings.Contains(a, "--o ") || !strings.Contains(a, "\n  -o ") {
		t.Errorf("-o:\n%s", a)
	}
	bp := mustOK(t, "help", "bond price")
	if strings.Contains(bp, "NaN") || strings.Contains(bp, "default 100)") && strings.Contains(bp, "--redemption") && strings.Contains(bp, "redemption value (default: the face value) (default") {
		t.Errorf("help text:\n%s", bp)
	}
	if !strings.Contains(bp, "redemption value (default: the face value)") {
		t.Errorf("redemption help:\n%s", bp)
	}
	if !strings.Contains(bp, "DD.MM.YYYY") {
		t.Error("date formats not documented")
	}
	if code, _, errOut := run(t, "swap", "rate", "--curve", curveFile, "--date", "2021-03-31"); code != 2 {
		t.Errorf("swap rate --date: %d %s", code, errOut)
	}
}

// TestTerminalDetection: RC16.
func TestTerminalDetection(t *testing.T) {
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Skip(err)
	}
	defer null.Close()
	if isTerminal(null) {
		t.Error("/dev/null is not a terminal")
	}
	if isTerminal(&bytes.Buffer{}) {
		t.Error("a buffer is not a terminal")
	}
	f, _ := os.CreateTemp(t.TempDir(), "x")
	defer f.Close()
	if isTerminal(f) {
		t.Error("a regular file is not a terminal")
	}
}

// TestCSVInjection: RC18.
func TestCSVInjection(t *testing.T) {
	in := "maturity,coupon_pct,price_clean,id,issuer\n2026-05-25,1.25,109.7,=CMD(),+x\n2027-05-25,1,100,-5,@y\n2028-05-25,1,100,T,ok\n"
	_, out, _ := runIn(t, in, "bond", "price", "--curve", curveFile, "--in", "-", "--date", "2021-03-31")
	rows := tableRows(t, out)
	if rows[0]["id"] != "'=CMD()" || rows[0]["issuer"] != "'+x" || rows[1]["id"] != "'-5" || rows[1]["issuer"] != "'@y" || rows[2]["id"] != "T" {
		t.Errorf("rows: %v", rows)
	}
	for _, s := range []string{"=1", "+1", "-1", "@a", "\tx", "\rx"} {
		if csvGuard(s) != "'"+s {
			t.Errorf("csvGuard(%q) = %q", s, csvGuard(s))
		}
	}
	if csvGuard("a=b") != "a=b" || csvGuard("") != "" {
		t.Error("csvGuard changes harmless text")
	}
	// numeric columns are untouched (negative yields stay numbers)
	if !strings.HasPrefix(rows[0]["yield_cc_pct"], "-") {
		t.Errorf("yield %q", rows[0]["yield_cc_pct"])
	}
	// json is not modified
	_, js, _ := runIn(t, in, "bond", "price", "--curve", curveFile, "--in", "-", "--date", "2021-03-31", "--format", "json")
	if !strings.Contains(js, `"id": "=CMD()"`) {
		t.Errorf("json: %s", js)
	}
}

// TestNegativeZeroAndSpread: RC20.
func TestNegativeZeroAndSpread(t *testing.T) {
	for _, f := range []string{"csv", "json", "text"} {
		out := mustOK(t, "curve", "show", "--curve", curveFile, "--t", "0", "--spread", "73.7715", "--format", f)
		if strings.Contains(out, "-0,") || strings.Contains(out, "-0\n") || strings.Contains(out, "-0 ") {
			t.Errorf("%s: negative zero in %q", f, out)
		}
	}
	if got := num(math.Copysign(0, -1)); got != "0" {
		t.Errorf("num(-0) = %q", got)
	}
	code, _, errOut := run(t, "curve", "show", "--curve", curveFile, "--spread", "1e9")
	if code != 1 || !strings.Contains(errOut, "curve discount factor underflows at t=") {
		t.Errorf("huge spread: %d %q", code, errOut)
	}
	code, _, errOut = run(t, "bond", "price", "--curve", curveFile, "--maturity", "2030-01-01", "--coupon", "1", "--spread", "-1e9")
	if code != 1 || !strings.Contains(errOut, "underflows") {
		t.Errorf("huge negative spread: %d %q", code, errOut)
	}
}

// TestRegenerateSample: the committed data/fit_residuals_2021-11-26.csv is
// the output of the documented command (see README).
func TestRegenerateSample(t *testing.T) {
	dir := t.TempDir()
	res := filepath.Join(dir, "result.csv")
	code, _, errOut := run(t, "curve", "fit", "--bonds", bondsFile, "--date", "2021-11-26", "--onrate", "-0.75", "--model", "both",
		"-o", filepath.Join(dir, "fit.json"), "--residuals", res)
	if code != 0 {
		t.Fatalf("%d %s", code, errOut)
	}
	want := mustRead(t, "../../data/fit_residuals_2021-11-26.csv")
	if got := mustRead(t, res); !bytes.Equal(got, want) {
		t.Errorf("data/fit_residuals_2021-11-26.csv is not reproduced by the documented command (%d vs %d bytes)", len(got), len(want))
	}
}
