package main

import (
	"bytes"
	"encoding/csv"
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
	data, err := filepath.Abs("bonddata.csv")
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

func TestRun(t *testing.T) {
	data := inTempDir(t)
	var out, logw bytes.Buffer
	on := -0.75
	c := config{file: data, date: "2021-11-26", start: "term.json", objective: "yield", model: "both", onRate: &on, minTTM: fit.DefaultMinTTM, lambda: -1}
	if err := run(c, &out, &logw); err != nil {
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
		if err != nil || meta.AsOf.Format(dateFmt) != "2021-11-26" || meta.Source == "" {
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
	for i, h := range resultHeader {
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
	c = config{file: data, date: "2021-11-26", start: "term.json", objective: "yield", model: "nss", onRate: &zero, minTTM: fit.DefaultMinTTM}
	out.Reset()
	if err := run(c, &out, &logw); err != nil {
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

func TestRunStartAndErrors(t *testing.T) {
	data := inTempDir(t)
	var out, logw bytes.Buffer
	// explicit -f that does not exist: logged, fit runs
	c := config{file: data, date: "2021-11-26", start: "missing.json", startExplicit: true, objective: "yield", model: "nss", minTTM: 0.25}
	if err := run(c, &out, &logw); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logw.String(), "-f missing.json") {
		t.Errorf("missing start file not logged: %q", logw.String())
	}
	// implicit default that does not exist: silent
	logw.Reset()
	c.startExplicit = false
	if err := run(c, &out, &logw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logw.String(), "-f ") {
		t.Errorf("default start file logged: %q", logw.String())
	}
	// not an NSS curve
	os.WriteFile("flat.json", []byte(`{"model":"flat","params":{"r":1}}`), 0o644)
	logw.Reset()
	c.start = "flat.json"
	if err := run(c, &out, &logw); err != nil || !strings.Contains(logw.String(), "not an NSS curve") {
		t.Errorf("flat start: %v %q", err, logw.String())
	}
	// valid NSS start with a spread, price objective, all bonds, settlement lag
	if err := os.WriteFile("term.json", []byte(`{"model":"nss","params":{"b0":-0.4,"b1":-0.3,"b2":5,"b3":-4,"t1":4.7,"t2":3.4},"spread_bp":5}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	c = config{file: data, date: "24.11.2021", settleLag: 2, start: "term.json", objective: "price", model: "nss", minTTM: 0, lambda: 0}
	if err := run(c, &out, &logw); err != nil {
		t.Fatal(err)
	}
	if s := out.String(); !strings.Contains(s, "starting point") || !strings.Contains(s, "39 of 39 bonds, settlement 2021-11-26") {
		t.Errorf("unexpected output %s", s)
	}
	for _, bad := range []config{
		{file: data, date: "2021-13-01", model: "both", objective: "yield"},
		{file: data, date: "2021-11-26", model: "linear", objective: "yield"},
		{file: data, date: "2021-11-26", model: "nss", objective: "duration"},
		{file: "missing.csv", date: "2021-11-26", model: "nss", objective: "yield"},
		{file: data, date: "2021-11-26", model: "both", objective: "yield", huber: -1},
	} {
		if err := run(bad, &out, &logw); err == nil {
			t.Errorf("%+v: expected error", bad)
		}
	}
}
