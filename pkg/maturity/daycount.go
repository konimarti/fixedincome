package maturity

import (
	"fmt"
	"strings"
	"time"

	"github.com/konimarti/daycount"
)

// DayCount is a day count convention for accrued interest. The day counts
// are those of github.com/konimarti/daycount.
type DayCount int

// Day count conventions. The zero value Thirty360E is the default.
const (
	// Thirty360E is "30E360" of github.com/konimarti/daycount (30E/360 with
	// the last day of February counted as the 30th); the default
	Thirty360E DayCount = iota
	// Eurobond is "EUROBOND" (30E/360, ICMA: day 31 is counted as 30)
	Eurobond
	// BondBasis is "BONDBASIS" (30/360 bond basis, ISDA)
	BondBasis
	// Act360 is "ACT360" (actual days / 360, money market)
	Act360
	// ActAct is "ACTACT" (ACT/ACT ICMA: actual days / actual days of the
	// coupon period)
	ActAct
)

// dayCountNames are the github.com/konimarti/daycount names of the
// conventions (index = DayCount).
var dayCountNames = [...]string{"30E360", "EUROBOND", "BONDBASIS", "ACT360", "ACTACT"}

// DayCountNames returns the names of the supported day count conventions in
// the order of the DayCount constants.
func DayCountNames() []string {
	return append([]string(nil), dayCountNames[:]...)
}

// String returns the github.com/konimarti/daycount name of the convention.
func (d DayCount) String() string {
	if d.Validate() != nil {
		return fmt.Sprintf("DayCount(%d)", int(d))
	}
	return dayCountNames[d]
}

// Validate returns an error for an unknown day count convention.
func (d DayCount) Validate() error {
	if d < Thirty360E || d > ActAct {
		return fmt.Errorf("maturity: invalid day count convention %d", int(d))
	}
	return nil
}

// ParseDayCount returns the day count convention with the given name
// (case-insensitive; spaces, "/", "_" and "-" are ignored). Accepted are the
// github.com/konimarti/daycount names and the aliases "30E/360" and
// "30E/360 ISDA" (Thirty360E), "30E/360 ICMA" and "Eurobond basis"
// (Eurobond), "30/360" and "Bond basis" (BondBasis), "ACT/360" and
// "Actual/360" (Act360), "ACT/ACT", "ACT/ACT ICMA" and "Actual/Actual ICMA"
// (ActAct). The empty name is the default Thirty360E.
func ParseDayCount(name string) (DayCount, error) {
	n := strings.ToUpper(name)
	for _, c := range []string{" ", "/", "_", "-"} {
		n = strings.ReplaceAll(n, c, "")
	}
	switch n {
	case "", "30E360", "30E360ISDA":
		return Thirty360E, nil
	case "EUROBOND", "EUROBONDBASIS", "30E360ICMA":
		return Eurobond, nil
	case "BONDBASIS", "30360":
		return BondBasis, nil
	case "ACT360", "ACTUAL360":
		return Act360, nil
	case "ACTACT", "ACTACTICMA", "ACTUALACTUALICMA", "ACTUALACTUAL":
		return ActAct, nil
	}
	return Thirty360E, fmt.Errorf("maturity: unknown day count convention %q (supported: %s)", name, strings.Join(dayCountNames[:], ", "))
}

// Days returns the number of days from start to end counted by the
// convention (daycount.Days on the UTC dates).
func (d DayCount) Days(start, end time.Time) (float64, error) {
	if err := d.Validate(); err != nil {
		return 0, err
	}
	return d.days(start, end), nil
}

// days counts the days without validation.
func (d DayCount) days(start, end time.Time) float64 {
	n, err := daycount.Days(Date(start), Date(end), dayCountNames[d])
	if err != nil {
		panic(err) // unreachable: all names are implemented by daycount
	}
	return n
}
