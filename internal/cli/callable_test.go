package cli

import (
	"strconv"
	"strings"
	"testing"
)

const callableBonds = "maturity,coupon_pct,price_clean,id,issuer,call,call_from,put\n" +
	"2031-03-31,2.0,101,CALL5Y,A,,2026-03-31@100,\n" +
	"2031-03-31,2.0,103,CALLDATES,A,2026-03-31@100;2028-03-31@100,,\n" +
	"2031-03-31,2.0,99,PUTABLE,B,,,2027-03-31@100\n" +
	"2031-03-31,2.0,,NOPRICE,B,,2026-03-31@100,\n" +
	"2028-03-31,1.0,100.5,STRAIGHT,B,,,\n" +
	"2031-03-31,2.0,101,BADCALL,B,2032-03-31@100,,\n"

func TestCallableSingle(t *testing.T) {
	base := []string{"bond", "price", "--curve", curveFile, "--date", "2021-03-31", "--maturity", "2031-03-31", "--coupon", "2"}
	args := func(extra ...string) []string { return append(append([]string{}, base...), extra...) }
	golden(t, "bond_price_callable_text", mustOK(t, args("--price", "101", "--call-from", "2026-03-31@100", "--vol", "1.0", "--format", "text")...))
	golden(t, "bond_price_callable_json", mustOK(t, args("--price", "101", "--call", "31.03.2026@100,2028-03-31@100", "--put", "2029-03-31@100", "--vol", "1.0", "--format", "json")...))
	golden(t, "bond_price_callable_model_csv", mustOK(t, args("--call-from", "2026-03-31@100", "--vol", "1.0", "--mean-reversion", "0.1", "--steps", "25", "--format", "csv")...))

	rec := func(extra ...string) map[string]float64 {
		m := map[string]float64{}
		for _, l := range strings.Split(strings.TrimSpace(mustOK(t, args(append(extra, "--format", "text")...)...)), "\n") {
			k, v, _ := strings.Cut(l, ": ")
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				m[k] = f
			}
		}
		return m
	}
	m := rec("--price", "101", "--call-from", "2026-03-31@100", "--vol", "1.0")
	if !(m["oas_bp"] < m["zspread_bp"]) || !(m["mod_duration"] < m["straight_duration"]) || !(m["option_value"] > 0) || m["dv01"] >= 0 {
		t.Errorf("callable sanity: %v", m)
	}
	if d := m["value_callable"] - 101; d > 1e-6 || d < -1e-6 {
		t.Errorf("value at OAS %v != price 101", m["value_callable"])
	}
	// vol dependence of the OAS
	lo := rec("--price", "101", "--call-from", "2026-03-31@100", "--vol", "0.3")["oas_bp"]
	hi := rec("--price", "101", "--call-from", "2026-03-31@100", "--vol", "1.5")["oas_bp"]
	if !(hi < lo) {
		t.Errorf("OAS should fall with the vol: %v %v", lo, hi)
	}
	// a straight bond is unchanged without call terms
	if _, ok := rec("--price", "101")["oas_bp"]; ok {
		t.Error("oas_bp for a straight bond")
	}
}

func TestCallableErrors(t *testing.T) {
	base := []string{"bond", "price", "--curve", curveFile, "--date", "2021-03-31", "--maturity", "2031-03-31", "--coupon", "2", "--price", "101"}
	tests := []struct {
		name  string
		extra []string
		code  int
		msg   string
	}{
		{"no vol", []string{"--call", "2026-03-31@100"}, 2, "--vol is required"},
		{"call and call-from", []string{"--call", "2026-03-31@100", "--call-from", "2026-03-31@100", "--vol", "1"}, 2, "cannot be combined"},
		{"bad call", []string{"--call", "2026-03-31", "--vol", "1"}, 2, "DATE@PRICE"},
		{"bad call price", []string{"--call", "2026-03-31@-1", "--vol", "1"}, 2, "invalid price"},
		{"bad vol", []string{"--call", "2026-03-31@100", "--vol", "0"}, 2, "invalid --vol"},
		{"bad mean reversion", []string{"--call", "2026-03-31@100", "--vol", "1", "--mean-reversion", "-1"}, 2, "--mean-reversion"},
		{"bad steps", []string{"--call", "2026-03-31@100", "--vol", "1", "--steps", "0"}, 2, "--steps"},
		{"yield", []string{"--call", "2026-03-31@100", "--vol", "1", "--yield", "1"}, 2, "--yield"},
		{"call at maturity", []string{"--call", "2031-03-31@100", "--vol", "1"}, 1, "not before the maturity"},
		{"call-from without coupon date", []string{"--call-from", "2031-01-01@100", "--vol", "1"}, 1, "no coupon date"},
	}
	for _, tt := range tests {
		args := append(append([]string{}, base...), tt.extra...)
		if tt.name == "yield" {
			// --price and --yield conflict: drop the price
			args = append(append([]string{}, base[:len(base)-2]...), tt.extra...)
		}
		code, _, errOut := run(t, args...)
		if code != tt.code || !strings.Contains(errOut, tt.msg) {
			t.Errorf("%s: exit %d, stderr %q", tt.name, code, errOut)
		}
	}
	code, _, errOut := run(t, "bond", "price", "--curve", curveFile, "--in", "../../data/bonds_callable_example.csv")
	if code != 2 || !strings.Contains(errOut, "--vol is required") {
		t.Errorf("batch without vol: %d %q", code, errOut)
	}
	code, _, errOut = run(t, "bond", "price", "--curve", curveFile, "--in", "../../data/bonds_callable_example.csv", "--call", "2026-03-31@100", "--vol", "1")
	if code != 2 || !strings.Contains(errOut, "cannot be combined with --in") {
		t.Errorf("batch with --call: %d %q", code, errOut)
	}
}

func TestCallableBatch(t *testing.T) {
	args := []string{"bond", "price", "--curve", curveFile, "--date", "2021-03-31", "--in", "-", "--vol", "1.0", "--format", "csv"}
	code, out, errOut := runIn(t, callableBonds, args...)
	if code != 0 || !strings.Contains(errOut, "1 of 6 rows failed") {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	golden(t, "bond_batch_callable_csv", out)
	code, out, _ = runIn(t, callableBonds, append(args, "--group-by", "issuer")...)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	golden(t, "bond_group_callable_csv", out)
	// files without call terms keep the old columns
	_, plain, _ := runIn(t, "maturity,coupon_pct,price_clean\n2026-05-25,1.25,109.7\n", "bond", "price", "--curve", curveFile, "--date", "2021-03-31", "--in", "-", "--format", "csv")
	if strings.Contains(plain, "oas_bp") {
		t.Errorf("straight file has callable columns: %s", plain)
	}
}
