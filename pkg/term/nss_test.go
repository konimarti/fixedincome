package term_test

import (
	"math"
	"testing"

	"github.com/konimarti/fixedincome/pkg/term"
)

func TestNelsonSiegelSvensson(t *testing.T) {
	// NSS parameters as of 2021-03-31 from https://data.snb.ch/en/topics/ziredev#!/cube/rendopar
	n := term.NelsonSiegelSvensson{
		-0.266372,
		-0.471343,
		5.68789,
		-5.12324,
		5.74881,
		4.14426,
		0.0, // spread
	}

	// data for 2020-03 from https://data.snb.ch/en/topics/ziredev#!/cube/rendoblim
	data := []struct {
		M             float64
		RateInPercent float64
	}{
		{1, -0.782},
		{2, -0.776},
		{3, -0.736},
		{4, -0.677},
		{5, -0.606},
		{6, -0.532},
		{7, -0.459},
		{8, -0.389},
		{9, -0.325},
		{10, -0.267},
		{15, -0.073},
		{20, -0.001},
		{30, -0.007},
	}

	for _, test := range data {
		got := n.Rate(test.M)
		expected := math.Log(1.0+test.RateInPercent/100.0) * 100.0
		if math.Abs(got-expected) > 0.001 {
			t.Errorf("got %f, but wanted %f, failed for maturity %f", got, expected, test.M)
		}
	}
}
