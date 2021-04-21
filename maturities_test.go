package bonds_test

import (
	"math"
	"testing"
	"time"

	"github.com/konimarti/bonds"
)

func TestMaturities(t *testing.T) {

	testData := []struct {
		Quote             time.Time
		Maturity          time.Time
		Frequency         int
		Expected          []float64
		ExpectedFrequency int
		ExpectedRemaining float64
	}{
		{
			Quote:             time.Date(2021, 01, 01, 0, 0, 0, 0, time.UTC),
			Maturity:          time.Date(2029, 01, 01, 0, 0, 0, 0, time.UTC),
			Frequency:         1,
			Expected:          []float64{8.0, 7.0, 6.0, 5.0, 4.0, 3.0, 2.0, 1.0},
			ExpectedFrequency: 1,
			ExpectedRemaining: 8.0,
		},
		{
			Quote:             time.Date(2021, 01, 01, 0, 0, 0, 0, time.UTC),
			Maturity:          time.Date(2029, 01, 01, 0, 0, 0, 0, time.UTC),
			Frequency:         2,
			Expected:          []float64{8.0, 7.5, 7.0, 6.494, 6.0, 5.494, 5.0, 4.494, 4.0, 3.494, 3.0, 2.494, 2.0, 1.494, 1.0, 0.5},
			ExpectedFrequency: 2,
			ExpectedRemaining: 8.0,
		},
		{
			Quote:             time.Date(2021, 01, 01, 0, 0, 0, 0, time.UTC),
			Maturity:          time.Date(2024, 07, 01, 0, 0, 0, 0, time.UTC),
			Frequency:         0,
			Expected:          []float64{3.5, 2.494, 1.494, 0.5},
			ExpectedFrequency: 1,
			ExpectedRemaining: 3.5,
		},
		{
			Quote:             time.Date(2021, 01, 01, 0, 0, 0, 0, time.UTC),
			Maturity:          time.Date(2019, 01, 01, 0, 0, 0, 0, time.UTC),
			Frequency:         1,
			Expected:          []float64{},
			ExpectedFrequency: 1,
			ExpectedRemaining: 0.0,
		},
		{
			Quote:             time.Date(2021, 4, 16, 0, 0, 0, 0, time.UTC),
			Maturity:          time.Date(2029, 10, 16, 0, 0, 0, 0, time.UTC),
			Frequency:         1,
			Expected:          []float64{8.506, 7.506, 6.50, 5.50, 4.50, 3.50, 2.50, 1.50, 0.50},
			ExpectedFrequency: 1,
			ExpectedRemaining: 8.506,
		},
	}

	for nr, test := range testData {
		m := bonds.Maturities{
			Settlement: test.Quote,
			Maturity:   test.Maturity,
			Frequency:  test.Frequency,
		}

		maturities := m.M()
		tolerance := 0.005

		if len(maturities) != len(test.Expected) {
			t.Errorf("length of maturities does not match")
			return
		}

		for i, m := range maturities {
			if math.Abs(m-test.Expected[i]) > tolerance {
				t.Errorf("maturities do not match for test nr %d, got: %f, expected: %f", nr, m, test.Expected[i])
			}
		}

		if math.Abs(m.YearsToMaturity()-test.ExpectedRemaining) > tolerance {
			t.Errorf("Wrong remaining years for test nr %d, got: %f, expected: %f", nr, m.YearsToMaturity(), test.ExpectedRemaining)
		}

		if m.Compounding() != test.ExpectedFrequency {
			t.Errorf("GetFrequency failed for test nr %d, got: %d, expected: %d", nr, m.Compounding(), test.ExpectedFrequency)
		}

	}
}

func TestMaturitiesDaysSinceLastPayment(t *testing.T) {

	testData := []struct {
		Quote     time.Time
		Maturity  time.Time
		Frequency int
		Expected  float64
	}{
		{
			Quote:     time.Date(2021, 4, 17, 0, 0, 0, 0, time.UTC),
			Maturity:  time.Date(2022, 5, 25, 0, 0, 0, 0, time.UTC),
			Frequency: 1,
			Expected:  0.89444,
		},
		{
			Quote:     time.Date(2015, 6, 16, 0, 0, 0, 0, time.UTC),
			Maturity:  time.Date(2018, 4, 15, 0, 0, 0, 0, time.UTC),
			Frequency: 2,
			Expected:  0.16944,
		},
	}

	for nr, test := range testData {
		m := bonds.Maturities{
			Settlement: test.Quote,
			Maturity:   test.Maturity,
			Frequency:  test.Frequency,
		}

		tolerance := 0.001

		value := m.DaysSinceLastCouponInYears()

		if math.Abs(value-test.Expected) > tolerance {
			t.Errorf("days do not match for test nr %d, got: %f, expected: %f", nr, value, test.Expected)
		}
	}
}
