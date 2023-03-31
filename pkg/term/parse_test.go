package term_test

import (
	"reflect"
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
			Data: []byte(" { \"maturities\": null, \"discountfactors\": null, \"spread\": 0.0 } "),
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
