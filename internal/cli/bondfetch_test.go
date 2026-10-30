package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/konimarti/fixedincome/pkg/source"
)

const sixSample = "../../pkg/source/testdata/six_ref.csv"

func TestBondFetchGolden(t *testing.T) {
	code, out, errOut := run(t, "bond", "fetch", "--source", "six", "--file", sixSample)
	if code != 0 {
		t.Fatal(errOut)
	}
	golden(t, "bond_fetch_six", out)
	golden(t, "bond_fetch_six_stderr", errOut)
	// the filters apply to the file: AA only, issuer substring ignoring case
	code, out, errOut = run(t, "bond", "fetch", "--source", "six", "--file", sixSample, "--rating", "AA", "--issuer", "GLARNER")
	if code != 0 || strings.Count(out, "\n") != 3 || !strings.Contains(errOut, "2 bonds written") {
		t.Errorf("%d %q %q", code, out, errOut)
	}
	code, out, _ = run(t, "bond", "fetch", "--source", "six", "--file", sixSample, "--rating", "AAA,AA")
	golden(t, "bond_fetch_six_rating", out)
}

func TestBondFetchUsageAndErrors(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"--source", "bloomberg"},
		{"--source", "six", "--issuer", "a*b"},
		{"--source", "six", "--issuer", "a|b"},
		{"--source", "six", "--issuer", "a\nb"},
		{"--source", "six", "--issuer", ""},
		{"--source", "six", "--rating", "AA*"},
		{"--source", "six", "--rating", "AA,"},
		{"--source", "six", "--currency", "chf"},
		{"--source", "six", "--currency", "CH"},
		{"--source", "six", "--timeout", "0s"},
		{"--source", "six", "--format", "json"},
		{"--source", "six", "extra"},
		{"--source", "six", "--file", sixSample, "-o", sixSample},
	} {
		code, out, errOut := run(t, append([]string{"bond", "fetch"}, args...)...)
		if code != 2 || out != "" || !strings.HasPrefix(errOut, "fic bond fetch: ") {
			t.Errorf("%v: %d %q %q", args, code, out, errOut)
		}
	}
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.csv")
	os.WriteFile(bad, []byte("MaturityDate;CouponRate;ClosingPrice;IssuerNameShort\n20300101;1;99;Bank \"X\" AG\n"), 0o644)
	html := filepath.Join(dir, "err.html")
	os.WriteFile(html, []byte("<html><body><tt>Invalid name 'X' in select parameter</tt></body></html>"), 0o644)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--file", "/nonexistent.csv"}, "no such file"},
		{[]string{"--file", bad}, "bare"},
		{[]string{"--file", html}, "Invalid name 'X'"},
		{[]string{"--file", sixSample, "--issuer", "nobody"}, "no bonds found for currency CHF, issuer \"nobody\" (11 rows read, none match)"},
		{[]string{"--file", sixSample, "--issuer", "Perpetual"}, "all 1 matching rows skipped (1 perpetual"},
	} {
		code, out, errOut := run(t, append([]string{"bond", "fetch", "--source", "six"}, tc.args...)...)
		if code != 1 || out != "" || !strings.Contains(errOut, tc.want) {
			t.Errorf("%v: %d %q %q", tc.args, code, out, errOut)
		}
	}
	if _, _, errOut := run(t, "bond", "fetch", "--source", "six", "--file", sixSample, "--timeout", "5s", "-o", filepath.Join(dir, "o.csv")); !strings.Contains(errOut, "--timeout is ignored with --file") {
		t.Errorf("%q", errOut)
	}
}

func TestBondFetchNetwork(t *testing.T) {
	data, _ := os.ReadFile(sixSample)
	var ua, rawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua, rawQuery = r.Header.Get("User-Agent"), r.URL.RawQuery
		switch r.URL.Path {
		case "/boom":
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(400)
			fmt.Fprint(w, "<html><tt>Bad request ERR-1</tt><tt>Invalid name 'X' in select parameter</tt></html>")
		case "/empty":
			fmt.Fprint(w, "ISIN;MaturityDate;CouponRate;ClosingPrice\n")
		default:
			w.Write(data)
		}
	}))
	defer srv.Close()
	base := srv.URL
	old := newSourceClient
	t.Cleanup(func() { newSourceClient = old })
	newSourceClient = func() *source.Client { return &source.Client{BaseSIX: base} }
	dir := t.TempDir()
	path := filepath.Join(dir, "b.csv")
	code, out, errOut := run(t, "bond", "fetch", "--source", "six", "--issuer", "Zürcher", "--rating", "AAA,AA", "-o", path)
	if code != 0 || out != "" {
		t.Fatalf("%d %q %q", code, out, errOut)
	}
	if !strings.HasPrefix(ua, "fic/"+Version+" (+https://github.com/konimarti/fixedincome)") ||
		!strings.Contains(rawQuery, "IssuerNameShort_~Z%FCrcher%2ACompositeRating%3DAAA%7CAA") {
		t.Errorf("request: %q %q", ua, rawQuery)
	}
	// the server filters, so the output is the whole (fixture) answer
	if b, _ := os.ReadFile(path); strings.Count(string(b), "\n") != 7 {
		t.Errorf("%s", b)
	}
	base = srv.URL + "/boom"
	code, out, errOut = run(t, "bond", "fetch", "--source", "six", "-o", path)
	if code != 1 || out != "" || !strings.Contains(errOut, "HTTP 400") || !strings.Contains(errOut, "Invalid name 'X'") {
		t.Errorf("%d %q %q", code, out, errOut)
	}
	base = srv.URL + "/empty"
	code, out, errOut = run(t, "bond", "fetch", "--source", "six", "--issuer", "Nobody", "--rating", "AAA")
	if code != 1 || out != "" || !strings.Contains(errOut, `no bonds found for currency CHF, issuer "Nobody", rating AAA`) {
		t.Errorf("%d %q %q", code, out, errOut)
	}
	// a failing run leaves the existing output untouched
	if b, _ := os.ReadFile(path); strings.Count(string(b), "\n") != 7 {
		t.Error("output changed")
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 1 {
		t.Errorf("leftover files %v", ents)
	}
}

func TestBondFetchFeedsPriceAndFit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "b.csv")
	if code, _, errOut := run(t, "bond", "fetch", "--source", "six", "--file", sixSample, "-o", path); code != 0 {
		t.Fatal(errOut)
	}
	// rating and issuer are carried through, grouped by rating
	code, out, errOut := run(t, "bond", "price", "--curve", curveFile, "--date", "2026-10-30", "--in", path, "--format", "csv")
	if code != 0 || !strings.HasPrefix(out, "id,issuer,rating,maturity,coupon_pct,freq,daycount,") ||
		!strings.Contains(out, `CH0000000008,"Bank ""X""; AG",AAA,2027-12-31,2.25,1,BONDBASIS,`) {
		t.Fatalf("%d %s %s", code, out, errOut)
	}
	code, out, errOut = run(t, "bond", "price", "--curve", curveFile, "--date", "2026-10-30", "--in", path, "--group-by", "rating", "--format", "csv")
	if code != 0 || !strings.HasPrefix(out, "rating,n,avg_zspread_bp,") || !strings.Contains(out, "AAA,2,") || !strings.Contains(out, "unknown,") {
		t.Errorf("%d %s %s", code, out, errOut)
	}
	golden(t, "bond_group_rating_stderr", errOut)
	// a bond file without rating column
	code, _, errOut = run(t, "bond", "price", "--curve", curveFile, "--in", badBondsFile, "--group-by", "rating")
	if code != 1 || !strings.Contains(errOut, "needs a rating column") {
		t.Errorf("%d %s", code, errOut)
	}
	// curve fit accepts the extra columns (the fit itself may fail on 5 bonds, but not because of the file)
	_, _, errOut = run(t, "curve", "fit", "--bonds", path, "--date", "2026-10-30", "--model", "nss")
	if strings.Contains(errOut, "rating") || strings.Contains(errOut, "CSV header") {
		t.Errorf("fit: %s", errOut)
	}
}
