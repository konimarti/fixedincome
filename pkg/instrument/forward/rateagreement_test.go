package forward_test

import (
	"math"
	"testing"

	"github.com/konimarti/bonds/pkg/instrument/forward"
	"github.com/konimarti/bonds/pkg/term"
)

func TestRateAgreement(t *testing.T) {
	ts := term.Flat{2.0, 0.0}
	t1, t2 := 1.0, 2.0
	m := ts.Z(t1) / ts.Z(t2)
	fra := forward.RateAgreement{
		N:  1e6,
		M:  m,
		T1: t1,
		T2: t2,
	}
	value := fra.PresentValue(&ts)
	expected := 0.0
	if math.Abs(value-expected) > 0.00001 {
		t.Errorf("wrong forward rate agreement value; got: %v, expected: %v", value, expected)
	}
}
