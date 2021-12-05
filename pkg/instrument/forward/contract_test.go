package forward_test

import (
	"math"
	"testing"

	"github.com/konimarti/bonds/pkg/instrument/forward"
	"github.com/konimarti/bonds/pkg/term"
)

func TestContract(t *testing.T) {
	ts := term.Flat{2.0, 0.0}
	forwardPrice, _ := forward.ZeroBondPrice(1.0, 2.0, &ts)
	contract := forward.Contract{
		K: forwardPrice,
		F: ts.Z(2.0) / ts.Z(1.0),
		T: 1.0,
	}
	value := contract.PresentValue(&ts)
	expected := 0.0
	if math.Abs(value-expected) > 0.00001 {
		t.Errorf("wrong forward contract value; got: %v, expected: %v", value, expected)
	}
}

func TestZeroBondPrice(t *testing.T) {
	ts := term.Flat{2.0, 0.0}
	forwardPrice, err := forward.ZeroBondPrice(1.0, 2.0, &ts)
	if err != nil {
		t.Error(err)
	}
	expected := ts.Z(2.0) / ts.Z(1.0)
	if math.Abs(forwardPrice-expected) > 0.00001 {
		t.Errorf("wrong zero-bond forward price; got: %v, expected: %v", forwardPrice, expected)
	}
}

func TestFx(t *testing.T) {
	// t.Errorf("not implemented yet")
}

func TestStockPrice(t *testing.T) {
	// t.Errorf("not implemented yet")
}
