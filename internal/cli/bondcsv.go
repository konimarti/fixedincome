package cli

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/konimarti/fixedincome/pkg/maturity"
)

// bondRow is one line of a bond CSV file. Problems of the line do not abort
// reading: they are recorded in err (the status of the row).
type bondRow struct {
	line int
	id   string
	// issuer is an optional pass-through column
	issuer string
	// matRaw is the maturity field as read
	matRaw   string
	maturity time.Time
	// couponRaw, freqRaw and dcRaw are the fields as read (echoed for
	// failed rows)
	couponRaw, freqRaw, dcRaw string
	// coupon is a decimal (the file has percent)
	coupon   float64
	couponOK bool
	// price is the clean price in percent of face
	// hasPrice is false for a missing price (empty, N/A, -)
	price    float64
	hasPrice bool
	freq     maturity.Frequency // 0: not given
	dc       maturity.DayCount
	dcSet    bool
	// face is 0 if not given; redemption is valid if hasRed
	face       float64
	redemption float64
	hasRed     bool
	weight     float64
	err        error
}

// bondFile are the rows of a bond CSV file.
type bondFile struct {
	rows      []bondRow
	hasIssuer bool
}

// errMissingPrice is the reason why fit skips a row without a price.
var errMissingPrice = errors.New("missing price")

// bondColumnAliases maps header names (lower case) to canonical names.
var bondColumnAliases = map[string]string{
	"maturity": "maturity", "maturity_date": "maturity",
	"coupon_pct": "coupon", "coupon": "coupon",
	"price_clean": "price", "price": "price", "clean_price": "price", "quote": "price",
	"id": "id", "isin": "id",
	"freq": "freq", "frequency": "freq",
	"daycount": "daycount", "day_count": "daycount",
	"face": "face", "redemption": "redemption", "weight": "weight",
	"issuer": "issuer",
}

// missingTokens are field values that mean "no price".
var missingTokens = map[string]bool{"": true, "n/a": true, "na": true, "n.a.": true, "nan": true, "null": true, "none": true, "-": true, "--": true}

// readBondRows reads a bond CSV file: comma-separated, header row with the
// columns maturity, coupon_pct, price_clean and optionally id, freq,
// daycount, face, redemption, issuer (column names are case-insensitive, in
// any order; "coupon", "price", "frequency" are accepted as aliases). A
// missing price (empty, N/A, -) is not an error of the row (hasPrice is
// false). A
// file without header has the columns maturity, coupon_pct, price_clean, id.
// Dates are YYYY-MM-DD or DD.MM.YYYY, coupon in percent, price in percent of
// face. CRLF line endings, a byte order mark, quoted fields, empty lines
// and missing prices (empty, N/A, -) are handled; errors in a row are
// recorded in the row (see bondRow.err).
func readBondRows(r io.Reader) (*bondFile, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	data = bytes.ReplaceAll(data, []byte("\r"), []byte("\n"))
	cr := csv.NewReader(bytes.NewReader(data))
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	cr.LazyQuotes = true
	var records [][]string
	var lines []int
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("invalid CSV: %w", err)
		}
		line, _ := cr.FieldPos(0)
		if len(rec) == 1 && strings.TrimSpace(rec[0]) == "" {
			continue
		}
		records = append(records, rec)
		lines = append(lines, line)
	}
	if len(records) > 0 && len(records[0]) == 1 {
		for _, d := range []struct{ sep, name string }{{";", "semicolons"}, {"\t", "tabs"}} {
			if strings.Contains(records[0][0], d.sep) {
				return nil, fmt.Errorf("CSV line 1 is separated by %s; bond files must be comma-separated", d.name)
			}
		}
	}
	col := map[string]int{"maturity": 0, "coupon": 1, "price": 2, "id": 3}
	start := 0
	if len(records) > 0 && looksLikeHeader(records[0]) {
		col = map[string]int{}
		for i, name := range records[0] {
			key := strings.ToLower(strings.TrimSpace(name))
			canon, ok := bondColumnAliases[key]
			if !ok {
				continue // unknown columns are ignored
			}
			if _, dup := col[canon]; dup {
				return nil, fmt.Errorf("CSV header: duplicate column %q", key)
			}
			col[canon] = i
		}
		for _, want := range [][2]string{{"maturity", "maturity"}, {"coupon", "coupon_pct"}, {"price", "price_clean"}} {
			if _, ok := col[want[0]]; !ok {
				return nil, fmt.Errorf("CSV header %v: missing column %q", records[0], want[1])
			}
		}
		start = 1
	}
	bf := &bondFile{}
	_, bf.hasIssuer = col["issuer"]
	get := func(row []string, name string) (string, bool) {
		i, ok := col[name]
		if !ok || i >= len(row) {
			return "", false
		}
		return strings.TrimSpace(row[i]), true
	}
	for n := start; n < len(records); n++ {
		row := records[n]
		b := bondRow{line: lines[n]}
		b.id, _ = get(row, "id")
		if b.id == "" {
			b.id = fmt.Sprintf("row%d", b.line)
		}
		b.issuer, _ = get(row, "issuer")
		fail := func(format string, args ...any) {
			if b.err == nil {
				b.err = fmt.Errorf(format, args...)
			}
		}
		v, _ := get(row, "maturity")
		b.matRaw = v
		if d, err := parseDate(v); err == nil {
			b.maturity = d
		} else {
			fail("invalid maturity %q", v)
		}
		v, _ = get(row, "coupon")
		b.couponRaw = v
		if v == "" {
			fail("missing coupon")
		} else if c, err := parseFinite(v); err == nil {
			b.coupon, b.couponOK = fromPct(c), true
		} else {
			fail("invalid coupon %q", v)
		}
		v, _ = get(row, "price")
		switch {
		case missingTokens[strings.ToLower(v)]:
			// no price: valued on the curve in batch mode, skipped by fit
		default:
			if p, err := parseFinite(v); err != nil {
				fail("invalid price %q", v)
			} else if !(p > 0) {
				fail("non-positive price %q", v)
			} else {
				b.price, b.hasPrice = p, true
			}
		}
		b.freqRaw, _ = get(row, "freq")
		b.dcRaw, _ = get(row, "daycount")
		if v, ok := get(row, "freq"); ok && v != "" {
			if f, err := maturity.ParseFrequency(v); err != nil {
				fail("invalid freq %q (valid: 1, 2, 3, 4, 6, 12)", v)
			} else {
				b.freq = f
			}
		}
		if v, ok := get(row, "daycount"); ok && v != "" {
			if d, err := maturity.ParseDayCount(v); err != nil {
				fail("invalid daycount %q (valid: %s)", v, strings.Join(maturity.DayCountNames(), ", "))
			} else {
				b.dc, b.dcSet = d, true
			}
		}
		if v, ok := get(row, "face"); ok && v != "" {
			if f, err := parseFinite(v); err != nil || !(f > 0) {
				fail("invalid face %q", v)
			} else {
				b.face = f
			}
		}
		if v, ok := get(row, "redemption"); ok && v != "" {
			if f, err := parseFinite(v); err != nil || f < 0 {
				fail("invalid redemption %q", v)
			} else {
				b.redemption, b.hasRed = f, true
			}
		}
		if v, ok := get(row, "weight"); ok && v != "" {
			if f, err := parseFinite(v); err != nil || !(f > 0) {
				fail("invalid weight %q", v)
			} else {
				b.weight = f
			}
		}
		bf.rows = append(bf.rows, b)
	}
	if len(bf.rows) == 0 {
		return nil, errors.New("no bonds in the CSV data")
	}
	return bf, nil
}

// parseFinite parses a finite number.
func parseFinite(s string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, errors.New("not finite")
	}
	return v, nil
}

// looksLikeHeader reports whether the row is a header: the first field is
// not a date and none of the others is numeric.
func looksLikeHeader(row []string) bool {
	if len(row) < 2 {
		return false
	}
	if _, err := parseDate(row[0]); err == nil {
		return false
	}
	for _, f := range row[1:] {
		if _, err := strconv.ParseFloat(strings.TrimSpace(f), 64); err == nil {
			return false
		}
	}
	return true
}

// openInput opens a file or stdin (path "-").
func openInput(e *env, path string) (io.Reader, func() error, error) {
	if path == "-" {
		return e.stdin, func() error { return nil }, nil
	}
	f, err := openFile(path)
	if err != nil {
		return nil, nil, err
	}
	return f, f.Close, nil
}
