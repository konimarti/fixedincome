package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/konimarti/fixedincome/pkg/source"
	"github.com/konimarti/fixedincome/pkg/term"
)

const (
	snbSample = "../../scripts/testdata/snb_live_sample.csv"
	ecbSample = "../../pkg/source/testdata/ecb_aaa.csv"
)

// fakeClock fixes today for the test.
func fakeClock(t *testing.T, d string) {
	t.Helper()
	old := clock
	ts, err := time.Parse(dateLayout, d)
	if err != nil {
		t.Fatal(err)
	}
	clock = func() time.Time { return ts }
	t.Cleanup(func() { clock = old })
}

func TestCurveFetchGolden(t *testing.T) {
	fakeClock(t, "2026-10-30")
	code, out, errOut := run(t, "curve", "fetch", "--source", "snb", "--file", snbSample)
	if code != 0 {
		t.Fatal(errOut)
	}
	golden(t, "curve_fetch_snb", out)
	golden(t, "curve_fetch_snb_stderr", errOut)
	code, out, errOut = run(t, "curve", "fetch", "--source", "ecb", "--file", ecbSample, "--spread", "25")
	if code != 0 {
		t.Fatal(errOut)
	}
	golden(t, "curve_fetch_ecb", out)
	golden(t, "curve_fetch_ecb_stderr", errOut)
	ecbOut := out
	// many digits: the percent values are written without conversion noise
	code, out, errOut = run(t, "curve", "fetch", "--source", "snb", "--file", "testdata/snb_digits.csv")
	if code != 0 {
		t.Fatal(errOut)
	}
	golden(t, "curve_fetch_digits", out)
	if !strings.Contains(out, `"b0": 3.14724,`) || !strings.Contains(out, `"b3": 0.115659,`) {
		t.Errorf("digits:\n%s", out)
	}
	// the output is a valid curve file
	c, meta, err := term.Parse([]byte(ecbOut))
	if err != nil || meta.Currency != "EUR" || meta.Source != "ECB AAA" {
		t.Fatalf("%v %+v", err, meta)
	}
	if _, s := term.Spread(c); s != 0.0025 {
		t.Errorf("spread %v", s)
	}
}

func TestCurveFetchDateAndAge(t *testing.T) {
	fakeClock(t, "2025-08-05")
	// 5 days old: no warning
	code, _, errOut := run(t, "curve", "fetch", "--source", "snb", "--file", snbSample)
	if code != 0 || strings.Contains(errOut, "warning") {
		t.Errorf("%d %s", code, errOut)
	}
	fakeClock(t, "2026-10-30")
	// stale: warning, but exit 0 (a user-chosen file: no statement about the source)
	code, out, errOut := run(t, "curve", "fetch", "--source", "snb", "--file", snbSample)
	want := "fic curve fetch: warning: data in " + snbSample + " is from 2025-07-31 (456 days old)\n"
	if code != 0 || !strings.HasPrefix(errOut, want) || out == "" {
		t.Errorf("%d %q", code, errOut)
	}
	// --max-age 0 disables, a large value too
	for _, a := range []string{"0", "1000"} {
		_, _, errOut = run(t, "curve", "fetch", "--source", "snb", "--file", snbSample, "--max-age", a)
		if strings.Contains(errOut, "warning") {
			t.Errorf("--max-age %s: %s", a, errOut)
		}
	}
	// an explicit date is not warned about
	code, out, errOut = run(t, "curve", "fetch", "--source", "snb", "--file", snbSample, "--date", "30.07.2025")
	if code != 0 || strings.Contains(errOut, "warning") || !strings.Contains(out, `"asof": "2025-07-30"`) {
		t.Errorf("%d %s %s", code, out, errOut)
	}
	// stdin
	data, _ := os.ReadFile(snbSample)
	code, out, _ = runIn(t, string(data), "curve", "fetch", "--source", "snb", "--file", "-", "--date", "latest")
	if code != 0 || !strings.Contains(out, `"asof": "2025-07-31"`) {
		t.Errorf("stdin: %d %s", code, out)
	}
	// missing date: runtime error naming the nearest earlier date
	code, out, errOut = run(t, "curve", "fetch", "--source", "snb", "--file", snbSample, "--date", "2025-08-05")
	if code != 1 || out != "" || !strings.Contains(errOut, "nearest earlier complete date is 2025-07-31") {
		t.Errorf("%d %q %q", code, out, errOut)
	}
}

func TestCurveFetchUsage(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"--source", "boe"},
		{"--source", "snb", "--date", "yesterday"},
		{"--source", "snb", "--ecb-issuers", "all"},
		{"--source", "ecb", "--ecb-issuers", "bbb"},
		{"--source", "snb", "--max-age", "-1"},
		{"--source", "snb", "--timeout", "0s"},
		{"--source", "snb", "--timeout", "abc"},
		{"--source", "snb", "--spread", "NaN"},
		{"--source", "snb", "--spread", "100001"},
		{"--source", "snb", "--spread", "-100001"},
		{"--source", "ecb", "--file", ecbSample, "--ecb-issuers", "all"}, // contradicts the AAA file
		{"--source", "snb", "--file", snbSample, "extra"},
	} {
		code, out, errOut := run(t, append([]string{"curve", "fetch"}, args...)...)
		if code != 2 || out != "" || !strings.HasPrefix(errOut, "fic curve fetch: ") {
			t.Errorf("%v: %d %q %q", args, code, out, errOut)
		}
	}
	// runtime errors: exit 1
	for _, args := range [][]string{
		{"--source", "snb", "--file", "/nonexistent/file.csv"},
		{"--source", "ecb", "--file", snbSample}, // wrong format
		{"--source", "snb", "--file", ecbSample},
	} {
		code, out, errOut := run(t, append([]string{"curve", "fetch"}, args...)...)
		if code != 1 || out != "" || !strings.HasPrefix(errOut, "fic curve fetch: ") {
			t.Errorf("%v: %d %q %q", args, code, out, errOut)
		}
	}
	// help and listing
	code, out, _ := run(t, "help", "curve", "fetch")
	if code != 0 || !strings.Contains(out, "--ecb-issuers") {
		t.Errorf("help: %d %s", code, out)
	}
	if _, out, _ = run(t, "help"); !strings.Contains(out, "fic curve fetch") {
		t.Error("fetch missing in fic help")
	}
}

func TestCurveFetchOutputFile(t *testing.T) {
	fakeClock(t, "2025-08-05")
	dir := t.TempDir()
	path := filepath.Join(dir, "curve.json")
	if code, out, errOut := run(t, "curve", "fetch", "--source", "snb", "--file", snbSample, "-o", path); code != 0 || out != "" {
		t.Fatalf("%d %q %s", code, out, errOut)
	}
	good, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := term.Parse(good); err != nil {
		t.Fatal(err)
	}
	// a failing run leaves the existing file untouched and no temporary files
	if code, _, _ := run(t, "curve", "fetch", "--source", "snb", "--file", ecbSample, "-o", path); code != 1 {
		t.Errorf("exit %d", code)
	}
	if now, _ := os.ReadFile(path); string(now) != string(good) {
		t.Error("existing output file changed by a failing run")
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 1 {
		t.Errorf("leftover files: %v", ents)
	}
	// -o equal to the input file is a usage error
	in := filepath.Join(dir, "in.csv")
	data, _ := os.ReadFile(snbSample)
	os.WriteFile(in, data, 0o644)
	if code, _, _ := run(t, "curve", "fetch", "--source", "snb", "--file", in, "-o", in); code != 2 {
		t.Errorf("exit %d", code)
	}
	if got, _ := os.ReadFile(in); string(got) != string(data) {
		t.Error("input file overwritten")
	}
}

func TestCurveFetchNetwork(t *testing.T) {
	fakeClock(t, "2026-10-30")
	data, _ := os.ReadFile(ecbSample)
	var ua, path, query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua, path, query = r.Header.Get("User-Agent"), r.URL.Path, r.URL.RawQuery
		if strings.Contains(r.URL.Path, "boom") {
			http.Error(w, "down", 503)
			return
		}
		fmt.Fprint(w, string(data))
	}))
	defer srv.Close()
	old := newSourceClient
	t.Cleanup(func() { newSourceClient = old })
	base := srv.URL
	newSourceClient = func() *source.Client { return &source.Client{BaseECB: base} }

	code, out, errOut := run(t, "curve", "fetch", "--source", "ecb", "--ecb-issuers", "all", "--timeout", "5s")
	if code != 0 || !strings.Contains(out, `"asof": "2026-10-29"`) {
		t.Fatalf("%d %s %s", code, out, errOut)
	}
	if !strings.HasPrefix(ua, "fic/"+Version+" (+https://github.com/konimarti/fixedincome)") || !strings.Contains(path, "G_N_C") || !strings.Contains(query, "lastNObservations") {
		t.Errorf("request: %q %q %q", ua, path, query)
	}
	base = srv.URL + "/boom"
	code, out, errOut = run(t, "curve", "fetch", "--source", "ecb")
	if code != 1 || out != "" || !strings.Contains(errOut, "HTTP 503") {
		t.Errorf("%d %q %q", code, out, errOut)
	}
}

func TestCurveFetchStrict(t *testing.T) {
	fakeClock(t, "2026-10-30")
	dir := t.TempDir()
	path := filepath.Join(dir, "c.json")
	code, out, errOut := run(t, "curve", "fetch", "--source", "snb", "--file", snbSample, "--strict", "-o", path)
	want := "fic curve fetch: data is stale: latest SNB parameters are from 2025-07-31 (456 days old, --max-age 14)\n"
	if code != 1 || out != "" || errOut != want {
		t.Errorf("%d %q %q", code, out, errOut)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Errorf("output created: %v", ents)
	}
	// stdout stays empty too
	code, out, _ = run(t, "curve", "fetch", "--source", "snb", "--file", snbSample, "--strict")
	if code != 1 || out != "" {
		t.Errorf("%d %q", code, out)
	}
	// not strict with --max-age 0, an explicit date, or fresh data
	for _, extra := range [][]string{{"--max-age", "0"}, {"--date", "2025-07-30"}, {"--max-age", "1000"}} {
		code, out, _ = run(t, append([]string{"curve", "fetch", "--source", "snb", "--file", snbSample, "--strict"}, extra...)...)
		if code != 0 || out == "" {
			t.Errorf("%v: %d", extra, code)
		}
	}
	// ECB file is current
	code, _, errOut = run(t, "curve", "fetch", "--source", "ecb", "--file", ecbSample, "--strict")
	if code != 0 || strings.Contains(errOut, "STALE") {
		t.Errorf("%d %s", code, errOut)
	}
	// in the network mode the warning blames the source
	data, _ := os.ReadFile(snbSample)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, string(data)) }))
	defer srv.Close()
	old := newSourceClient
	t.Cleanup(func() { newSourceClient = old })
	newSourceClient = func() *source.Client { return &source.Client{BaseSNB: srv.URL} }
	code, _, errOut = run(t, "curve", "fetch", "--source", "snb")
	if code != 0 || !strings.Contains(errOut, "latest available SNB parameters are from 2025-07-31; the source may no longer be updated") ||
		!strings.Contains(errOut, "[STALE: 456 days]") {
		t.Errorf("%d %s", code, errOut)
	}
}

func TestCurveFetchNotesAndExplicitIssuers(t *testing.T) {
	fakeClock(t, "2025-08-05")
	dir := t.TempDir()
	inc := filepath.Join(dir, "inc.csv")
	data, _ := os.ReadFile(snbSample)
	extra := "\"2025-08-01\";\"b0\";\"0.1\"\r\n\"2025-08-01\";\"b1\";\"0.1\"\r\n"
	os.WriteFile(inc, append(data, extra...), 0o644)
	code, out, errOut := run(t, "curve", "fetch", "--source", "snb", "--file", inc)
	if code != 0 || !strings.Contains(out, `"asof": "2025-07-31"`) ||
		!strings.Contains(errOut, "warning: skipped newer incomplete date 2025-08-01 (missing b2, b3, t1, t2)") {
		t.Errorf("%d %s", code, errOut)
	}
	// matching explicit issuers is fine, the file's variant is used without the flag
	if code, _, errOut = run(t, "curve", "fetch", "--source", "ecb", "--file", ecbSample, "--ecb-issuers", "aaa"); code != 0 {
		t.Error(errOut)
	}
	code, _, errOut = run(t, "curve", "fetch", "--source", "ecb", "--file", ecbSample, "--ecb-issuers", "all")
	if code != 2 || !strings.Contains(errOut, "contradicts") {
		t.Errorf("%d %s", code, errOut)
	}
	// a file of mixed variants
	b, _ := os.ReadFile(ecbSample)
	mixed := filepath.Join(dir, "mixed.csv")
	os.WriteFile(mixed, []byte(strings.Replace(string(b), ",G_N_A,", ",G_N_C,", 1)), 0o644)
	code, _, errOut = run(t, "curve", "fetch", "--source", "ecb", "--file", mixed)
	if code != 1 || !strings.Contains(errOut, "mixes several values of INSTRUMENT_FM") {
		t.Errorf("%d %s", code, errOut)
	}
}

func TestCurveFetchWriteFailure(t *testing.T) {
	fakeClock(t, "2025-08-05")
	if _, err := os.Stat("/dev/full"); err != nil {
		t.Skip("no /dev/full")
	}
	code, _, errOut := run(t, "curve", "fetch", "--source", "snb", "--file", snbSample, "-o", "/dev/full")
	if code != 1 || strings.Contains(errOut, "as of") || strings.Contains(errOut, "warning") {
		t.Errorf("%d %q", code, errOut)
	}
	code, _, errOut = run(t, "curve", "fetch", "--source", "snb", "--file", snbSample, "-o", "/nonexistent-dir/c.json")
	if code != 1 || strings.Contains(errOut, "as of") {
		t.Errorf("%d %q", code, errOut)
	}
}
