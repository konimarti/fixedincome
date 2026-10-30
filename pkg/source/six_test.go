package source

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/konimarti/fixedincome/pkg/maturity"
)

func TestParseSIX(t *testing.T) {
	res, err := ParseSIX(strings.NewReader(readFile(t, "six_ref.csv")), &SIXQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Rows != 11 || res.Filtered != 1 || len(res.Bonds) != 6 || res.SkippedTotal() != 4 {
		t.Fatalf("rows %d bonds %d skipped %v", res.Rows, len(res.Bonds), res.Skipped)
	}
	for _, why := range []string{SkipPerpetual, SkipFlat, `unsupported frequency code "Y"`, `unsupported accrued interest code "A099"`} {
		if res.Skipped[why] != 1 {
			t.Errorf("skip reason %q: %v", why, res.Skipped)
		}
	}
	var ids []string
	for _, b := range res.Bonds {
		ids = append(ids, b.ISIN)
	}
	// sorted by maturity, then ISIN
	if got := strings.Join(ids, " "); got != "CH0000000008 CH0000000002 CH0000000011 CH0000000001 CH0000000006 CH0000000007" {
		t.Errorf("order %s", got)
	}
	b := res.Bonds[0] // quote edge case: quoted field with doubled quotes and a semicolon
	if b.Issuer != `Bank "X"; AG` || b.DayCount != maturity.BondBasis || b.Freq != 1 || b.CouponPct != 2.25 || !b.HasPrice {
		t.Errorf("%+v", b)
	}
	// Latin-1 decoded umlaut
	if b = res.Bonds[3]; b.Issuer != "Zürcher Kantonalbank" || b.DayCount != maturity.Eurobond || b.Rating != "AAA" {
		t.Errorf("%+v", b)
	}
	// unrated, frequency 2, ICMA
	if b = res.Bonds[4]; b.Rating != "" || b.Freq != 2 || b.DayCount != maturity.Eurobond {
		t.Errorf("%+v", b)
	}
	// zero price is no price; frequency 4 and Act/360
	if b = res.Bonds[5]; b.HasPrice || b.Freq != 4 || b.DayCount != maturity.Act360 {
		t.Errorf("%+v", b)
	}
	if b = res.Bonds[2]; b.HasPrice || b.CouponPct != 0 { // empty price
		t.Errorf("%+v", b)
	}
}

func TestParseSIXUTF8AndMinimal(t *testing.T) {
	// UTF-8 input and a file with the required columns only: no freq, no day count
	in := "\xef\xbb\xbfMaturityDate;CouponRate;ClosingPrice;IssuerNameShort\n20300101;1.5;99.5;Zürcher\n"
	res, err := ParseSIX(strings.NewReader(in), nil)
	if err != nil || len(res.Bonds) != 1 {
		t.Fatalf("%v %+v", err, res)
	}
	if b := res.Bonds[0]; b.Issuer != "Zürcher" || b.Freq != 0 || b.HasDayCount || !b.HasPrice {
		t.Errorf("%+v", b)
	}
}

func TestParseSIXErrors(t *testing.T) {
	head := "MaturityDate;CouponRate;ClosingPrice;IssuerNameShort\n"
	for name, tc := range map[string]struct{ in, want string }{
		"empty":        {"", "empty input"},
		"html":         {"<html>\n<body>\n  <tt>Bad request ERR-1</tt>\n <tt>Invalid name 'X' in select parameter</tt></body></html>", "Invalid name 'X' in select parameter"},
		"no columns":   {"ISIN;IssuerNameShort\nCH1;A\n", "lacks the columns"},
		"bare quote":   {head + "20300101;1;99;Bank \"X\" AG\n", "bare"},
		"open quote":   {head + "20300101;1;99;\"Bank\n", "quote"},
		"short row":    {head + "20300101;1;99\n", "wrong number of fields"},
		"long row":     {head + "20300101;1;99;A;B\n", "wrong number of fields"},
		"quote in num": {head + "\"2030\"0101;1;99;A\n", "quote"},
	} {
		_, err := ParseSIX(strings.NewReader(tc.in), nil)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// bad values skip the row
	res, err := ParseSIX(strings.NewReader(head+"2030-01-01;1;99;A\n20300101;N/A;99;A\n20300101;1;abc;A\n20300101;-1;99;A\n20300101;1;-5;A\n20300101;1;99;A\n"), nil)
	if err != nil || len(res.Bonds) != 1 || res.SkippedTotal() != 5 {
		t.Errorf("%v %+v", err, res)
	}
}

func TestSIXQueryValidate(t *testing.T) {
	ok := []SIXQuery{{}, {Issuer: "Glarner & Co", Ratings: []string{"AAA", "AA"}, Currency: "EUR"}, {Issuer: "Zürcher"}, {Ratings: []string{"BBB+"}}}
	for _, q := range ok {
		if err := q.Validate(); err != nil {
			t.Errorf("%+v: %v", q, err)
		}
	}
	bad := []SIXQuery{{Issuer: "a*b"}, {Issuer: "a|b"}, {Issuer: "a\nb"}, {Issuer: "a\x00"}, {Issuer: "  "}, {Issuer: "€"},
		{Currency: "chf"}, {Currency: "CHFF"}, {Ratings: []string{"AA*"}}, {Ratings: []string{"A|AA"}}, {Ratings: []string{""}}, {Ratings: []string{"a"}}}
	for _, q := range bad {
		if err := q.Validate(); err == nil {
			t.Errorf("%+v accepted", q)
		}
	}
	if w := (SIXQuery{Issuer: "Glarner", Ratings: []string{"AAA", "AA"}}).Where(); w != "PortalSegment=BO*TradingBaseCurrency=CHF*IssuerNameShort_~Glarner*CompositeRating=AAA|AA" {
		t.Error(w)
	}
}

func TestFetchSIX(t *testing.T) {
	body := readFile(t, "six_ref.csv")
	s := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv; charset=ISO-8859-1")
		fmt.Fprint(w, body)
	})
	c := &Client{BaseSIX: s.URL + "/fqs/ref.csv", UserAgent: "fic/test (+x)"}
	res, err := c.FetchSIX(context.Background(), SIXQuery{Issuer: "Glarner & Co*", Ratings: []string{"AA"}})
	if err == nil || !strings.Contains(err.Error(), "'*'") && !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("injection not refused: %v", err)
	}
	res, err = c.FetchSIX(context.Background(), SIXQuery{Issuer: "Glarner & Co/Zürcher", Ratings: []string{"AAA", "AA"}})
	if err != nil || len(res.Bonds) != 6 {
		t.Fatalf("%v %+v", err, res)
	}
	r := s.reqs[0]
	if r.URL.Path != "/fqs/ref.csv" || r.Header.Get("User-Agent") != "fic/test (+x)" {
		t.Errorf("request %s", r.URL)
	}
	q := r.URL.Query()
	want := map[string]string{
		"select":   "ISIN,IssuerNameShort,CompositeRating,MaturityDate,CouponRate,ClosingPrice,InterestFrequencyCode,AccruedInterestCalcCode,Currency",
		"where":    "PortalSegment=BO*TradingBaseCurrency=CHF*IssuerNameShort_~Glarner & Co/Z\xfcrcher*CompositeRating=AAA|AA", // Latin-1 byte
		"orderby":  "MaturityDate",
		"page":     "1",
		"pagesize": "99999",
	}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, q.Get(k), v)
		}
	}
	// exact raw query: the umlaut as %FC, spaces as %20, & as %26, | as %7C
	if !strings.Contains(r.URL.RawQuery, "where=PortalSegment%3DBO%2ATradingBaseCurrency%3DCHF%2AIssuerNameShort_~Glarner%20%26%20Co%2FZ%FCrcher%2ACompositeRating%3DAAA%7CAA") ||
		len(q) != 5 {
		t.Errorf("raw query %s", r.URL.RawQuery)
	}
}

func TestFetchSIXErrors(t *testing.T) {
	html := "<html>\n<body>\n  <tt>Bad request ERR-17</tt>\n  <br/>\n  <tt>Invalid name 'Bogus' in select parameter</tt>\n</body>\n</html>\n"
	for name, status := range map[string]int{"400": 400, "200": 200} {
		s := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(status)
			fmt.Fprint(w, html)
		})
		_, err := (&Client{BaseSIX: s.URL}).FetchSIX(context.Background(), SIXQuery{})
		if err == nil || !strings.Contains(err.Error(), "Invalid name 'Bogus' in select parameter") || strings.Contains(err.Error(), "\n") || strings.Contains(err.Error(), "select=") {
			t.Errorf("%s: %v", name, err)
		}
	}
	// header only: no rows, no error
	s := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ISIN;MaturityDate;CouponRate;ClosingPrice\n")
	})
	res, err := (&Client{BaseSIX: s.URL}).FetchSIX(context.Background(), SIXQuery{})
	if err != nil || res.Rows != 0 || len(res.Bonds) != 0 {
		t.Errorf("%v %+v", err, res)
	}
	// size limit
	s = newServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write(make([]byte, 2<<20)) })
	_, err = (&Client{BaseSIX: s.URL, MaxBytes: 1 << 20}).FetchSIX(context.Background(), SIXQuery{})
	if err == nil || !strings.Contains(err.Error(), "larger than 1048576 bytes") {
		t.Errorf("%v", err)
	}
}

func TestSIXMatch(t *testing.T) {
	b := SIXBond{Issuer: "Zürcher Kantonalbank", Rating: "AA", Currency: "CHF"}
	for q, want := range map[string]bool{"zürcher": true, "BANK": true, "basler": false} {
		if got := (SIXQuery{Issuer: q}).Match(b); got != want {
			t.Errorf("issuer %q: %v", q, got)
		}
	}
	if (SIXQuery{Ratings: []string{"AAA"}}).Match(b) || !(SIXQuery{Ratings: []string{"AAA", "AA"}}).Match(b) || (SIXQuery{Currency: "EUR"}).Match(b) {
		t.Error("rating or currency match")
	}
}

func TestLiveSIX(t *testing.T) {
	if os.Getenv("FIC_LIVE") != "1" {
		t.Skip("set FIC_LIVE=1 to test against the live endpoint")
	}
	c := &Client{UserAgent: "fixedincome-source-test (+https://github.com/konimarti/fixedincome)"}
	res, err := c.FetchSIX(context.Background(), SIXQuery{Issuer: "Glarner", Ratings: []string{"AA"}})
	if err != nil || len(res.Bonds) == 0 {
		t.Fatalf("%v %+v", err, res)
	}
	for _, b := range res.Bonds {
		if b.Rating != "AA" || b.Freq == 0 || !b.HasDayCount || !strings.Contains(strings.ToLower(b.Issuer), "glarner") {
			t.Errorf("%+v", b)
		}
	}
	t.Logf("%d bonds, skipped %v", len(res.Bonds), res.Skipped)
}
