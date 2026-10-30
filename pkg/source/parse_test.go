package source

import (
	"encoding/csv"
	"errors"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/konimarti/fixedincome/pkg/term"
)

func day(s string) time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return d
}

func readFile(t testing.TB, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const snbHead = "\"CubeId\";\"rendopar\"\n\"PublishingDate\";\"2025-09-01 14:30\"\n\n\"Date\";\"d0\";\"Value\"\n"

// snbDay returns the six rows of a date.
func snbDay(d string, v ...string) string {
	var sb strings.Builder
	for i, n := range paramNames {
		if i < len(v) && v[i] != "" {
			sb.WriteString("\"" + d + "\";\"" + n + "\";\"" + v[i] + "\"\n")
		}
	}
	return sb.String()
}

func TestParseSNBLive(t *testing.T) {
	c, err := ParseSNB(strings.NewReader(readFile(t, "snb_live_sample.csv")), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	// published in percent, the library has decimals
	want := term.NSS{B0: 0.000347105, B1: -0.000981406, B2: -0.0187828, B3: 0.0314724, Tau1: 3.51767, Tau2: 8.23272}
	if c.NSS != want {
		t.Errorf("NSS = %+v, want %+v", c.NSS, want)
	}
	m := c.Meta
	if m.AsOf != day("2025-07-31") || m.Currency != "CHF" || m.Source != "SNB rendopar" {
		t.Errorf("meta = %+v", m)
	}
	c, err = ParseSNB(strings.NewReader(readFile(t, "snb_live_sample.csv")), day("2025-07-29"))
	if err != nil || c.NSS.Tau1 != 2.94402 {
		t.Errorf("2025-07-29: %+v, %v", c, err)
	}
}

func TestParseSNBFormats(t *testing.T) {
	full := snbDay("2021-03-31", "-0.266372", "-0.471343", "5.68789", "-5.12324", "5.74881", "4.14426")
	cases := map[string]string{
		"lf":               snbHead + full,
		"crlf":             strings.ReplaceAll(snbHead+full, "\n", "\r\n"),
		"bom":              "\xef\xbb\xbf" + snbHead + full,
		"upper":            strings.ToUpper(snbHead + full),
		"noquotes":         strings.ReplaceAll(snbHead+full, "\"", ""),
		"long name":        strings.NewReplacer("\"b0\"", "\"BETA0\"", "\"t1\"", "\"Tau1\"").Replace(snbHead + full),
		"reordered":        "\"Value\";\"Date\";\"x\";\"D0\"\n" + reorder(full),
		"no blank":         strings.Replace(snbHead+full, "\n\n", "\n", 1),
		"no final newline": strings.TrimSuffix(snbHead+full, "\n"),
		"junk rows":        snbHead + "\"2021-03-31\";\"zz\";\"abc\"\n" + full + "\n\n",
	}
	for name, in := range cases {
		c, err := ParseSNB(strings.NewReader(in), time.Time{})
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if math.Abs(c.NSS.B0-(-0.00266372)) > 1e-15 || c.NSS.Tau2 != 4.14426 || c.Meta.AsOf != day("2021-03-31") {
			t.Errorf("%s: %+v", name, c)
		}
	}
}

// reorder converts rows Date;d0;Value to Value;Date;x;D0.
func reorder(s string) string {
	var sb strings.Builder
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		f := strings.Split(l, ";")
		sb.WriteString(f[2] + ";" + f[0] + ";\"-\";" + f[1] + "\n")
	}
	return sb.String()
}

func TestParseSNBSelection(t *testing.T) {
	a := snbDay("2021-03-29", "1", "2", "3", "4", "5", "6")
	b := snbDay("2021-03-30", "1.5", "2", "3", "4", "5", "6")
	partial := snbDay("2021-03-31", "9", "9", "9", "9", "9") // t2 missing
	empty := "\"2021-04-01\";\"t2\";\"\"\n"                  // empty value: skipped
	in := snbHead + a + b + partial + empty

	c, err := ParseSNB(strings.NewReader(in), time.Time{})
	if err != nil || c.Meta.AsOf != day("2021-03-30") || c.NSS.B0 != 0.015 {
		t.Errorf("latest complete: %+v %v", c, err)
	}
	if _, err = ParseSNB(strings.NewReader(in), day("2021-03-29")); err != nil {
		t.Error(err)
	}
	_, err = ParseSNB(strings.NewReader(in), day("2021-03-31"))
	if err == nil || !strings.Contains(err.Error(), "missing t2") || !strings.Contains(err.Error(), "nearest earlier complete date is 2021-03-30") {
		t.Errorf("incomplete date: %v", err)
	}
	_, err = ParseSNB(strings.NewReader(in), day("2021-04-05"))
	if err == nil || !strings.Contains(err.Error(), "no data") || !strings.Contains(err.Error(), "2021-03-30") {
		t.Errorf("missing date: %v", err)
	}
	_, err = ParseSNB(strings.NewReader(in), day("2021-03-01"))
	if err == nil || !strings.Contains(err.Error(), "no earlier complete date") {
		t.Errorf("no earlier: %v", err)
	}
	if _, err = ParseSNB(strings.NewReader(snbHead+partial), time.Time{}); err == nil {
		t.Error("no complete date accepted")
	}
}

func TestParseSNBErrors(t *testing.T) {
	full := snbDay("2021-03-31", "1", "2", "3", "4", "5", "6")
	cases := []struct{ name, in, want string }{
		{"empty", "", "empty input"},
		{"blank only", "\n\r\n  \n", "empty input"},
		{"no header", "\"CubeId\";\"x\"\n\"a\";\"b\"\n", "no header row"},
		{"wrong columns", "\"Date\";\"Code\";\"Value\"\n\"2021-03-31\";\"b0\";\"1\"\n", "no header row"},
		{"non numeric", snbHead + full + "\"2021-04-01\";\"b0\";\"abc\"\n", "line 11: b0 on 2021-04-01: invalid number"},
		{"nan", snbHead + "\"2021-04-01\";\"b0\";\"NaN\"\n", "invalid number"},
		{"inf", snbHead + "\"2021-04-01\";\"b0\";\"+Inf\"\n", "invalid number"},
		{"overflow", snbHead + "\"2021-04-01\";\"b0\";\"1e999\"\n", "not finite"},
		{"hex float", snbHead + "\"2021-04-01\";\"b0\";\"0x1p3\"\n", "invalid number"},
		{"underscore", snbHead + "\"2021-04-01\";\"b0\";\"1_0\"\n", "invalid number"},
		{"huge beta", snbHead + snbDay("2021-03-31", "1001", "2", "3", "4", "5", "6"), "implausible"},
		{"huge tau", snbHead + snbDay("2021-03-31", "1", "2", "3", "4", "5", "1001"), "implausible"},
		{"duplicate", snbHead + full + "\"2021-03-31\";\"B2\";\"3\"\n", "line 11: duplicate value for b2 on 2021-03-31"},
		{"bad date", snbHead + "\"31.03.2021\";\"b0\";\"1\"\n", "line 5: invalid date"},
		{"short row", snbHead + "\"2021-03-31\";\"b0\"\n", "line 5: row has 2 fields"},
		{"zero tau", snbHead + snbDay("2021-03-31", "1", "2", "3", "4", "0", "6"), "decay parameters must be positive"},
		{"negative tau", snbHead + snbDay("2021-03-31", "1", "2", "3", "4", "5", "-6"), "decay parameters must be positive"},
		{"huge line", snbHead + "\"2021-03-31\";\"b0\";\"" + strings.Repeat("1", 2<<20) + "\"\n", "longer than"},
	}
	for _, tc := range cases {
		_, err := ParseSNB(strings.NewReader(tc.in), time.Time{})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %v, want %q", tc.name, err, tc.want)
		}
	}
}

func TestParseECBFixture(t *testing.T) {
	in := readFile(t, "ecb_aaa.csv")
	c, err := ParseECB(strings.NewReader(in), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	want := term.NSS{B0: 0.016800217241, B1: 0.0070324395, B2: 0.020537804995, B3: 0.062444466681, Tau1: 1.0199921835, Tau2: 13.8819323903}
	if g, w := c.NSS.Params(), want.Params(); !closeTo(g, w) {
		t.Errorf("NSS = %+v, want %+v", c.NSS, want)
	}
	if c.Meta.AsOf != day("2026-10-29") || c.Meta.Currency != "EUR" || c.Meta.Source != "ECB AAA" {
		t.Errorf("meta = %+v", c.Meta)
	}
	c, err = ParseECB(strings.NewReader(strings.ReplaceAll(in, "G_N_A", "G_N_C")), day("2026-10-28"))
	if err != nil || c.Meta.Source != "ECB all issuers" || c.Meta.AsOf != day("2026-10-28") {
		t.Errorf("%+v %v", c, err)
	}
	_, err = ParseECB(strings.NewReader(in), day("2026-10-31"))
	if err == nil || !strings.Contains(err.Error(), "nearest earlier complete date is 2026-10-29") {
		t.Errorf("missing date: %v", err)
	}
}

func TestParseECBFormats(t *testing.T) {
	hdr := "KEY,data_type_fm,TIME_PERIOD,note,OBS_VALUE\r\n"
	rows := ""
	for i, n := range []string{"BETA0", "BETA1", "BETA2", "BETA3", "TAU1", "TAU2"} {
		rows += "k," + n + ",2026-01-02,\"a, \"\"b\"\"\"," + strconv.Itoa(i+1) + "\r\n"
	}
	rows += "k,SR_1Y,2026-01-02,x,abc\r\n" // ignored data type
	for name, in := range map[string]string{"plain": hdr + rows, "bom": "\xef\xbb\xbf" + hdr + rows, "preamble": "# comment\r\n\r\n" + hdr + rows} {
		c, err := ParseECB(strings.NewReader(in), time.Time{})
		if err != nil || c.NSS.B0 != 0.01 || c.NSS.Tau2 != 6 || c.Meta.Source != "ECB" {
			t.Errorf("%s: %+v %v", name, c, err)
		}
	}
	for name, in := range map[string]string{
		"empty":      "",
		"no columns": "a,b,c\n1,2,3\n",
		"bad value":  hdr + "k,BETA0,2026-01-02,x,abc\n",
		"dup":        hdr + rows + "k,BETA0,2026-01-02,x,1\n",
		"bad date":   hdr + "k,BETA0,02.01.2026,x,1\n",
		"short":      hdr + "k,BETA0\n",
	} {
		if _, err := ParseECB(strings.NewReader(in), time.Time{}); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

// TestECBSpotRatesAgree is an independent check of the parameters and of the
// NSS formula and units: the spot rates SR_1Y, SR_5Y and SR_10Y that the ECB
// publishes (continuously compounded, percent) must be reproduced by the
// curve built from the six parameters.
func TestECBSpotRatesAgree(t *testing.T) {
	spot := map[string]map[string]float64{} // date -> data type -> value
	r := csv.NewReader(strings.NewReader(readFile(t, "ecb_aaa_sr.csv")))
	recs, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	col := map[string]int{}
	for i, n := range recs[0] {
		col[n] = i
	}
	for _, rec := range recs[1:] {
		v, err := strconv.ParseFloat(rec[col["OBS_VALUE"]], 64)
		if err != nil {
			t.Fatal(err)
		}
		d := rec[col["TIME_PERIOD"]]
		if spot[d] == nil {
			spot[d] = map[string]float64{}
		}
		spot[d][rec[col["DATA_TYPE_FM"]]] = v
	}
	if len(spot) != 3 {
		t.Fatalf("%d dates in the spot rate fixture", len(spot))
	}
	for d, rates := range spot {
		c, err := ParseECB(strings.NewReader(readFile(t, "ecb_aaa.csv")), day(d))
		if err != nil {
			t.Fatal(err)
		}
		for name, years := range map[string]float64{"SR_1Y": 1, "SR_5Y": 5, "SR_10Y": 10} {
			got := c.NSS.ZeroRate(years) * 100
			if math.Abs(got-rates[name]) > 1e-6 {
				t.Errorf("%s %s: NSS %.10f, ECB %.10f", d, name, got, rates[name])
			}
		}
	}
}

// TestCurveFileRoundTrip checks the units end to end: the curve written with
// term.Marshal (percent) and read back with term.Parse gives the same zero
// rates as the decimals of the parsed parameters.
func TestCurveFileRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name string
		c    Curve
		pct  float64 // b0 in percent as published
	}{
		{"snb", mustSNB(t), 0.0347105},
		{"ecb", mustECB(t), 1.6800217241},
	} {
		data, err := term.Marshal(tc.c.NSS, tc.c.Meta)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), `"b0": `+strconv.FormatFloat(tc.pct, 'g', -1, 64)) {
			t.Errorf("%s: file does not hold %v percent:\n%s", tc.name, tc.pct, data)
		}
		back, meta, err := term.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		if meta != tc.c.Meta {
			t.Errorf("%s: meta %+v != %+v", tc.name, meta, tc.c.Meta)
		}
		for _, y := range []float64{0.25, 1, 5, 10, 30} {
			if a, b := back.ZeroRate(y), tc.c.NSS.ZeroRate(y); math.Abs(a-b) > 1e-14 {
				t.Errorf("%s: zero rate at %gy: %v != %v", tc.name, y, a, b)
			}
		}
	}
}

func mustSNB(t *testing.T) Curve {
	c, err := ParseSNB(strings.NewReader(readFile(t, "snb_live_sample.csv")), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func mustECB(t *testing.T) Curve {
	c, err := ParseECB(strings.NewReader(readFile(t, "ecb_aaa.csv")), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// nasty inputs must give errors, never panics
var nasty = []string{
	"", "\x00", "\xef\xbb\xbf", "\"", "\"\"\"", ";;;;", ",,,,", "\"Date\";\"d0\";\"Value\"", "Date;D0;Value\n;;\n\"",
	"Date;D0;Value\n2021-01-01;b0;1e999\n", "Date;D0;Value\n2021-01-01;b0;-\n", "Date;D0;Value\n2021-13-45;b0;1\n",
	"DATA_TYPE_FM,TIME_PERIOD,OBS_VALUE\nBETA0,2021-01-01,1\n", "DATA_TYPE_FM,TIME_PERIOD,OBS_VALUE\n\"BETA0\",\"2021-01-01\n",
	"date;d0;value;date;d0;value\n1;2;3;4;5;6\n", strings.Repeat("\"", 5000), strings.Repeat("a;", 100000),
	"\r\r\r\n\r", "Date;D0;Value\n" + strings.Repeat("2021-01-01;b0;1\n", 3),
}

func TestNastyInputs(t *testing.T) {
	for _, in := range nasty {
		for _, asof := range []time.Time{{}, day("2021-01-01")} {
			ParseSNB(strings.NewReader(in), asof)
			ParseECB(strings.NewReader(in), asof)
		}
	}
}

func FuzzParseSNB(f *testing.F) {
	f.Add(readFile(f, "snb_live_sample.csv"))
	for _, s := range nasty[:12] {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		if c, err := ParseSNB(strings.NewReader(in), time.Time{}); err == nil {
			if c.NSS.Validate() != nil || c.Meta.AsOf.IsZero() {
				t.Fatalf("invalid result %+v", c)
			}
		}
	})
}

func FuzzParseECB(f *testing.F) {
	f.Add(readFile(f, "ecb_aaa.csv"))
	f.Add(readFile(f, "ecb_aaa_sr.csv"))
	for _, s := range nasty[:12] {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		if c, err := ParseECB(strings.NewReader(in), time.Time{}); err == nil {
			if c.NSS.Validate() != nil || c.Meta.AsOf.IsZero() {
				t.Fatalf("invalid result %+v", c)
			}
		}
	})
}

func closeTo(a, b []float64) bool {
	for i := range a {
		if math.Abs(a[i]-b[i]) > 1e-15 {
			return false
		}
	}
	return true
}

func TestNotes(t *testing.T) {
	// a newer incomplete date is skipped with a note
	in := snbHead + snbDay("2021-03-30", "1", "2", "3", "4", "5", "6") + snbDay("2021-03-31", "1", "2", "3", "", "5", "")
	c, err := ParseSNB(strings.NewReader(in), time.Time{})
	if err != nil || len(c.Notes) != 1 || c.Notes[0] != "skipped newer incomplete date 2021-03-31 (missing b3, t2)" {
		t.Errorf("%v %v", c.Notes, err)
	}
	// no note if the newest date is complete or a date is requested
	if c, _ = ParseSNB(strings.NewReader(in), day("2021-03-30")); len(c.Notes) != 0 {
		t.Errorf("%v", c.Notes)
	}
	// unusually large parameters: valid, with a note
	c, err = ParseSNB(strings.NewReader(snbHead+snbDay("2021-03-31", "150", "2", "3", "4", "150", "6")), time.Time{})
	if err != nil || len(c.Notes) != 2 {
		t.Errorf("%v %v", c.Notes, err)
	}
	// the date error is typed
	_, err = ParseSNB(strings.NewReader(in), day("2021-03-31"))
	var de *DateError
	if !errors.As(err, &de) || de.Earlier != "2021-03-30" {
		t.Errorf("%v", err)
	}
}

func TestECBMixedVariants(t *testing.T) {
	row := func(inst, ccy, area, typ, date, v string) string {
		return "YC.x," + inst + "," + ccy + "," + area + "," + typ + "," + date + "," + v + "\n"
	}
	hdr := "KEY,INSTRUMENT_FM,CURRENCY,REF_AREA,DATA_TYPE_FM,TIME_PERIOD,OBS_VALUE\n"
	types := []string{"BETA0", "BETA1", "BETA2", "BETA3", "TAU1", "TAU2"}
	build := func(f func(i int, typ string) string) string {
		s := hdr
		for i, typ := range types {
			s += f(i, typ)
		}
		return s
	}
	ok := build(func(i int, typ string) string { return row("G_N_A", "EUR", "U2", typ, "2026-01-02", "1") })
	if c, err := ParseECB(strings.NewReader(ok), time.Time{}); err != nil || c.Meta.Source != "ECB AAA" {
		t.Fatalf("%+v %v", c, err)
	}
	frank := build(func(i int, typ string) string {
		inst := "G_N_A"
		if i >= 3 {
			inst = "G_N_C"
		}
		return row(inst, "EUR", "U2", typ, "2026-01-02", "1")
	})
	// AAA on one date, all issuers on another (each date complete)
	twoDates := ok + build(func(i int, typ string) string { return row("G_N_C", "EUR", "U2", typ, "2026-01-05", "1") })[len(hdr):]
	usd := ok + row("G_N_A", "USD", "U2", "SR_1Y", "2026-01-02", "1")
	for name, in := range map[string]string{"frankenstein": frank, "two dates": twoDates} {
		_, err := ParseECB(strings.NewReader(in), time.Time{})
		if err == nil || !strings.Contains(err.Error(), "mixes several values of INSTRUMENT_FM") {
			t.Errorf("%s: %v", name, err)
		}
	}
	// a USD row among known-type rows
	usd = ok + row("G_N_A", "USD", "U2", "BETA0", "2026-01-05", "1")
	if _, err := ParseECB(strings.NewReader(usd), time.Time{}); err == nil || !strings.Contains(err.Error(), "CURRENCY") {
		t.Errorf("usd: %v", err)
	}
	allUSD := build(func(i int, typ string) string { return row("G_N_A", "USD", "U2", typ, "2026-01-02", "1") })
	if _, err := ParseECB(strings.NewReader(allUSD), time.Time{}); err == nil || !strings.Contains(err.Error(), "unsupported CURRENCY") {
		t.Errorf("all usd: %v", err)
	}
}

func TestParseValuePlain(t *testing.T) {
	for _, s := range []string{"1", "-1.5", "+2", ".5", "5.", "1e3", "1.5E-3"} {
		if _, err := parseValue(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	for _, s := range []string{"", "0x10", "1_000", "Inf", "+Inf", "NaN", "1e", "--1", "1,5", "1 2", "0b1", "infinity"} {
		if _, err := parseValue(s); err == nil {
			t.Errorf("%q accepted", s)
		}
	}
}
