package option_test

import (
	"math"
	"testing"

	"github.com/konimarti/bonds/pkg/option"
	"github.com/konimarti/bonds/pkg/term"
)

var (
	testOption = option.European{
		option.Call,
		110.0,
		100.0,
		2.0,
		0.0,
		0.3,
	}
	ts = term.Flat{2.0, 0.0}
)

func TestN(t *testing.T) {
	tests := []struct {
		Value    float64
		Expected float64
	}{
		{
			Value:    0.5311,
			Expected: 0.7023,
		},
		{
			Value:    -0.5311,
			Expected: 0.2977,
		},
		{
			Value:    0.1068,
			Expected: 0.5425,
		},
		{
			Value:    -0.1068,
			Expected: 0.4575,
		},
	}

	for _, test := range tests {
		got := option.N(test.Value)
		if math.Abs(got-test.Expected) > 0.0001 {
			t.Errorf("N function failed; Got %v, Expected %v", got, test.Expected)
		}
	}
}

func TestD1(t *testing.T) {
	value := option.D1(testOption.S, testOption.K, testOption.T, testOption.Q, testOption.Vola, &ts)
	expected := 0.5311
	if math.Abs(value-expected) > 0.0001 {
		t.Errorf("D1 function failed; Got %v, Expected %v", value, expected)
	}
}

func TestD2(t *testing.T) {
	d1 := option.D1(testOption.S, testOption.K, testOption.T, testOption.Q, testOption.Vola, &ts)
	value := option.D2(d1, testOption.T, testOption.Vola)
	expected := 0.1068
	if math.Abs(value-expected) > 0.0001 {
		t.Errorf("D2 function failed; Got %v, Expected %v", value, expected)
	}
}

func TestEuropeanCallOption(t *testing.T) {
	call := testOption
	call.Type = option.Call

	value := call.PresentValue(&ts)
	expected := 25.1291

	if math.Abs(value-expected) > 0.0001 {
		t.Errorf("pricing of European call failed; Got %v, Expected %v", value, expected)
	}
}

func TestEuropeanPutOption(t *testing.T) {
	put := testOption
	put.Type = option.Put

	value := put.PresentValue(&ts)
	expected := 11.2080

	if math.Abs(value-expected) > 0.0001 {
		t.Errorf("pricing of European put failed; Got %v, Expected %v", value, expected)
	}
}
