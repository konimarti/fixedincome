package source

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// snbCodes maps the (lower-case) codes of the dimension "d0" of the SNB
// cube rendopar to the NSS parameters.
var snbCodes = map[string]int{
	"b0": 0, "b1": 1, "b2": 2, "b3": 3, "t1": 4, "t2": 5,
	"beta0": 0, "beta1": 1, "beta2": 2, "beta3": 3, "tau1": 4, "tau2": 5,
}

// SNBSource is the Meta.Source of curves from the SNB.
const SNBSource = "SNB rendopar"

// ParseSNB parses the CSV of the SNB cube "rendopar" (semicolon separated,
// optional byte order mark, CRLF or LF, a preamble of lines such as CubeId
// and PublishingDate, then a header row with the columns Date, D0 and Value;
// the columns are found by name and the codes b0..b3, t1, t2 are case
// insensitive). Rows with an empty value and unknown codes are ignored.
//
// asof selects the date of the parameters. The zero time selects the latest
// date for which all six parameters are available; otherwise that date must
// be complete, else the error names the nearest earlier complete date. A
// non-numeric value of a known code, a bad date, a repeated (date, code) or
// a line longer than 1 MiB is an error naming the line.
//
// The result is in the library convention (betas as decimals) with the
// currency CHF and the source "SNB rendopar".
func ParseSNB(r io.Reader, asof time.Time) (Curve, error) {
	c := collection{}
	err := readTable(r, ';', []string{"date", "d0", "value"}, func(line int, f []string, col map[string]int) error {
		code, err := field(f, col, "d0", line)
		if err != nil {
			return err
		}
		i, ok := snbCodes[strings.ToLower(code)]
		if !ok {
			return nil
		}
		val, err := field(f, col, "value", line)
		if err != nil {
			return err
		}
		if val == "" {
			return nil
		}
		ds, err := field(f, col, "date", line)
		if err != nil {
			return err
		}
		return addRow(c, line, ds, i, val)
	})
	if err != nil {
		return Curve{}, fmt.Errorf("SNB: %w", err)
	}
	return c.pick(asof, "SNB", "CHF", SNBSource)
}

func addRow(c collection, line int, date string, i int, val string) error {
	d, err := parseISODate(date)
	if err != nil {
		return fmt.Errorf("line %d: %v", line, err)
	}
	v, err := parseValue(val)
	if err != nil {
		return fmt.Errorf("line %d: %s on %s: %v", line, paramNames[i], d, err)
	}
	return c.add(d, i, v, line)
}

// ecbCodes maps DATA_TYPE_FM to the NSS parameters.
var ecbCodes = map[string]int{"beta0": 0, "beta1": 1, "beta2": 2, "beta3": 3, "tau1": 4, "tau2": 5}

// ParseECB parses the CSV (comma separated, format "csvdata") of the ECB
// yield curve dataflow YC for the parameters BETA0..BETA3, TAU1, TAU2. The
// columns DATA_TYPE_FM, TIME_PERIOD (YYYY-MM-DD) and OBS_VALUE are found by
// name (case insensitive); all other columns and data types (e.g. the spot
// rates SR_1Y) are ignored, as are rows with an empty observation. asof and
// the error behaviour are as for ParseSNB. The currency is EUR; the source
// is "ECB AAA" or "ECB all issuers" if the column INSTRUMENT_FM says so
// (G_N_A, G_N_C), else "ECB". A file that holds more than one value of
// INSTRUMENT_FM, CURRENCY or REF_AREA (e.g. AAA and all-issuer parameters
// mixed) is an error, as is another currency than EUR.
func ParseECB(r io.Reader, asof time.Time) (Curve, error) {
	c := collection{}
	insts, ccys, areas := map[string]bool{}, map[string]bool{}, map[string]bool{}
	err := readTable(r, ',', []string{"data_type_fm", "time_period", "obs_value"}, func(line int, f []string, col map[string]int) error {
		code, err := field(f, col, "data_type_fm", line)
		if err != nil {
			return err
		}
		i, ok := ecbCodes[strings.ToLower(code)]
		if !ok {
			return nil
		}
		val, err := field(f, col, "obs_value", line)
		if err != nil {
			return err
		}
		if val == "" {
			return nil
		}
		ds, err := field(f, col, "time_period", line)
		if err != nil {
			return err
		}
		for name, set := range map[string]map[string]bool{"instrument_fm": insts, "currency": ccys, "ref_area": areas} {
			if k, ok := col[name]; ok && k < len(f) {
				set[strings.ToUpper(strings.TrimSpace(f[k]))] = true
			}
		}
		return addRow(c, line, ds, i, val)
	})
	if err != nil {
		return Curve{}, fmt.Errorf("ECB: %w", err)
	}
	for name, set := range map[string]map[string]bool{"INSTRUMENT_FM": insts, "CURRENCY": ccys, "REF_AREA": areas} {
		if len(set) > 1 {
			return Curve{}, fmt.Errorf("ECB: the data mixes several values of %s (%s); download one series only", name, keys(set))
		}
	}
	for _, x := range []struct {
		name string
		set  map[string]bool
		want string
	}{{"CURRENCY", ccys, "EUR"}, {"REF_AREA", areas, "U2"}} {
		for v := range x.set {
			if v != x.want {
				return Curve{}, fmt.Errorf("ECB: unsupported %s %q (only the euro area curve, %s)", x.name, v, x.want)
			}
		}
	}
	src := "ECB"
	for v := range insts {
		src = ecbInstrument(v)
	}
	return c.pick(asof, "ECB", "EUR", src)
}

func keys(m map[string]bool) string {
	var k []string
	for v := range m {
		k = append(k, strconv.Quote(v))
	}
	sort.Strings(k)
	return strings.Join(k, ", ")
}

// ecbInstrument returns the source name for an INSTRUMENT_FM value.
func ecbInstrument(s string) string {
	switch strings.ToUpper(s) {
	case "G_N_A":
		return "ECB AAA"
	case "G_N_C":
		return "ECB all issuers"
	}
	return "ECB"
}
