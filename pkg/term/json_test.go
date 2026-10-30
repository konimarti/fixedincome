package term_test

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/konimarti/fixedincome/pkg/term"
)

func TestParse(t *testing.T) {
	testData := []struct {
		Name     string
		Data     string
		Type     term.Curve
		Rate5    float64 // expected zero rate at 5y (decimal) including spread
		SpreadBP float64
	}{
		{"nss", `{"model":"nss","asof":"2021-03-31","currency":"CHF","source":"SNB","params":{"b0":-0.266372,"b1":-0.471343,"b2":5.68789,"b3":-5.12324,"t1":5.74881,"t2":4.14426},"spread_bp":25}`,
			term.NSS{}, snb20210331.ZeroRate(5) + 0.0025, 25},
		{"spline", `{"model":"spline","knots":[1,2,5],"discount_factors":[0.99,0.98,0.95],"spread_bp":0}`,
			&term.Spline{}, -math.Log(0.95) / 5, 0},
		{"flat", `{"model":"flat","params":{"r":2.0},"spread_bp":-10}`, term.Flat{}, 0.019, -10},
		{"flat without spread", `{"model":"flat","params":{"r":2.0}}`, term.Flat{}, 0.02, 0},
	}

	for _, test := range testData {
		for i := 0; i < 5; i++ {
			c, _, err := term.Parse([]byte(test.Data))
			if err != nil {
				t.Errorf("%s: %v", test.Name, err)
				break
			}
			base, s := term.Spread(c)
			if fmt.Sprintf("%T", base) != fmt.Sprintf("%T", test.Type) {
				t.Errorf("%s: parse returned wrong type: got: %T, expected %T", test.Name, base, test.Type)
			}
			if math.Abs(s*1e4-test.SpreadBP) > 1e-9 {
				t.Errorf("%s: spread %v bp, expected %v", test.Name, s*1e4, test.SpreadBP)
			}
			if r := c.ZeroRate(5); math.Abs(r-test.Rate5) > 1e-12 {
				t.Errorf("%s: ZeroRate(5) = %v, expected %v", test.Name, r, test.Rate5)
			}
		}
	}
}

func TestParse_LegacyFormatRejected(t *testing.T) {
	_, _, err := term.Parse([]byte(`{"r": 2.0, "spread": 0.0}`))
	want := `curve file has no "model" field (legacy format is no longer supported; see "fic curve template")`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("got %v, want an error containing %q", err, want)
	}
}

func TestParse_Meta(t *testing.T) {
	_, meta, err := term.Parse([]byte(`{"model":"flat","asof":"2021-08-31","currency":"CHF","source":"SNB","params":{"r":1}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !meta.AsOf.Equal(time.Date(2021, 8, 31, 0, 0, 0, 0, time.UTC)) || meta.Currency != "CHF" || meta.Source != "SNB" {
		t.Errorf("wrong metadata %+v", meta)
	}
}

func TestParse_Errors(t *testing.T) {
	for _, data := range []string{
		`not json`,
		`{"model":"unknown"}`,
		`{"model":"nss","params":{"b0":1,"b1":0,"b2":0,"b3":0,"t1":1}}`,              // missing t2
		`{"model":"nss","params":{"b0":1,"b1":0,"b2":0,"b3":0,"t1":0,"t2":1}}`,       // tau = 0
		`{"model":"nss","params":{"b0":1,"b1":0,"b2":0,"b3":0,"t1":1,"t2":1,"x":1}}`, // unknown param
		`{"model":"flat","params":{"r":2.0},"spread":10}`,                            // legacy spread key in new schema
		`{"model":"flat","params":{"r":2.0},"asof":"31.08.2021"}`,                    // invalid date
		`{"model":"spline","knots":[1,2],"discount_factors":[0.99]}`,                 // unequal lengths
		`{"model":"spline","knots":[2,1],"discount_factors":[0.98,0.99]}`,            // unsorted
		`{"model":"flat","params":{"r":2.0},"knots":[1],"discount_factors":[0.99]}`,  // knots for flat
		`{"spread": 1.0}`, // no model
		`{"b0": -0.266372, "b1": -0.471343, "b2": 5.68789, "b3": -5.12324, "t1": 5.74881, "t2": 4.14426, "spread": 0.0}`, // legacy NSS
		`{"maturities": [1.0, 2.0], "discountfactors": [0.99, 0.98]}`,                                                    // legacy spline
		`{"r": 2.0}`, // legacy flat
	} {
		if _, _, err := term.Parse([]byte(data)); err == nil {
			t.Errorf("expected error for %s", data)
		}
	}
}

func TestMarshal_RoundTrip(t *testing.T) {
	spline, err := term.NewSpline([]float64{0.5, 1, 5}, []float64{0.995, 0.99, 0.95})
	if err != nil {
		t.Fatal(err)
	}
	meta := term.Meta{AsOf: time.Date(2021, 8, 31, 0, 0, 0, 0, time.UTC), Currency: "CHF", Source: "SNB"}
	for _, c := range []term.Curve{
		snb20210331,
		term.WithSpread(snb20210331, 0.0042),
		term.Flat{Rate: 0.0123},
		term.WithSpread(spline, -0.001),
	} {
		data, err := term.Marshal(c, meta)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), `"model"`) || !strings.Contains(string(data), `"spread_bp"`) {
			t.Errorf("unexpected schema: %s", data)
		}
		back, gotMeta, err := term.Parse(data)
		if err != nil {
			t.Fatalf("%s: %v", data, err)
		}
		if gotMeta != meta {
			t.Errorf("metadata not preserved: %+v", gotMeta)
		}
		for _, x := range []float64{0, 0.3, 1, 4, 12} {
			if math.Abs(back.ZeroRate(x)-c.ZeroRate(x)) > 1e-15 {
				t.Errorf("%s: round trip ZeroRate(%v) %v != %v", data, x, back.ZeroRate(x), c.ZeroRate(x))
			}
		}
	}
	if _, err := term.Marshal(numericOnly{term.Flat{}}, term.Meta{}); err == nil {
		t.Errorf("expected error for unsupported curve type")
	}
}

// TestParse_Concurrent parses curves and applies spreads concurrently (run
// with -race)
func TestParse_Concurrent(t *testing.T) {
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := float64(i)
			c, _, err := term.Parse([]byte(fmt.Sprintf(`{"model": "flat", "params": {"r": %v}, "spread_bp": 0}`, r)))
			if err != nil {
				errs <- err
				return
			}
			s := term.WithSpread(c, float64(i)*1e-4)
			if got, expected := s.ZeroRate(1.0), r/100+float64(i)*1e-4; math.Abs(got-expected) > 1e-15 {
				errs <- fmt.Errorf("goroutine %d: got rate %v, expected %v", i, got, expected)
			}
			if c.ZeroRate(1.0) != r/100 {
				errs <- fmt.Errorf("goroutine %d: WithSpread modified the curve", i)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestMarshalRounding(t *testing.T) {
	// the published SNB values survive the decimal round trip exactly
	n, err := term.NewNSS(0.000347105, -0.000981406, -0.0187828, 0.0314724, 3.51767, 8.23272)
	if err != nil {
		t.Fatal(err)
	}
	data, err := term.Marshal(n, term.Meta{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"b0": 0.0347105`, `"b1": -0.0981406`, `"b2": -1.87828`, `"b3": 3.14724`, `"t1": 3.51767`, `"t2": 8.23272`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("missing %s in\n%s", want, data)
		}
	}
	// full-precision taus are rounded as well (14 significant digits)
	n.Tau1 = 1.0199921835123456789
	data, _ = term.Marshal(n, term.Meta{})
	if !strings.Contains(string(data), `"t1": 1.0199921835123`) || strings.Contains(string(data), "1.01999218351234") {
		t.Errorf("tau not rounded:\n%s", data)
	}
}
