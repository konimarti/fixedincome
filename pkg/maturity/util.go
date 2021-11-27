package maturity

import "time"

//DaysInYear calculates the number of days in the given year
func DaysInYear(year int) float64 {
	return float64(time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC).YearDay())
}

//LastDay of a given year
func LastDay(year int) time.Time {
	return time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC)
}

//FirstDay of a given year
func FirstDay(year int) time.Time {
	return time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
}

// DifferenceInYears returns the difference between two dates in years (Act/Act)
// We assume a year has 365.25 days.
func DifferenceInYears(start, stop time.Time) float64 {
	return float64(stop.Sub(start).Hours()) / 24.0 / 365.25
}
