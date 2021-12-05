package maturity_test

import (
	"math"
	"testing"
	"time"

	"github.com/konimarti/bonds/pkg/maturity"
)

func TestSchedule(t *testing.T) {

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
			Expected:          []float64{8.0, 7.5, 7.0, 6.5, 6.0, 5.5, 5.0, 4.5, 4.0, 3.5, 3.0, 2.5, 2.0, 1.5, 1.0, 0.5},
			ExpectedFrequency: 2,
			ExpectedRemaining: 8.0,
		},
		{
			Quote:             time.Date(2021, 01, 01, 0, 0, 0, 0, time.UTC),
			Maturity:          time.Date(2024, 07, 01, 0, 0, 0, 0, time.UTC),
			Frequency:         0,
			Expected:          []float64{3.5, 2.5, 1.5, 0.5},
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
			Expected:          []float64{8.5, 7.5, 6.50, 5.50, 4.50, 3.50, 2.50, 1.50, 0.50},
			ExpectedFrequency: 1,
			ExpectedRemaining: 8.5,
		},
	}

	for nr, test := range testData {
		m := maturity.Schedule{
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

func TestSchedule_DayCountFraction(t *testing.T) {

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
		m := maturity.Schedule{
			Settlement: test.Quote,
			Maturity:   test.Maturity,
			Frequency:  test.Frequency,
		}

		tolerance := 0.0001

		value := m.DayCountFraction()

		if math.Abs(value-test.Expected) > tolerance {
			t.Errorf("days do not match for test nr %d, got: %f, expected: %f", nr, value, test.Expected)
		}
	}
}

func TestSchedule_YearsToMaturity(t *testing.T) {

	testData := []struct {
		Quote    time.Time
		Maturity time.Time
		Expected float64
	}{
		{
			Quote:    time.Date(2021, 11, 30, 0, 0, 0, 0, time.UTC),
			Maturity: time.Date(2022, 05, 25, 0, 0, 0, 0, time.UTC),
			Expected: 0.486111,
		},
		{
			Quote:    time.Date(2021, 11, 30, 0, 0, 0, 0, time.UTC),
			Maturity: time.Date(2023, 02, 11, 0, 0, 0, 0, time.UTC),
			Expected: 1.197222,
		},
		{
			Quote:    time.Date(2021, 11, 30, 0, 0, 0, 0, time.UTC),
			Maturity: time.Date(2024, 06, 11, 0, 0, 0, 0, time.UTC),
			Expected: 2.530556,
		},
		{
			Quote:    time.Date(2021, 11, 30, 0, 0, 0, 0, time.UTC),
			Maturity: time.Date(2025, 07, 24, 0, 0, 0, 0, time.UTC),
			Expected: 3.650000,
		},
		{
			Quote:    time.Date(2021, 11, 30, 0, 0, 0, 0, time.UTC),
			Maturity: time.Date(2033, 04, 8, 0, 0, 0, 0, time.UTC),
			Expected: 11.355556,
		},
		{
			Quote:    time.Date(2021, 11, 30, 0, 0, 0, 0, time.UTC),
			Maturity: time.Date(2058, 05, 30, 0, 0, 0, 0, time.UTC),
			Expected: 36.50000,
		},
	}

	tolerance := 0.000001
	for nr, test := range testData {
		m := maturity.Schedule{
			Settlement: test.Quote,
			Maturity:   test.Maturity,
			Frequency:  1,
			Basis:      "30E360",
		}

		if math.Abs(m.YearsToMaturity()-test.Expected) > tolerance {
			t.Errorf("Wrong remaining years for test nr %d, got: %f, expected: %f", nr, m.YearsToMaturity(), test.Expected)
		}

	}
}
