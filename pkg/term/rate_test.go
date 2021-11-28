package term_test

import (
	"math"
	"testing"

	"github.com/konimarti/bonds/pkg/term"
)

var (
	annualRate = 12.0
	ccRate     = 11.332869
)

func TestToCC(t *testing.T) {
	if math.Abs(term.ToCC(annualRate)-ccRate) > 1e-6 {
		t.Errorf("conversion from annual to cc rate failed")
	}
}

func TestToAnnual(t *testing.T) {
	if math.Abs(term.ToAnnual(ccRate)-annualRate) > 1e-6 {
		t.Errorf("conversion from cc to annual rate failed")
	}
}
