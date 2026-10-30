package source

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// server serves the fixtures and records the requests.
type server struct {
	*httptest.Server
	mu   sync.Mutex
	reqs []*http.Request
}

func newServer(t *testing.T, h http.HandlerFunc) *server {
	s := &server{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.reqs = append(s.reqs, r)
		s.mu.Unlock()
		h(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

func fixtureHandler(t *testing.T, name string) http.HandlerFunc {
	body := readFile(t, name)
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		fmt.Fprint(w, body)
	}
}

func TestFetchSNB(t *testing.T) {
	s := newServer(t, fixtureHandler(t, "snb_live_sample.csv"))
	c := &Client{BaseSNB: s.URL + "/cube", UserAgent: "fic/test (+x)"}
	cv, err := c.FetchSNB(context.Background(), day("2025-07-30"))
	if err != nil || cv.Meta.AsOf != day("2025-07-30") {
		t.Fatalf("%+v %v", cv, err)
	}
	r := s.reqs[0]
	if r.URL.Path != "/cube" || r.URL.Query().Get("fromDate") != "2025-07-16" || r.URL.Query().Get("toDate") != "2025-07-30" {
		t.Errorf("request %s", r.URL)
	}
	if r.Header.Get("User-Agent") != "fic/test (+x)" {
		t.Errorf("user agent %q", r.Header.Get("User-Agent"))
	}
	// default user agent
	c = &Client{BaseSNB: s.URL}
	if _, err = c.FetchSNB(context.Background(), day("2025-07-30")); err != nil {
		t.Fatal(err)
	}
	if ua := s.reqs[1].Header.Get("User-Agent"); ua != DefaultUserAgent {
		t.Errorf("user agent %q", ua)
	}
	// date not in the data
	_, err = c.FetchSNB(context.Background(), day("2025-08-05"))
	if err == nil || !strings.Contains(err.Error(), "nearest earlier complete date is 2025-07-31") {
		t.Errorf("%v", err)
	}
}

func TestFetchSNBLatestWindowAndFallback(t *testing.T) {
	full := readFile(t, "snb_live_sample.csv")
	s := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("fromDate") != "" {
			// the window holds no data (stale cube)
			fmt.Fprint(w, snbHead)
			return
		}
		fmt.Fprint(w, full)
	})
	now := day("2026-10-30")
	c := &Client{BaseSNB: s.URL, Now: func() time.Time { return now }}
	cv, err := c.FetchSNB(context.Background(), time.Time{})
	if err != nil || cv.Meta.AsOf != day("2025-07-31") {
		t.Fatalf("%+v %v", cv, err)
	}
	if len(s.reqs) != 2 || s.reqs[0].URL.Query().Get("fromDate") != "2026-08-31" || s.reqs[1].URL.RawQuery != "" {
		t.Errorf("requests: %v", s.reqs)
	}
	// fresh data: one request only
	s2 := newServer(t, fixtureHandler(t, "snb_live_sample.csv"))
	c = &Client{BaseSNB: s2.URL, Now: func() time.Time { return day("2025-08-05") }}
	if _, err = c.FetchSNB(context.Background(), time.Time{}); err != nil || len(s2.reqs) != 1 {
		t.Errorf("%v, %d requests", err, len(s2.reqs))
	}
}

func TestFetchECB(t *testing.T) {
	s := newServer(t, fixtureHandler(t, "ecb_aaa.csv"))
	c := &Client{BaseECB: s.URL + "/service/data/YC"}
	cv, err := c.FetchECB(context.Background(), day("2026-10-28"), ECBOptions{})
	if err != nil || cv.Meta.AsOf != day("2026-10-28") || cv.Meta.Source != "ECB AAA" {
		t.Fatalf("%+v %v", cv, err)
	}
	r := s.reqs[0]
	if r.URL.Path != "/service/data/YC/B.U2.EUR.4F.G_N_A.SV_C_YM.BETA0+BETA1+BETA2+BETA3+TAU1+TAU2" {
		t.Errorf("path %s", r.URL.Path)
	}
	q := r.URL.Query()
	if q.Get("startPeriod") != "2026-10-14" || q.Get("endPeriod") != "2026-10-28" || q.Get("format") != "csvdata" {
		t.Errorf("query %s", r.URL.RawQuery)
	}
	// latest, all issuers
	if _, err = c.FetchECB(context.Background(), time.Time{}, ECBOptions{Issuers: "ALL"}); err != nil {
		t.Fatal(err)
	}
	r = s.reqs[1]
	if !strings.Contains(r.URL.Path, ".G_N_C.") || r.URL.Query().Get("lastNObservations") != "5" || r.URL.Query().Get("startPeriod") != "" {
		t.Errorf("request %s", r.URL)
	}
	if _, err = c.FetchECB(context.Background(), time.Time{}, ECBOptions{Issuers: "bbb"}); err == nil {
		t.Error("invalid issuers accepted")
	}
}

func TestClientHTTPErrors(t *testing.T) {
	long := strings.Repeat("x", 1000)
	s := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/404":
			http.Error(w, "No results found", 404)
		case "/500":
			http.Error(w, long, 500)
		case "/big":
			fmt.Fprint(w, strings.Repeat("a", 5000))
		case "/trunc":
			w.Header().Set("Content-Length", "1000")
			fmt.Fprint(w, "Date;D0")
		case "/slow":
			time.Sleep(500 * time.Millisecond)
			fmt.Fprint(w, snbHead)
		case "/redirect":
			http.Redirect(w, r, "http://example.invalid/x", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		}
	})
	for _, tc := range []struct {
		path, want string
		c          Client
	}{
		{"/404", "HTTP 404: No results found", Client{}},
		{"/500", "HTTP 500", Client{}},
		{"/big", "larger than 1000 bytes", Client{MaxBytes: 1000}},
		{"/trunc", "unexpected EOF", Client{}},
		{"/slow", "timeout after 100ms", Client{Timeout: 100 * time.Millisecond}},
		{"/redirect", "only https", Client{}},
		{"/loop", "", Client{}},
	} {
		c := tc.c
		c.BaseSNB = s.URL + tc.path
		_, err := c.FetchSNB(context.Background(), day("2025-07-30"))
		if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), s.URL+tc.path) {
			t.Errorf("%s: %v", tc.path, err)
		}
		if tc.path == "/500" && len(err.Error()) > 600 {
			t.Errorf("error message of %d bytes dumps the body", len(err.Error()))
		}
	}
	// a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (&Client{BaseSNB: s.URL}).FetchSNB(ctx, day("2025-07-30")); err == nil {
		t.Error("cancelled context accepted")
	}
	// connection refused
	dead := newServer(t, nil)
	url := dead.URL
	dead.Close()
	if _, err := (&Client{BaseECB: url}).FetchECB(context.Background(), time.Time{}, ECBOptions{}); err == nil {
		t.Error("closed server accepted")
	}
}

func TestRedirectToHTTPSAllowedOnly(t *testing.T) {
	body := readFile(t, "snb_live_sample.csv")
	tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
	defer tls.Close()
	s := newServer(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, tls.URL, http.StatusFound) })
	c := &Client{BaseSNB: s.URL, HTTP: tls.Client()}
	if _, err := c.FetchSNB(context.Background(), day("2025-07-30")); err != nil {
		t.Errorf("redirect to https refused: %v", err)
	}
}

// TestLive downloads from the real endpoints; opt in with FIC_LIVE=1.
func TestLive(t *testing.T) {
	if os.Getenv("FIC_LIVE") != "1" {
		t.Skip("set FIC_LIVE=1 to test against the live endpoints")
	}
	c := &Client{UserAgent: "fixedincome-source-test (+https://github.com/konimarti/fixedincome)"}
	ctx := context.Background()
	snb, err := c.FetchSNB(ctx, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("SNB as of %s: %+v", snb.Meta.AsOf.Format("2006-01-02"), snb.NSS)
	for _, iss := range []string{IssuersAAA, IssuersAll} {
		ecb, err := c.FetchECB(ctx, time.Time{}, ECBOptions{Issuers: iss})
		if err != nil {
			t.Fatal(err)
		}
		if r := ecb.NSS.ZeroRate(10); !(r > -0.05 && r < 0.2) {
			t.Errorf("ECB %s 10y zero rate %v", iss, r)
		}
		t.Logf("%s as of %s: %+v", ecb.Meta.Source, ecb.Meta.AsOf.Format("2006-01-02"), ecb.NSS)
	}
}

func TestClientBodyCaps(t *testing.T) {
	var gz bytes.Buffer
	w := gzip.NewWriter(&gz)
	zero := make([]byte, 1<<20)
	for i := 0; i < 30; i++ {
		w.Write(zero)
	}
	w.Close()
	s := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bomb":
			w.Header().Set("Content-Encoding", "gzip")
			w.Write(gz.Bytes())
		default:
			w.Write(make([]byte, 2<<20))
		}
	})
	for _, p := range []string{"/big", "/bomb"} {
		c := &Client{BaseSNB: s.URL + p, MaxBytes: 1 << 20}
		_, err := c.FetchSNB(context.Background(), day("2025-07-30"))
		if err == nil || !strings.Contains(err.Error(), "larger than 1048576 bytes") {
			t.Errorf("%s: %v", p, err)
		}
	}
}

func TestClientMessages(t *testing.T) {
	// SNB: no data in the window; the query is not part of the message
	s := newServer(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, snbHead) })
	_, err := (&Client{BaseSNB: s.URL + "/cube"}).FetchSNB(context.Background(), day("2025-07-30"))
	if err == nil || !strings.Contains(err.Error(), "no complete date in the 14 days up to 2025-07-30 (downloaded window)") ||
		!strings.Contains(err.Error(), "--date latest") || strings.Contains(err.Error(), "fromDate") {
		t.Errorf("%v", err)
	}
	// ECB: empty body and 404
	for name, h := range map[string]http.HandlerFunc{
		"empty": func(w http.ResponseWriter, r *http.Request) {},
		"404":   func(w http.ResponseWriter, r *http.Request) { http.Error(w, "No results found", 404) },
	} {
		s := newServer(t, h)
		_, err := (&Client{BaseECB: s.URL}).FetchECB(context.Background(), day("2026-10-31"), ECBOptions{})
		if err == nil || !strings.Contains(err.Error(), "ECB returned no data for 2026-10-17 to 2026-10-31") || strings.Contains(err.Error(), "startPeriod") {
			t.Errorf("%s: %v", name, err)
		}
	}
}
