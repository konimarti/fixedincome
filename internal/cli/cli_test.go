package cli

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"flag"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/konimarti/fixedincome/pkg/cashflow"
	"github.com/konimarti/fixedincome/pkg/instrument/bond"
	"github.com/konimarti/fixedincome/pkg/instrument/option"
	"github.com/konimarti/fixedincome/pkg/maturity"
	"github.com/konimarti/fixedincome/pkg/term"
)

var update = flag.Bool("update", false, "update the golden files")

const (
	curveFile = "../../data/curve_snb_2021-03-31.json"
	bondsFile = "../../data/bonds_2021-11-26.csv"
)

// run executes fic with args and returns exit code, stdout and stderr.
func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	return runIn(t, "", args...)
}

func runIn(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := RunIO(args, strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

// golden compares got with testdata/<name>.golden (or updates it).
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update)", err)
	}
	if string(want) != got {
		t.Errorf("%s differs from the golden file:\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func mustOK(t *testing.T, args ...string) string {
	t.Helper()
	code, out, errOut := run(t, args...)
	if code != 0 {
		t.Fatalf("fic %v: exit %d: %s", args, code, errOut)
	}
	return out
}

func TestGolden(t *testing.T) {
	c := []string{"--curve", curveFile}
	cases := []struct {
		name string
		args []string
	}{
		{"curve_show_text", []string{"curve", "show", "--t", "1y,2y,5y,10y", "--format", "text"}},
		{"curve_show_csv", []string{"curve", "show", "--t", "6m,1y,30y"}},
		{"curve_show_json", []string{"curve", "show", "--grid", "1y:3y:1y", "--format", "json"}},
		{"curve_show_spread", []string{"curve", "show", "--t", "1y,5y", "--spread", "50"}},
		{"curve_forward_csv", []string{"curve", "forward", "--tenor", "1y", "--grid", "0:2y:1y"}},
		{"curve_template_nss", []string{"curve", "template"}},
		{"curve_template_spline", []string{"curve", "template", "--model", "spline"}},
		{"curve_template_flat", []string{"curve", "template", "--model", "flat"}},
		{"bond_price_model_text", []string{"bond", "price", "--maturity", "2026-05-25", "--coupon", "1.25", "--format", "text"}},
		{"bond_price_quote_text", []string{"bond", "price", "--maturity", "2026-05-25", "--coupon", "1.25", "--price", "109.70", "--format", "text"}},
		{"bond_price_spread_text", []string{"bond", "price", "--maturity", "2026-05-25", "--coupon", "1.25", "--freq", "2", "--price", "101", "--spread", "50", "--format", "text"}},
		{"bond_price_quote_csv", []string{"bond", "price", "--maturity", "2026-05-25", "--coupon", "1.25", "--price", "109.70", "--format", "csv"}},
		{"bond_price_quote_json", []string{"bond", "price", "--maturity", "2026-05-25", "--coupon", "1.25", "--price", "109.70", "--format", "json"}},
		{"bond_price_yield_text", []string{"bond", "price", "--maturity", "5y", "--coupon", "2", "--freq", "2", "--daycount", "ACT360", "--yield", "1.5", "--format", "text"}},
		{"swap_rate_csv", []string{"swap", "rate", "--tenors", "2y,5y,10y"}},
		{"swap_rate_text", []string{"swap", "rate", "--tenors", "1y,18m", "--freq", "4", "--format", "text"}},
		{"sim_zcb_sigma0_csv", []string{"sim", "zcb", "--sigma", "0", "--horizon", "2y", "--steps", "4", "--paths", "10", "--seed", "1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := tc.args
			if tc.args[0] != "option" && tc.name != "curve_template_nss" && !strings.HasPrefix(tc.name, "curve_template") {
				args = append(append([]string{}, tc.args...), c...)
				if tc.args[0] == "bond" {
					args = append(args, "--date", "2021-03-31")
				}
			}
			code, out, errOut := run(t, args...)
			if code != 0 {
				t.Fatalf("exit %d: %s", code, errOut)
			}
			golden(t, tc.name, out)
		})
	}
	t.Run("option", func(t *testing.T) {
		golden(t, "option_price_text", mustOK(t, "option", "price", "--type", "call", "--spot", "110", "--strike", "100",
			"--expiry", "2y", "--vol", "30", "--rate", "2", "--format", "text"))
		golden(t, "option_price_json", mustOK(t, "option", "price", "--type", "put", "--spot", "100", "--strike", "105",
			"--expiry", "1y", "--vol", "25", "--curve", curveFile, "--div", "1.5", "--format", "json"))
		golden(t, "option_implied_csv", mustOK(t, "option", "price", "--type", "call", "--spot", "110", "--strike", "100",
			"--expiry", "2y", "--rate", "2", "--implied-from", "25.129067", "--format", "csv"))
	})
}

// batchInput has a header, CRLF line endings, quoted fields, N/A and empty
// prices, a matured bond and a bad date.
const batchInput = "maturity,coupon_pct,price_clean,id,issuer\r\n" +
	"2026-05-25,1.25,109.70,CH0224396983,\"Bank, Example\"\r\n" +
	"2030-01-15,0.5,N/A,NOPRICE,Other\r\n" +
	"2029-03-31,0,98.5,ZERO,Other\r\n" +
	"\r\n" +
	"2021-01-01,1,100,MATURED,Other\r\n" +
	"31.12.2027,2.25,\"104,5\",BADPRICE,Other\r\n" +
	"not-a-date,1,100,BADDATE,Other\r\n" +
	"2031-06-30,1.5,,EMPTY,Other\r\n" +
	"2028-06-30,1.5,101.2,DDMM,Other\r\n"

func TestBatchGolden(t *testing.T) {
	code, out, errOut := runIn(t, batchInput, "bond", "price", "--curve", curveFile, "--in", "-", "--date", "2021-03-31")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(errOut, "3 of 8 rows failed") {
		t.Errorf("stderr %q", errOut)
	}
	golden(t, "bond_batch_csv", out)
	recs, err := csv.NewReader(strings.NewReader(out)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	st := map[string]string{}
	for _, r := range recs[1:] {
		st[r[0]] = r[len(r)-1]
	}
	for id, want := range map[string]string{"CH0224396983": "ok", "ZERO": "ok", "DDMM": "ok", "NOPRICE": "ok", "EMPTY": "ok",
		"MATURED": "maturity 2021-01-01 is not after the settlement date 2021-03-31"} {
		if st[id] != want {
			t.Errorf("%s: status %q, want %q", id, st[id], want)
		}
	}
	for _, id := range []string{"BADPRICE", "BADDATE"} {
		if st[id] == "ok" || st[id] == "" {
			t.Errorf("%s: status %q", id, st[id])
		}
	}
	// text and json formats
	_, js, _ := runIn(t, batchInput, "bond", "price", "--curve", curveFile, "--in", "-", "--date", "2021-03-31", "--format", "json")
	var objs []map[string]any
	if err := json.Unmarshal([]byte(js), &objs); err != nil || len(objs) != 8 {
		t.Fatalf("json: %v, %d objects", err, len(objs))
	}
	if objs[0]["status"] != "ok" || objs[1]["yield_cc_pct"] != nil || objs[1]["price_source"] != "model" || objs[0]["price_source"] != "market" {
		t.Errorf("json rows: %v %v", objs[0], objs[1])
	}
	// all rows failing: exit 1
	code, _, errOut = runIn(t, "maturity,coupon_pct,price_clean\nnot-a-date,1,100\n", "bond", "price", "--curve", curveFile, "--in", "-")
	if code != 1 || !strings.Contains(errOut, "fic bond price: all rows failed") {
		t.Errorf("all failed: exit %d %q", code, errOut)
	}
	// missing columns in the header: file level error
	code, _, errOut = runIn(t, "maturity,coupon_pct\n2030-01-01,1\n", "bond", "price", "--curve", curveFile, "--in", "-")
	if code != 1 || !strings.Contains(errOut, "missing column") {
		t.Errorf("missing column: exit %d %q", code, errOut)
	}
}

func TestExitCodes(t *testing.T) {
	cases := []struct {
		name string
		args []string
		code int
		msg  string
	}{
		{"no args", nil, 2, ""},
		{"unknown command", []string{"foo"}, 2, "fic foo: unknown command"},
		{"unknown subcommand", []string{"curve", "foo"}, 2, "unknown subcommand"},
		{"missing subcommand", []string{"curve"}, 2, "missing subcommand"},
		{"unknown flag", []string{"bond", "price", "--nope"}, 2, "fic bond price: flag provided but not defined"},
		{"bad float flag", []string{"bond", "price", "--coupon", "x"}, 2, "invalid value"},
		{"positional", []string{"curve", "show", "extra"}, 2, "unexpected argument"},
		{"missing maturity", []string{"bond", "price", "--curve", curveFile}, 2, "--maturity and --coupon are required"},
		{"bad daycount", []string{"bond", "price", "--curve", curveFile, "--maturity", "2030-01-01", "--coupon", "1", "--daycount", "FOO"}, 2, "30E360, EUROBOND, BONDBASIS, ACT360, ACTACT"},
		{"bad freq", []string{"bond", "price", "--curve", curveFile, "--maturity", "2030-01-01", "--coupon", "1", "--freq", "5"}, 2, "invalid --freq"},
		{"bad maturity", []string{"bond", "price", "--curve", curveFile, "--maturity", "soon", "--coupon", "1"}, 2, "invalid maturity"},
		{"price and yield", []string{"bond", "price", "--curve", curveFile, "--maturity", "2030-01-01", "--coupon", "1", "--price", "100", "--yield", "1"}, 2, "cannot be combined"},
		{"in and maturity", []string{"bond", "price", "--curve", curveFile, "--in", "x.csv", "--maturity", "2030-01-01"}, 2, "cannot be combined with --in"},
		{"matured", []string{"bond", "price", "--curve", curveFile, "--date", "2021-03-31", "--maturity", "2021-03-31", "--coupon", "1"}, 1, "is not after the settlement date"},
		{"matured past", []string{"bond", "price", "--curve", curveFile, "--date", "2021-03-31", "--maturity", "2020-01-01", "--coupon", "1"}, 1, "is not after the settlement date"},
		{"missing curve", []string{"bond", "price", "--curve", "/nonexistent/curve.json", "--maturity", "2030-01-01", "--coupon", "1"}, 1, "fic bond price: reading curve"},
		{"bad format", []string{"curve", "show", "--curve", curveFile, "--format", "xml"}, 2, "invalid --format"},
		{"bad grid", []string{"curve", "show", "--curve", curveFile, "--grid", "1y"}, 2, "invalid grid"},
		{"grid and t", []string{"curve", "show", "--curve", curveFile, "--grid", "1m:1y:1m", "--t", "1y"}, 2, "cannot be combined"},
		{"bad tenor", []string{"curve", "forward", "--curve", curveFile, "--tenor", "0y"}, 2, "invalid --tenor"},
		{"swap tenor", []string{"swap", "rate", "--curve", curveFile, "--tenors", "1m"}, 2, "not a multiple"},
		{"swap freq", []string{"swap", "rate", "--curve", curveFile, "--freq", "0"}, 2, "invalid --freq"},
		{"option type", []string{"option", "price", "--type", "straddle", "--spot", "1", "--strike", "1", "--expiry", "1y", "--vol", "20", "--rate", "1"}, 2, "invalid --type"},
		{"option vol", []string{"option", "price", "--type", "call", "--spot", "1", "--strike", "1", "--expiry", "1y", "--vol", "0", "--rate", "1"}, 2, "--vol"},
		{"option no vol", []string{"option", "price", "--type", "call", "--spot", "1", "--strike", "1", "--expiry", "1y", "--rate", "1"}, 2, "--vol"},
		{"option expiry", []string{"option", "price", "--type", "call", "--spot", "1", "--strike", "1", "--expiry", "never", "--vol", "20", "--rate", "1"}, 2, "invalid --expiry"},
		{"option past expiry", []string{"option", "price", "--type", "call", "--spot", "1", "--strike", "1", "--expiry", "2000-01-01", "--vol", "20", "--rate", "1", "--date", "2021-01-01"}, 2, "not after"},
		{"option rate and curve", []string{"option", "price", "--type", "call", "--spot", "1", "--strike", "1", "--expiry", "1y", "--vol", "20", "--rate", "1", "--curve", curveFile}, 2, "exactly one of"},
		{"option no rate", []string{"option", "price", "--type", "call", "--spot", "1", "--strike", "1", "--expiry", "1y", "--vol", "20"}, 2, "exactly one of"},
		{"option vol and implied", []string{"option", "price", "--type", "call", "--spot", "1", "--strike", "1", "--expiry", "1y", "--vol", "20", "--rate", "1", "--implied-from", "0.1"}, 2, "cannot be combined"},
		{"implied unreachable", []string{"option", "price", "--type", "call", "--spot", "100", "--strike", "100", "--expiry", "1y", "--rate", "1", "--implied-from", "500"}, 1, "implied volatility"},
		{"sim no sigma", []string{"sim", "zcb", "--curve", curveFile}, 2, "--sigma is required"},
		{"sim model", []string{"sim", "zcb", "--curve", curveFile, "--sigma", "1", "--model", "cir"}, 2, "invalid --model"},
		{"sim mean reversion", []string{"sim", "zcb", "--curve", curveFile, "--sigma", "1", "--mean-reversion", "0.1"}, 2, "hullwhite only"},
		{"sim paths", []string{"sim", "zcb", "--curve", curveFile, "--sigma", "1", "--paths", "1"}, 2, "invalid --paths"},
		{"sim negative sigma", []string{"sim", "zcb", "--curve", curveFile, "--sigma", "-1"}, 2, "invalid --sigma"},
		{"fit no bonds", []string{"curve", "fit"}, 2, "--bonds is required"},
		{"fit model", []string{"curve", "fit", "--bonds", bondsFile, "--model", "linear"}, 2, "invalid --model"},
		{"fit both stdout", []string{"curve", "fit", "--bonds", bondsFile, "--model", "both"}, 2, "needs -o"},
		{"fit objective", []string{"curve", "fit", "--bonds", bondsFile, "--objective", "x"}, 2, "invalid --objective"},
		{"fit lambda", []string{"curve", "fit", "--bonds", bondsFile, "--lambda", "-1"}, 2, "invalid --lambda"},
		{"fit date", []string{"curve", "fit", "--bonds", bondsFile, "--date", "2021-13-01"}, 2, "invalid --date"},
		{"fit missing file", []string{"curve", "fit", "--bonds", "/nonexistent.csv", "--date", "2021-11-26"}, 1, "fic curve fit:"},
		{"template model", []string{"curve", "template", "--model", "x"}, 2, "invalid --model"},
		{"help ok", []string{"help"}, 0, ""},
		{"help sub", []string{"help", "bond", "price"}, 0, ""},
		{"help unknown", []string{"help", "nothing"}, 2, "unknown command"},
		{"-h", []string{"bond", "price", "-h"}, 0, ""},
		{"version", []string{"version"}, 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out, errOut := run(t, tc.args...)
			if code != tc.code {
				t.Fatalf("exit code %d, want %d (stdout %q stderr %q)", code, tc.code, out, errOut)
			}
			if tc.msg != "" && !strings.Contains(errOut, tc.msg) {
				t.Errorf("stderr %q does not contain %q", errOut, tc.msg)
			}
			if tc.code != 0 {
				if out != "" {
					t.Errorf("stdout on error: %q", out)
				}
				if len(tc.args) > 0 && strings.Count(strings.TrimSpace(errOut), "\n") != 0 {
					t.Errorf("error is not one line: %q", errOut)
				}
			}
		})
	}
}

func TestHelpAndVersion(t *testing.T) {
	out := mustOK(t, "help", "bond", "price")
	for _, want := range []string{"--curve", "--maturity", "--coupon", "--daycount", "30E360, EUROBOND"} {
		if !strings.Contains(out, want) {
			t.Errorf("help lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\n  -curve") {
		t.Errorf("help uses single dashes:\n%s", out)
	}
	if v := mustOK(t, "version"); !strings.HasPrefix(v, "fic ") {
		t.Errorf("version %q", v)
	}
	if h := mustOK(t, "help"); !strings.Contains(h, "fic curve fit") || !strings.Contains(h, "fic sim zcb") {
		t.Errorf("help %q", h)
	}
}

// TestUnits checks the conversions at the command line boundary.
func TestUnits(t *testing.T) {
	curve, meta, err := term.Parse(mustRead(t, curveFile))
	if err != nil {
		t.Fatal(err)
	}
	settle := meta.AsOf
	mat := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)

	// --coupon 1.25 is 1.25%
	b, err := bond.NewStraight(maturity.Schedule{Maturity: mat, Frequency: maturity.Annual}, 0.0125)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := b.CleanPrice(settle, curve)
	got := field(t, mustOK(t, "bond", "price", "--curve", curveFile, "--maturity", "2026-05-25", "--coupon", "1.25", "--format", "csv"), "price_clean_model")
	if math.Abs(got-want) > 1e-7 {
		t.Errorf("--coupon 1.25: clean price %v, want %v", got, want)
	}

	// --spread 50 adds 0.005 to the curve
	shifted := term.WithSpread(curve, 0.005)
	want, _ = b.CleanPrice(settle, shifted)
	got = field(t, mustOK(t, "bond", "price", "--curve", curveFile, "--maturity", "2026-05-25", "--coupon", "1.25", "--spread", "50", "--format", "csv"), "price_clean_model")
	if math.Abs(got-want) > 1e-7 {
		t.Errorf("--spread 50: clean price %v, want %v", got, want)
	}
	// the z-spread of the market price is relative to the shifted curve
	cfs, _ := b.CashFlows(settle)
	accrued, _ := b.Accrued(settle)
	z, err := cashflow.ZSpread(109.70+accrued, cfs, shifted)
	if err != nil {
		t.Fatal(err)
	}
	got = field(t, mustOK(t, "bond", "price", "--curve", curveFile, "--maturity", "2026-05-25", "--coupon", "1.25", "--spread", "50", "--price", "109.70", "--format", "csv"), "zspread_bp")
	if math.Abs(got-z*1e4) > 1e-6 {
		t.Errorf("zspread_bp %v, want %v", got, z*1e4)
	}
	// yields are percent: cc and annual
	rec := mustOK(t, "bond", "price", "--curve", curveFile, "--maturity", "2026-05-25", "--coupon", "1.25", "--price", "109.70", "--format", "csv")
	ycc, _ := cashflow.YieldCC(109.70+accrued, cfs)
	yann, _ := cashflow.Yield(109.70+accrued, cfs, 1)
	if g := field(t, rec, "yield_cc_pct"); math.Abs(g-ycc*100) > 1e-7 {
		t.Errorf("yield_cc_pct %v want %v", g, ycc*100)
	}
	if g := field(t, rec, "yield_annual_pct"); math.Abs(g-yann*100) > 1e-7 {
		t.Errorf("yield_annual_pct %v want %v", g, yann*100)
	}
	if g := field(t, rec, "mod_duration"); g <= 0 {
		t.Errorf("duration %v not positive", g)
	}
	// --yield is percent: price at the yield of the quote returns the quote
	yrec := mustOK(t, "bond", "price", "--curve", curveFile, "--maturity", "2026-05-25", "--coupon", "1.25", "--yield", strconv.FormatFloat(ycc*100, 'g', 17, 64), "--format", "csv")
	if g := field(t, yrec, "price_clean"); math.Abs(g-109.70) > 1e-6 {
		t.Errorf("--yield round trip: clean price %v", g)
	}
	// the spread_bp of the file is kept and --spread adds to it
	dir := t.TempDir()
	withSpread := filepath.Join(dir, "c.json")
	data, err := term.Marshal(term.WithSpread(curve, 0.001), meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(withSpread, data, 0o644); err != nil {
		t.Fatal(err)
	}
	want, _ = b.CleanPrice(settle, term.WithSpread(curve, 0.006))
	got = field(t, mustOK(t, "bond", "price", "--curve", withSpread, "--maturity", "2026-05-25", "--coupon", "1.25", "--spread", "50", "--format", "csv"), "price_clean_model")
	if math.Abs(got-want) > 1e-7 {
		t.Errorf("file spread 10bp + --spread 50: %v want %v", got, want)
	}

	// --vol 30 is 30%, --rate 2 is 2%, --div 1 is 1%
	o := option.European{Type: option.Call, S: 110, K: 100, T: 2, Vol: 0.3, Q: 0.01}
	flat := term.Flat{Rate: 0.02}
	p, _ := o.Price(flat)
	g, _ := o.Greeks(flat)
	orec := mustOK(t, "option", "price", "--type", "call", "--spot", "110", "--strike", "100", "--expiry", "2y", "--vol", "30", "--rate", "2", "--div", "1", "--format", "csv")
	for k, w := range map[string]float64{"price": p, "delta": g.Delta, "gamma": g.Gamma, "vega_per_volpt": g.Vega / 100, "theta_per_day": g.Theta / 365, "rho_per_pct": g.Rho / 100, "phi_per_pct": g.Phi / 100, "vol_pct": 30, "rate_cc_pct": 2, "div_pct": 1} {
		if got := field(t, orec, k); math.Abs(got-w) > 1e-8*math.Max(1, math.Abs(w)) {
			t.Errorf("option %s = %v, want %v", k, got, w)
		}
	}
	// implied volatility is returned in percent
	irec := mustOK(t, "option", "price", "--type", "call", "--spot", "110", "--strike", "100", "--expiry", "2y", "--rate", "2", "--div", "1",
		"--implied-from", strconv.FormatFloat(p, 'g', 17, 64), "--format", "csv")
	if got := field(t, irec, "vol_pct"); math.Abs(got-30) > 1e-6 {
		t.Errorf("implied vol %v%%, want 30", got)
	}
	// expiry as a date relative to --date
	drec := mustOK(t, "option", "price", "--type", "call", "--spot", "110", "--strike", "100", "--expiry", "2023-01-01", "--date", "2021-01-01", "--vol", "30", "--rate", "2", "--format", "csv")
	if got := field(t, drec, "expiry_years"); math.Abs(got-730.0/365) > 1e-12 {
		t.Errorf("expiry from date: %v", got)
	}

	// curve show: percent and annual conversion
	srec := mustOK(t, "curve", "show", "--curve", curveFile, "--t", "5y", "--format", "csv")
	if got := field(t, srec, "zero_cc_pct"); math.Abs(got-curve.ZeroRate(5)*100) > 1e-7 {
		t.Errorf("zero_cc_pct %v", got)
	}
	if got := field(t, srec, "zero_annual_pct"); math.Abs(got-math.Expm1(curve.ZeroRate(5))*100) > 1e-7 {
		t.Errorf("zero_annual_pct %v", got)
	}
	// --sigma is percent: sigma 2 equals the library's 0.02
	r, err := simulate(simParams{model: "holee", curve: curve, sigma: 0.02, horizon: 1, steps: 4, paths: 200, seed: 3})
	if err != nil {
		t.Fatal(err)
	}
	code, out, errOut := run(t, "sim", "zcb", "--curve", curveFile, "--sigma", "2", "--horizon", "1y", "--steps", "4", "--paths", "200", "--seed", "3")
	if code != 0 {
		t.Fatalf("sim: %d %s", code, errOut)
	}
	recs := tableRows(t, out)
	if v, _ := strconv.ParseFloat(recs[4]["mc_discount"], 64); math.Abs(v-r.nodes[4].mcDF) > 1e-8 {
		t.Errorf("--sigma 2: mc_discount %v, want %v (library sigma 0.02)", v, r.nodes[4].mcDF)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// tableRows parses csv output into maps.
func tableRows(t *testing.T, s string) []map[string]string {
	t.Helper()
	recs, err := csv.NewReader(strings.NewReader(s)).ReadAll()
	if err != nil {
		t.Fatalf("csv: %v\n%s", err, s)
	}
	var rows []map[string]string
	for _, r := range recs[1:] {
		m := map[string]string{}
		for i, h := range recs[0] {
			m[h] = r[i]
		}
		rows = append(rows, m)
	}
	return rows
}

// field returns the named numeric field of the first row of csv output.
func field(t *testing.T, s, name string) float64 {
	t.Helper()
	rows := tableRows(t, s)
	v, err := strconv.ParseFloat(rows[0][name], 64)
	if err != nil {
		t.Fatalf("field %s of %q: %v", name, s, err)
	}
	return v
}

func TestDefaultFormats(t *testing.T) {
	// tables: csv when stdout is not a terminal; single values: text
	out := mustOK(t, "curve", "show", "--curve", curveFile, "--t", "1y")
	if !strings.HasPrefix(out, "t_years,zero_cc_pct,") {
		t.Errorf("table default: %q", out)
	}
	out = mustOK(t, "bond", "price", "--curve", curveFile, "--maturity", "2026-05-25", "--coupon", "1")
	if !strings.HasPrefix(out, "settlement: ") {
		t.Errorf("single default: %q", out)
	}
	out = mustOK(t, "option", "price", "--type", "call", "--spot", "1", "--strike", "1", "--expiry", "1y", "--vol", "20", "--rate", "1")
	if !strings.HasPrefix(out, "type: call") {
		t.Errorf("option default: %q", out)
	}
	// writing a table to a file: csv
	f := filepath.Join(t.TempDir(), "x.out")
	mustOK(t, "curve", "show", "--curve", curveFile, "--t", "1y", "-o", f)
	if b := mustRead(t, f); !strings.HasPrefix(string(b), "t_years,") {
		t.Errorf("file default: %q", b)
	}
	// both - and -- work
	mustOK(t, "curve", "show", "-curve", curveFile, "-t", "1y")
}

func TestCurveEnvAndDate(t *testing.T) {
	t.Setenv("FIC_CURVE", curveFile)
	code, out, errOut := run(t, "curve", "show", "--t", "1y")
	if code != 0 || !strings.Contains(out, "t_years") {
		t.Fatalf("FIC_CURVE: %d %s", code, errOut)
	}
	// diagnostics go to stderr, stdout stays parseable
	code, out, errOut = run(t, "bond", "price", "--maturity", "2026-05-25", "--coupon", "1", "--date", "2021-04-17", "--format", "csv")
	if code != 0 || !strings.Contains(errOut, "warning: --date 2021-04-17 differs from the asof date 2021-03-31") {
		t.Errorf("date warning: %d %q", code, errOut)
	}
	if rows := tableRows(t, out); rows[0]["settlement"] != "2021-04-17" {
		t.Errorf("settlement %v", rows[0])
	}
	// default date is the asof date of the curve, no warning
	code, out, errOut = run(t, "bond", "price", "--maturity", "2026-05-25", "--coupon", "1", "--format", "csv")
	if errOut != "" || tableRows(t, out)[0]["settlement"] != "2021-03-31" {
		t.Errorf("asof default: %q %q", errOut, out)
	}
	// without FIC_CURVE the default is ./curve.json
	t.Setenv("FIC_CURVE", "")
	code, _, errOut = run(t, "curve", "show")
	if code != 1 || !strings.Contains(errOut, "curve.json") {
		t.Errorf("default curve file: %d %q", code, errOut)
	}
}

func TestNoCwdWrites(t *testing.T) {
	wd, _ := os.Getwd()
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	abs := func(p string) string { a, _ := filepath.Abs(filepath.Join(wd, p)); return a }
	curve, bonds := abs(curveFile), abs(bondsFile)
	for _, args := range [][]string{
		{"curve", "show", "--curve", curve},
		{"curve", "forward", "--curve", curve},
		{"curve", "template"},
		{"curve", "fit", "--bonds", bonds, "--date", "2021-11-26", "--model", "both"}, // usage error
		{"curve", "fit", "--bonds", bonds, "--date", "2021-11-26"},
		{"bond", "price", "--curve", curve, "--in", bonds},
		{"swap", "rate", "--curve", curve},
		{"sim", "zcb", "--curve", curve, "--sigma", "1", "--steps", "10", "--paths", "100"},
	} {
		run(t, args...)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("files written to the working directory: %v", entries)
	}
}

func TestOutputFile(t *testing.T) {
	dir := t.TempDir()
	// the output is opened before the work: a bad path fails before reading the bonds
	code, _, errOut := run(t, "curve", "fit", "--bonds", "/nonexistent.csv", "-o", filepath.Join(dir, "no", "such", "c.json"))
	if code != 1 || !strings.Contains(errOut, "creating output file") {
		t.Errorf("bad output path: %d %q", code, errOut)
	}
	// a failing command leaves no empty output file behind
	f := filepath.Join(dir, "x.json")
	code, _, _ = run(t, "curve", "fit", "--bonds", "/nonexistent.csv", "-o", f)
	if _, err := os.Stat(f); code != 1 || err == nil {
		t.Errorf("failed fit left %s (%v)", f, err)
	}
	// bad format is detected before the file is created
	code, _, _ = run(t, "curve", "show", "--curve", curveFile, "--format", "x", "-o", f)
	if _, err := os.Stat(f); code != 2 || err == nil {
		t.Errorf("usage error created %s", f)
	}
}

func TestFitPipeline(t *testing.T) {
	dir := t.TempDir()
	curveOut := filepath.Join(dir, "curve.json")
	resid := filepath.Join(dir, "res.csv")
	code, out, errOut := run(t, "curve", "fit", "--bonds", bondsFile, "--date", "2021-11-26", "--onrate", "-0.75", "-o", curveOut, "--residuals", resid)
	if code != 0 || out != "" {
		t.Fatalf("fit: %d stdout %q stderr %q", code, out, errOut)
	}
	if !strings.Contains(errOut, "nss: 35 of 39 bonds") {
		t.Errorf("fit summary: %q", errOut)
	}
	c, meta, err := term.Parse(mustRead(t, curveOut))
	if err != nil {
		t.Fatal(err)
	}
	if meta.AsOf.Format(dateLayout) != "2021-11-26" || !strings.Contains(meta.Source, "bonds_2021-11-26.csv") {
		t.Errorf("meta %+v", meta)
	}
	// the overnight constraint: -0.75% simple ACT/360
	if want := 365 * math.Log1p(-0.0075/360); math.Abs(c.ZeroRate(0)-want) > 1e-12 {
		t.Errorf("short rate %v, want %v", c.ZeroRate(0), want)
	}
	// residuals file
	rows := tableRows(t, string(mustRead(t, resid)))
	if len(rows) != 35 {
		t.Fatalf("%d residual rows", len(rows))
	}
	prev := 0.0
	for _, r := range rows {
		ttm, _ := strconv.ParseFloat(r["ttm_years"], 64)
		if ttm < prev {
			t.Fatal("residuals not sorted by ttm")
		}
		prev = ttm
		cc, _ := strconv.ParseFloat(r["yield_cc_model_pct"], 64)
		an, _ := strconv.ParseFloat(r["yield_annual_model_pct"], 64)
		if math.Abs(an-math.Expm1(cc/100)*100) > 1e-8 {
			t.Errorf("annual yield %v vs cc %v", an, cc)
		}
	}
	// curve json -> bond price batch -> csv
	code, out, errOut = run(t, "bond", "price", "--curve", curveOut, "--in", bondsFile)
	if code != 0 {
		t.Fatalf("batch: %d %s", code, errOut)
	}
	brows := tableRows(t, out)
	if len(brows) != 39 {
		t.Fatalf("%d rows", len(brows))
	}
	for _, r := range brows {
		if r["status"] != "ok" {
			t.Errorf("%s: status %q", r["id"], r["status"])
			continue
		}
		ttm, _ := strconv.ParseFloat(r["ttm_years"], 64)
		z, _ := strconv.ParseFloat(r["zspread_bp"], 64)
		d, _ := strconv.ParseFloat(r["mod_duration"], 64)
		if d <= 0 || d > ttm+1e-9 {
			t.Errorf("%s: duration %v for ttm %v", r["id"], d, ttm)
		}
		// the fit has an RMSE of a few bp; the z-spread of a bond is a price error
		if ttm > 0.5 && math.Abs(z) > 10 {
			t.Errorf("%s: z-spread %vbp", r["id"], z)
		}
	}
	// both models: two files
	base := filepath.Join(dir, "both.json")
	if code, _, errOut := run(t, "curve", "fit", "--bonds", bondsFile, "--date", "2021-11-26", "--model", "both", "-o", base); code != 0 {
		t.Fatalf("both: %s", errOut)
	}
	for _, m := range []string{"nss", "spline"} {
		if _, _, err := term.Parse(mustRead(t, filepath.Join(dir, "both."+m+".json"))); err != nil {
			t.Errorf("%s: %v", m, err)
		}
	}
	// fit from stdin, curve to stdout
	code, out, _ = runIn(t, string(mustRead(t, bondsFile)), "curve", "fit", "--bonds", "-", "--date", "2021-11-26", "--model", "spline")
	if _, meta, err := term.Parse([]byte(out)); code != 0 || err != nil || meta.Source != "fic curve fit: stdin" {
		t.Errorf("stdin fit: %d %v %+v", code, err, meta)
	}
}

func TestFitSkipsBadRows(t *testing.T) {
	in := string(mustRead(t, bondsFile)) + "garbage,1,2,BAD\n2030-01-01,1,N/A,NOPRICE\n"
	code, out, errOut := runIn(t, in, "curve", "fit", "--bonds", "-", "--date", "2021-11-26")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if _, _, err := term.Parse([]byte(out)); err != nil {
		t.Error(err)
	}
	if !strings.Contains(errOut, "2 of 41 rows skipped") || !strings.Contains(errOut, "missing price") {
		t.Errorf("stderr: %s", errOut)
	}
	// old bond file format (coupon, price, DD.MM.YYYY) is still accepted
	old := "maturity,coupon,price,id\n31.12.2022,1,101,A\n31.12.2024,1.5,103,B\n31.12.2027,2,106,C\n31.12.2031,2,108,D\n31.12.2036,2.5,115,E\n"
	if code, _, errOut := runIn(t, old, "curve", "fit", "--bonds", "-", "--date", "2021-11-26", "--model", "spline"); code != 0 {
		t.Errorf("old format: %d %s", code, errOut)
	}
}

func TestSimStatistics(t *testing.T) {
	code, out, errOut := run(t, "sim", "zcb", "--curve", curveFile, "--sigma", "1", "--horizon", "5y", "--steps", "20", "--paths", "20000", "--seed", "7", "--antithetic")
	if code != 0 {
		t.Fatalf("%d %s", code, errOut)
	}
	rows := tableRows(t, out)
	if len(rows) != 21 || rows[20]["t_years"] != "5" {
		t.Fatalf("%d rows", len(rows))
	}
	for _, r := range rows[1:] {
		z, _ := strconv.ParseFloat(r["z_score"], 64)
		if math.Abs(z) > 5 {
			t.Errorf("t=%s: z-score %v", r["t_years"], z)
		}
	}
	// reproducible with a seed
	_, out2, _ := run(t, "sim", "zcb", "--curve", curveFile, "--sigma", "1", "--horizon", "5y", "--steps", "20", "--paths", "20000", "--seed", "7", "--antithetic")
	if out != out2 {
		t.Error("same seed, different output")
	}
	for _, m := range []string{"hullwhite", "vasicek"} {
		args := []string{"sim", "zcb", "--curve", curveFile, "--sigma", "1", "--horizon", "3y", "--steps", "6", "--paths", "2000", "--seed", "2", "--model", m}
		if m == "hullwhite" {
			args = append(args, "--mean-reversion", "0.1")
		}
		if code, _, errOut := run(t, args...); code != 0 {
			t.Errorf("%s: %d %s", m, code, errOut)
		}
	}
}

func TestSwapDualCurve(t *testing.T) {
	// with identical curves the dual curve par rate is the single curve par rate
	single := mustOK(t, "swap", "rate", "--curve", curveFile, "--tenors", "5y")
	dual := mustOK(t, "swap", "rate", "--curve", curveFile, "--fwd-curve", curveFile, "--tenors", "5y")
	if single != dual {
		t.Errorf("single %q dual %q", single, dual)
	}
	// a higher forecast curve raises the par rate
	dir := t.TempDir()
	high := filepath.Join(dir, "high.json")
	curve, meta, _ := term.Parse(mustRead(t, curveFile))
	data, _ := term.Marshal(term.WithSpread(curve, 0.01), meta)
	os.WriteFile(high, data, 0o644)
	hi := field(t, mustOK(t, "swap", "rate", "--curve", curveFile, "--fwd-curve", high, "--tenors", "5y"), "par_rate_pct")
	if lo := field(t, single, "par_rate_pct"); !(hi > lo+0.5) {
		t.Errorf("forecast +100bp: par rate %v vs %v", hi, lo)
	}
}

func TestTenorsAndGrid(t *testing.T) {
	g, err := parseGrid("1m:30y:1m")
	if err != nil || len(g) != 360 || math.Abs(g[359]-30) > 1e-12 {
		t.Errorf("grid: %v len %d", err, len(g))
	}
	d, err := parseMaturityDate("6m", time.Date(2021, 8, 31, 0, 0, 0, 0, time.UTC))
	if err != nil || d.Format(dateLayout) != "2022-02-28" {
		t.Errorf("6m from 2021-08-31: %v %v", d, err)
	}
	for _, bad := range []string{"", "0y", "-1y", "1x", "y", "1.5.2y"} {
		if _, err := parseTenor(bad); err == nil {
			t.Errorf("tenor %q accepted", bad)
		}
	}
	if _, err := parseGrid("1y:0y:1m"); err == nil {
		t.Error("end before start accepted")
	}
	if _, err := parseGrid("1d:1000y:1d"); err == nil {
		t.Error("huge grid accepted")
	}
}

// TestSimWarnings: degenerate inputs give warnings instead of absurd dev/SE
// ratios.
func TestSimWarnings(t *testing.T) {
	for _, c := range []struct {
		args []string
		warn string
	}{
		{[]string{"--sigma", "50", "--horizon", "30y", "--steps", "30", "--paths", "1000", "--seed", "7"}, "underflow"},
		{[]string{"--sigma", "1", "--horizon", "0.000000001", "--steps", "3", "--paths", "100", "--seed", "7"}, "rounding level"},
	} {
		code, _, errOut := run(t, append([]string{"sim", "zcb", "--curve", curveFile}, c.args...)...)
		if code != 0 {
			t.Fatalf("%v: exit %d: %s", c.args, code, errOut)
		}
		if !strings.Contains(errOut, c.warn) {
			t.Errorf("%v: no %q warning in %q", c.args, c.warn, errOut)
		}
		if strings.Contains(errOut, "e+") && !strings.Contains(errOut, "1000 standard errors") {
			t.Errorf("%v: huge dev/SE without warning: %q", c.args, errOut)
		}
	}
}

// TestFitZeroOvernightRate: --onrate 0 imposes a 0% short rate.
func TestFitZeroOvernightRate(t *testing.T) {
	out := filepath.Join(t.TempDir(), "curve.json")
	if code, _, errOut := run(t, "curve", "fit", "--bonds", bondsFile, "--date", "2021-11-26", "--onrate", "0", "--model", "nss", "-o", out); code != 0 {
		t.Fatalf("%d %s", code, errOut)
	}
	c, _, err := term.Parse(mustRead(t, out))
	if err != nil {
		t.Fatal(err)
	}
	if r0 := c.ZeroRate(0); math.Abs(r0) > 1e-15 {
		t.Errorf("--onrate 0: short rate %v", r0)
	}
}
