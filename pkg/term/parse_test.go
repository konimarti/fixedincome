package term_test

import (
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/konimarti/fixedincome/pkg/term"
)

func TestParse(t *testing.T) {

	testData := []struct {
		Data []byte
		Type interface{}
	}{
		{
			Data: []byte(" { \"b0\": -0.596356, \"b1\": -0.153952, \"b2\": 5.79009, \"b3\": -4.69599, \"t1\": 6.5912, \"t2\": 4.63027, \"spread\": 0.0 } "),
			Type: &term.NelsonSiegelSvensson{},
		},
		{
			Data: []byte(" { \"maturities\": [1.0, 2.0, 3.0], \"discountfactors\": [0.99, 0.98, 0.97], \"spread\": 0.0 } "),
			Type: &term.Spline{},
		},
		{
			Data: []byte(" { \"r\": 0.0, \"spread\": 0.0 } "),
			Type: &term.Flat{},
		},
	}

	for i, test := range testData {
		ts, err := term.Parse(test.Data)
		if err != nil {
			t.Errorf("test %d: %v", i+1, err)
		}
		if reflect.TypeOf(ts) != reflect.TypeOf(test.Type) {
			t.Errorf("test %d: parse returned wrong type: got: %T, expected %T", i+1, ts, test.Type)
		}
	}
}

// TestParse_Independent checks that every call to Parse returns a new,
// independent term structure
func TestParse_Independent(t *testing.T) {
	data := []byte(`{ "b0": -0.596356, "b1": -0.153952, "b2": 5.79009, "b3": -4.69599, "t1": 6.5912, "t2": 4.63027, "spread": 0.0 }`)

	ts1, err := term.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	ts2, err := term.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if ts1 == ts2 {
		t.Fatalf("parse returned the same instance twice")
	}

	ref := ts2.Rate(5.0)
	ts1.SetSpread(100.0)
	if ts2.Rate(5.0) != ref {
		t.Errorf("changing the spread of one parsed curve changed the other one; got: %v, expected: %v", ts2.Rate(5.0), ref)
	}

	other := []byte(`{ "b0": 1.0, "b1": 0.0, "b2": 0.0, "b3": 0.0, "t1": 1.0, "t2": 1.0, "spread": 0.0 }`)
	ts3, err := term.Parse(other)
	if err != nil {
		t.Fatal(err)
	}
	if ts2.Rate(5.0) != ref || ts3.Rate(5.0) == ref {
		t.Errorf("parsing different data changed a previously parsed curve")
	}
}

// TestParse_Concurrent parses curves concurrently (run with -race)
func TestParse_Concurrent(t *testing.T) {
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := float64(i)
			ts, err := term.Parse([]byte(fmt.Sprintf(`{"r": %v, "spread": 0.0}`, r)))
			if err != nil {
				errs <- err
				return
			}
			ts.SetSpread(float64(i))
			if got, expected := ts.Rate(1.0), r+float64(i)*0.01; got != expected {
				errs <- fmt.Errorf("goroutine %d: got rate %v, expected %v", i, got, expected)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// TestParse_Order checks that the most specific term structure is chosen if
// the JSON data contains keys of several term structures
func TestParse_Order(t *testing.T) {
	data := []byte(`{ "b0": 1.0, "b1": 0.0, "b2": 0.0, "b3": 0.0, "t1": 1.0, "t2": 1.0, "r": 5.0, "spread": 0.0 }`)
	for i := 0; i < 20; i++ {
		ts, err := term.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := ts.(*term.NelsonSiegelSvensson); !ok {
			t.Fatalf("parse returned wrong type: got: %T, expected %T", ts, &term.NelsonSiegelSvensson{})
		}
	}
}

// TestParse_OptionalSpread checks that the spread is optional (default 0)
func TestParse_OptionalSpread(t *testing.T) {
	testData := []struct {
		Data []byte
		Type interface{}
	}{
		{[]byte(`{ "b0": 1.0, "b1": 0.0, "b2": 0.0, "b3": 0.0, "t1": 1.0, "t2": 1.0 }`), &term.NelsonSiegelSvensson{}},
		{[]byte(`{ "maturities": [1.0, 2.0], "discountfactors": [0.99, 0.98] }`), &term.Spline{}},
		{[]byte(`{ "r": 2.0 }`), &term.Flat{}},
	}
	for i, test := range testData {
		ts, err := term.Parse(test.Data)
		if err != nil {
			t.Errorf("test %d: %v", i+1, err)
			continue
		}
		if reflect.TypeOf(ts) != reflect.TypeOf(test.Type) {
			t.Errorf("test %d: parse returned wrong type: got: %T, expected %T", i+1, ts, test.Type)
		}
		if g, ok := ts.(interface{ GetSpread() float64 }); !ok || g.GetSpread() != 0.0 {
			t.Errorf("test %d: spread is not 0", i+1)
		}
	}
}
