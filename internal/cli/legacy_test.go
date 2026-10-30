package cli

import (
	"bytes"
	"encoding/csv"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/konimarti/fixedincome/pkg/fit"
	"github.com/konimarti/fixedincome/pkg/term"
)

// inTempDir runs the test in a temporary working directory and returns the
// absolute path of the sample data.
func inTempDir(t *testing.T) string {
	t.Helper()
	data, err := filepath.Abs("../../data/bonds_2021-11-26.csv")
	if err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) })
	return data
}

func readResults(t *testing.T) [][]string {
	t.Helper()
	f, err := os.Open("result.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	recs, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return recs
}

func pf(t *testing.T, s string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestLegacyTermfit(t *testing.T) {
	data := inTempDir(t)
	var out, logw bytes.Buffer
	on := -0.75
	c := termfitConfig{file: data, date: "2021-11-26", start: "term.json", objective: "yield", model: "both", onRate: &on, minTTM: fit.DefaultMinTTM, lambda: -1}
	if err := termfitRun(c, &out, &logw); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "35 of 39 bonds") {
		t.Errorf("unexpected output %s", out.String())
	}
	// ACT/360 overnight rate on the ACT/365F axis: 365*ln(1-0.0075/360)
	want := 365 * math.Log1p(-0.0075/360)
	for _, name := range []string{"nss_opt.json", "spline_opt.json"} {
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		curve, meta, err := term.Parse(b)
		if err != nil || meta.AsOf.Format(dateLayout) != "2021-11-26" || meta.Source == "" {
			t.Errorf("%s: %v %+v", name, err, meta)
		}
		tol := 1e-12
		if name == "spline_opt.json" {
			tol = 1e-4 // stiff penalty, not exact
		}
		if d := curve.ZeroRate(0) - want; math.Abs(d) > tol {
			t.Errorf("%s: short rate %v, want %v", name, curve.ZeroRate(0), want)
		}
	}
	recs := readResults(t)
	if len(recs) != 1+2*35 {
		t.Fatalf("got %d rows", len(recs))
	}
	for i, h := range residualHeader {
		if recs[0][i] != h {
			t.Fatalf("header %v", recs[0])
		}
	}
	prev := 0.0
	for _, r := range recs[1:] {
		ttm := pf(t, r[3])
		if ttm < prev {
			t.Fatalf("rows not sorted by ttm: %v", r)
		}
		prev = ttm
		// annual yield columns are the annual equivalents of the cc yields
		for _, k := range [][2]int{{6, 8}, {7, 9}} {
			cc, annual := pf(t, r[k[0]])/100, pf(t, r[k[1]])/100
			if math.Abs(annual-math.Expm1(cc)) > 1e-9 {
				t.Errorf("annual yield %v != exp(%v)-1", annual, cc)
			}
		}
		// error_bp = model - market yield; positive if the model price is
		// below the market price
		e := pf(t, r[10])
		if math.Abs(e-(pf(t, r[7])-pf(t, r[6]))*100) > 1e-6 {
			t.Errorf("error_bp %v inconsistent", e)
		}
		if dp := pf(t, r[5]) - pf(t, r[4]); math.Abs(e) > 0.01 && (e > 0) == (dp > 0) {
			t.Errorf("error_bp %v has the sign of the price error %v", e, dp)
		}
	}
	// -onrate 0 imposes a 0% short rate
	zero := 0.0
	c = termfitConfig{file: data, date: "2021-11-26", start: "term.json", objective: "yield", model: "nss", onRate: &zero, minTTM: fit.DefaultMinTTM}
	out.Reset()
	if err := termfitRun(c, &out, &logw); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile("nss_opt.json")
	curve, _, _ := term.Parse(b)
	if r0 := curve.ZeroRate(0); math.Abs(r0) > 1e-15 {
		t.Errorf("-onrate 0: short rate %v", r0)
	}
	if !strings.Contains(logw.String(), "spline_opt.json from an earlier run is not updated") {
		t.Errorf("no stale-file warning: %s", logw.String())
	}
}

func TestLegacyTermfitStartAndErrors(t *testing.T) {
	data := inTempDir(t)
	var out, logw bytes.Buffer
	// explicit -f that does not exist: logged, fit runs
	c := termfitConfig{file: data, date: "2021-11-26", start: "missing.json", startExplicit: true, objective: "yield", model: "nss", minTTM: 0.25}
	if err := termfitRun(c, &out, &logw); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logw.String(), "-f missing.json") {
		t.Errorf("missing start file not logged: %q", logw.String())
	}
	// implicit default that does not exist: silent
	logw.Reset()
	c.startExplicit = false
	if err := termfitRun(c, &out, &logw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logw.String(), "-f ") {
		t.Errorf("default start file logged: %q", logw.String())
	}
	// not an NSS curve
	os.WriteFile("flat.json", []byte(`{"model":"flat","params":{"r":1}}`), 0o644)
	logw.Reset()
	c.start = "flat.json"
	if err := termfitRun(c, &out, &logw); err != nil || !strings.Contains(logw.String(), "not an NSS curve") {
		t.Errorf("flat start: %v %q", err, logw.String())
	}
	// valid NSS start with a spread, price objective, all bonds, settlement lag
	if err := os.WriteFile("term.json", []byte(`{"model":"nss","params":{"b0":-0.4,"b1":-0.3,"b2":5,"b3":-4,"t1":4.7,"t2":3.4},"spread_bp":5}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	c = termfitConfig{file: data, date: "24.11.2021", settleLag: 2, start: "term.json", objective: "price", model: "nss", minTTM: 0, lambda: 0}
	if err := termfitRun(c, &out, &logw); err != nil {
		t.Fatal(err)
	}
	if s := out.String(); !strings.Contains(s, "starting point") || !strings.Contains(s, "39 of 39 bonds, settlement 2021-11-26") {
		t.Errorf("unexpected output %s", s)
	}
	for _, bad := range []termfitConfig{
		{file: data, date: "2021-13-01", model: "both", objective: "yield"},
		{file: data, date: "2021-11-26", model: "linear", objective: "yield"},
		{file: data, date: "2021-11-26", model: "nss", objective: "duration"},
		{file: "missing.csv", date: "2021-11-26", model: "nss", objective: "yield"},
		{file: data, date: "2021-11-26", model: "both", objective: "yield", huber: -1},
	} {
		if err := termfitRun(bad, &out, &logw); err == nil {
			t.Errorf("%+v: expected error", bad)
		}
	}
}

// legacy holee tests

func TestLegacyHolee(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := holeeRun([]string{"-n", "20", "-m", "5", "-s", "0", "-paths", "10", "-seed", "1"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&out).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	header := strings.Join(rows[0], ",")
	if header != "t,zero_rate,fwd_rate,theta,exp_short_rate,discount,mc_discount,mc_stderr,mc_zero_rate" {
		t.Errorf("header %q", header)
	}
	if len(rows) != 22 {
		t.Fatalf("%d rows, want header + 21 nodes", len(rows))
	}
	last := rows[21]
	if last[0] != "5" {
		t.Errorf("last node t=%v, want 5 (the horizon)", last[0])
	}
	for _, r := range rows[1:] {
		if len(r) != len(rows[0]) {
			t.Fatalf("row %v has %d columns", r, len(r))
		}
		z, _ := strconv.ParseFloat(r[5], 64)
		m, _ := strconv.ParseFloat(r[6], 64)
		if math.Abs(z-m) > 1e-9 {
			t.Errorf("σ=0: mc_discount %v, discount %v", m, z)
		}
	}
	if !strings.Contains(errOut.String(), "sigma=0") {
		t.Errorf("summary: %q", errOut.String())
	}
	for _, args := range [][]string{{"-n", "0"}, {"-m", "-1"}, {"-s", "-0.01"}, {"-paths", "1"}, {"-f", "missing.json"}, {"extra"}} {
		if err := holeeRun(args, &out, &errOut); err == nil {
			t.Errorf("holeeRun(%v): expected error", args)
		}
	}
}

// TestRunOutputFirst: the output file is created before the simulation, so
// an invalid -o fails immediately (2^40 paths would take forever).
func TestLegacyHoleeOutputFirst(t *testing.T) {
	var out, errOut bytes.Buffer
	err := holeeRun([]string{"-paths", "1099511627776", "-o", t.TempDir() + "/missing/dir/holee.csv"}, &out, &errOut)
	if err == nil {
		t.Fatal("expected error for an unwritable output file")
	}
}

func TestLegacyHoleeFile(t *testing.T) {
	var out, errOut bytes.Buffer
	name := t.TempDir() + "/holee.csv"
	if err := holeeRun([]string{"-n", "4", "-m", "1", "-paths", "100", "-seed", "1", "-o", name}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(name)
	if err != nil || !strings.HasPrefix(string(data), "t,zero_rate,") || out.Len() != 0 {
		t.Errorf("file %q (%v), stdout %q", data, err, out.String())
	}
}

// TestRunWarnings: degenerate inputs give warnings instead of absurd
// dev/SE ratios.
func TestLegacyHoleeWarnings(t *testing.T) {
	for _, c := range []struct {
		args []string
		warn string
	}{
		{[]string{"-s", "0.5", "-m", "30", "-n", "30", "-paths", "1000", "-seed", "7"}, "underflow"},
		{[]string{"-m", "1e-9", "-n", "3", "-paths", "100", "-seed", "7"}, "rounding level"},
	} {
		var out, errOut bytes.Buffer
		if err := holeeRun(c.args, &out, &errOut); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(errOut.String(), c.warn) {
			t.Errorf("%v: no %q warning in %q", c.args, c.warn, errOut.String())
		}
		if strings.Contains(errOut.String(), "e+") && !strings.Contains(errOut.String(), "1000 standard errors") {
			t.Errorf("%v: huge dev/SE without warning: %q", c.args, errOut.String())
		}
	}
}

// holeeRun runs the holee shim with the legacy term.json of the repository.
func holeeRun(args []string, out, errOut io.Writer) error {
	return legacyHolee(append([]string{"-f", "../../term.json"}, args...), out, errOut)
}
