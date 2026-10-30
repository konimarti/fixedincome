package source

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/konimarti/fixedincome/pkg/maturity"
)

// DefaultBaseSIX is the reference data query service of SIX Swiss Exchange.
const DefaultBaseSIX = "https://www.six-group.com/fqs/ref.csv"

// sixSelect are the fields requested from the reference data service.
var sixSelect = []string{"ISIN", "IssuerNameShort", "CompositeRating", "MaturityDate", "CouponRate", "ClosingPrice",
	"InterestFrequencyCode", "AccruedInterestCalcCode", "Currency"}

// sixPerpetual is the MaturityDate of bonds without a fixed maturity.
const sixPerpetual = "99991231"

// SIXQuery selects the bonds of the segment BO (bonds) of SIX Swiss Exchange.
type SIXQuery struct {
	// Issuer is a case-insensitive substring of the short issuer name
	// (empty: all issuers).
	Issuer string
	// Ratings are exact values of the composite rating (AAA, AA, A, BBB,
	// ...); empty: all bonds, rated or not.
	Ratings []string
	// Currency is the trading currency, three upper-case letters (empty:
	// CHF).
	Currency string
}

var (
	sixRatingRE   = regexp.MustCompile(`^[A-Z][A-Za-z0-9+-]{0,5}$`)
	sixCurrencyRE = regexp.MustCompile(`^[A-Z]{3}$`)
)

func (q SIXQuery) currency() string {
	if q.Currency == "" {
		return "CHF"
	}
	return q.Currency
}

// Validate checks the query. The where expression of the service joins terms
// with "*" and values with "|", so these characters (and control characters)
// are refused in the issuer; characters outside ISO 8859-1 cannot be sent.
func (q SIXQuery) Validate() error {
	if !sixCurrencyRE.MatchString(q.currency()) {
		return fmt.Errorf("invalid currency %q (three upper-case letters)", q.Currency)
	}
	for _, r := range q.Ratings {
		if !sixRatingRE.MatchString(r) {
			return fmt.Errorf("invalid rating %q (e.g. AAA, AA, A, BBB)", r)
		}
	}
	if q.Issuer != "" {
		if strings.TrimSpace(q.Issuer) == "" {
			return errors.New("invalid issuer: empty")
		}
		for _, r := range q.Issuer {
			switch {
			case r == '*' || r == '|':
				return fmt.Errorf("invalid issuer %q: the characters * and | are not allowed", q.Issuer)
			case r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0):
				return fmt.Errorf("invalid issuer %q: control characters are not allowed", q.Issuer)
			case r > 0xff || r == utf8.RuneError:
				return fmt.Errorf("invalid issuer %q: only ISO 8859-1 characters are allowed", q.Issuer)
			}
		}
	}
	return nil
}

// Where returns the where expression of the service (not URL-encoded, the
// issuer in UTF-8). Call Validate first.
func (q SIXQuery) Where() string {
	w := "PortalSegment=BO*TradingBaseCurrency=" + q.currency()
	if q.Issuer != "" {
		w += "*IssuerNameShort_~" + q.Issuer
	}
	if len(q.Ratings) > 0 {
		w += "*CompositeRating=" + strings.Join(q.Ratings, "|")
	}
	return w
}

// String describes the filters for messages.
func (q SIXQuery) String() string {
	s := "currency " + q.currency()
	if q.Issuer != "" {
		s += ", issuer " + fmt.Sprintf("%q", q.Issuer)
	}
	if len(q.Ratings) > 0 {
		s += ", rating " + strings.Join(q.Ratings, "|")
	}
	return s
}

// latin1 converts s to ISO 8859-1 bytes (Validate ensures that all runes
// are below 0x100).
func latin1(s string) string {
	b := make([]byte, 0, len(s))
	for _, r := range s {
		b = append(b, byte(r))
	}
	return string(b)
}

// sixEscape percent-encodes the bytes of s (space as %20).
func sixEscape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// URL returns the request URL for the base. The service expects ISO 8859-1,
// so the percent-encoded where expression has the Latin-1 bytes of the
// issuer (a UTF-8 encoded umlaut matches nothing).
func (q SIXQuery) URL(base string) string {
	params := [][2]string{
		{"select", strings.Join(sixSelect, ",")},
		{"where", latin1(q.Where())},
		{"orderby", "MaturityDate"},
		{"page", "1"},
		{"pagesize", "99999"},
	}
	var sb strings.Builder
	sb.WriteString(base)
	for i, p := range params {
		if i == 0 {
			sb.WriteByte('?')
		} else {
			sb.WriteByte('&')
		}
		sb.WriteString(p[0] + "=" + sixEscape(p[1]))
	}
	return sb.String()
}

// Match reports whether a parsed bond satisfies the query, with the
// semantics of the service: issuer substring ignoring case, exact rating,
// currency if known (used for files that were not downloaded with the query).
func (q SIXQuery) Match(b SIXBond) bool {
	if q.Issuer != "" && !strings.Contains(strings.ToLower(b.Issuer), strings.ToLower(q.Issuer)) {
		return false
	}
	if len(q.Ratings) > 0 {
		ok := false
		for _, r := range q.Ratings {
			ok = ok || r == b.Rating
		}
		if !ok {
			return false
		}
	}
	return b.Currency == "" || b.Currency == q.currency()
}

// SIXBond is a bond of the SIX reference data.
type SIXBond struct {
	ISIN, Issuer, Rating, Currency string
	Maturity                       time.Time
	// CouponPct is the annual coupon in percent of face.
	CouponPct float64
	// PriceClean is the delayed closing price in percent of face
	// (HasPrice false: empty or 0 in the data).
	PriceClean float64
	HasPrice   bool
	// Freq is the number of coupons per year (0: the data has no frequency
	// column).
	Freq int
	// DayCount is the accrual convention (HasDayCount false: the data has
	// no accrued interest column).
	DayCount    maturity.DayCount
	HasDayCount bool
}

// SIXResult is the parsed reference data.
type SIXResult struct {
	// Bonds are the usable bonds sorted by maturity, then ISIN.
	Bonds []SIXBond
	// Rows is the number of data rows read, Filtered the number of rows
	// that do not match the query given to ParseSIX; Skipped counts the
	// other rows that are not in Bonds, by reason (see SkipReasons).
	Rows, Filtered int
	Skipped        map[string]int
}

// SkipReasons returns the reasons of the skipped rows in a stable order.
func (r SIXResult) SkipReasons() []string {
	var s []string
	for k := range r.Skipped {
		s = append(s, k)
	}
	sort.Strings(s)
	return s
}

// SkippedTotal is the number of skipped rows.
func (r SIXResult) SkippedTotal() int {
	n := 0
	for _, c := range r.Skipped {
		n += c
	}
	return n
}

// Skip reasons.
const (
	SkipPerpetual = "perpetual (no fixed maturity)"
	SkipFlat      = "no accrued interest, flat (A000)"
)

// sixDayCounts maps AccruedInterestCalcCode to the day count of the
// maturity package. Codes that are not here are skipped, not guessed.
//
//	A011 "30/360 (ICMA)"          30E/360 ICMA: day 31 counted as 30 = Eurobond.
//	A013 "Eurobond basis model 3" the Eurobond basis 30E/360 = Eurobond (the
//	                              "model" variants differ in the treatment of
//	                              the last day of February, not modelled).
//	A001 "30/360 (ISDA)"          ISDA 30/360 is the bond basis (31st becomes
//	                              30th only if the start is the 30th or 31st,
//	                              ISDA 2006 "30/360"); this is BondBasis, not
//	                              Thirty360E (= 30E/360 ISDA, the Eurobond
//	                              style without the start-day condition).
//	A004 "Actual/360"             Act360.
var sixDayCounts = map[string]maturity.DayCount{
	"A011": maturity.Eurobond,
	"A013": maturity.Eurobond,
	"A001": maturity.BondBasis,
	"A004": maturity.Act360,
}

// SIXDayCount returns the day count for an AccruedInterestCalcCode.
func SIXDayCount(code string) (maturity.DayCount, bool) {
	d, ok := sixDayCounts[strings.ToUpper(strings.TrimSpace(code))]
	return d, ok
}

// decodeSIX converts the data to UTF-8: ISO 8859-1 (the encoding of the
// service) unless the data is valid UTF-8 already (e.g. a file that was
// converted).
func decodeSIX(b []byte) string {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	if utf8.Valid(b) {
		return string(b)
	}
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

// excerpt returns a short sanitized start of a response for messages.
func excerpt(s string) string {
	s = strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)), " ")
	if len(s) > 160 {
		s = strings.ToValidUTF8(s[:160], "") + "..."
	}
	return s
}

// ParseSIX reads the CSV of the reference data service (semicolon separated,
// header row, ISO 8859-1 or UTF-8). Columns are found by name; MaturityDate,
// CouponRate and ClosingPrice are required, ISIN, IssuerNameShort,
// CompositeRating, InterestFrequencyCode, AccruedInterestCalcCode and
// Currency are optional (the fields of a missing column are empty; without
// the frequency or day count column those are not set). The syntax is strict:
// a quote inside an unquoted field, an unterminated quote, a row with a
// different number of fields than the header, or data that is not a CSV
// with the required header (e.g. the HTML error page of the service) is an
// error. Problems of a row (unsupported frequency or day count code,
// perpetual, invalid date, coupon or price) skip the row and are counted in
// SIXResult.Skipped. If q is not nil, only rows that match it (SIXQuery.Match)
// are used; the others are counted in SIXResult.Filtered.
func ParseSIX(r io.Reader, q *SIXQuery) (SIXResult, error) {
	raw, err := io.ReadAll(io.LimitReader(r, DefaultMaxBytes+1))
	if err != nil {
		return SIXResult{}, fmt.Errorf("reading: %w", err)
	}
	if int64(len(raw)) > DefaultMaxBytes {
		return SIXResult{}, fmt.Errorf("input larger than %d bytes", DefaultMaxBytes)
	}
	text := decodeSIX(raw)
	if strings.TrimSpace(text) == "" {
		return SIXResult{}, errors.New("empty input")
	}
	cr := csv.NewReader(strings.NewReader(strings.ReplaceAll(text, "\r\n", "\n")))
	cr.Comma = ';'
	cr.FieldsPerRecord = 0 // all rows as long as the header
	cr.LazyQuotes = false
	head, err := cr.Read()
	if err != nil {
		return SIXResult{}, fmt.Errorf("not a SIX CSV (%v); the data starts with: %s", err, excerpt(text))
	}
	col := map[string]int{}
	for i, n := range head {
		n = strings.ToLower(strings.TrimSpace(n))
		if _, dup := col[n]; !dup {
			col[n] = i
		}
	}
	for _, n := range []string{"maturitydate", "couponrate", "closingprice"} {
		if _, ok := col[n]; !ok {
			return SIXResult{}, fmt.Errorf("not a SIX CSV: the header lacks the columns MaturityDate, CouponRate, ClosingPrice; the data starts with: %s", excerpt(text))
		}
	}
	get := func(rec []string, name string) (string, bool) {
		i, ok := col[name]
		if !ok {
			return "", false
		}
		return clean(rec[i]), true
	}
	res := SIXResult{Skipped: map[string]int{}}
	skip := func(why string) { res.Skipped[why]++ }
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return SIXResult{}, fmt.Errorf("invalid CSV: %w", err)
		}
		res.Rows++
		var b SIXBond
		b.ISIN, _ = get(rec, "isin")
		b.Issuer, _ = get(rec, "issuernameshort")
		b.Rating, _ = get(rec, "compositerating")
		b.Currency, _ = get(rec, "currency")
		if q != nil && !q.Match(b) {
			res.Filtered++
			continue
		}
		mat, _ := get(rec, "maturitydate")
		if mat == sixPerpetual {
			skip(SkipPerpetual)
			continue
		}
		if b.Maturity, err = time.Parse("20060102", mat); err != nil || len(mat) != 8 {
			skip("invalid maturity " + fmt.Sprintf("%q", clip(mat)))
			continue
		}
		v, _ := get(rec, "couponrate")
		if b.CouponPct, err = parseValue(v); err != nil || b.CouponPct < 0 {
			skip("invalid coupon " + fmt.Sprintf("%q", clip(v)))
			continue
		}
		v, _ = get(rec, "closingprice")
		if v != "" {
			p, err := parseValue(v)
			if err != nil || p < 0 {
				skip("invalid price " + fmt.Sprintf("%q", clip(v)))
				continue
			}
			b.PriceClean, b.HasPrice = p, p > 0
		}
		if v, ok := get(rec, "interestfrequencycode"); ok {
			switch v {
			case "1", "2", "4":
				b.Freq = int(v[0] - '0')
			default:
				skip("unsupported frequency code " + fmt.Sprintf("%q", clip(v)))
				continue
			}
		}
		if v, ok := get(rec, "accruedinterestcalccode"); ok {
			if strings.EqualFold(v, "A000") {
				skip(SkipFlat)
				continue
			}
			d, known := SIXDayCount(v)
			if !known {
				skip("unsupported accrued interest code " + fmt.Sprintf("%q", clip(v)))
				continue
			}
			b.DayCount, b.HasDayCount = d, true
		}
		res.Bonds = append(res.Bonds, b)
	}
	if res.Rows == 0 {
		return res, nil
	}
	sort.SliceStable(res.Bonds, func(i, j int) bool {
		a, b := res.Bonds[i], res.Bonds[j]
		if !a.Maturity.Equal(b.Maturity) {
			return a.Maturity.Before(b.Maturity)
		}
		return a.ISIN < b.ISIN
	})
	return res, nil
}

// clean trims a field and replaces control characters by spaces.
func clean(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s))
}

// FetchSIX downloads and parses the bonds selected by q (the service applies
// the filters; only the currency of the rows is checked again). The query is
// validated first. An HTML error page of the service (HTTP 400 or an
// unexpected 200) is an error that shows a short excerpt.
func (c *Client) FetchSIX(ctx context.Context, q SIXQuery) (SIXResult, error) {
	if err := q.Validate(); err != nil {
		return SIXResult{}, err
	}
	base := c.base(DefaultBaseSIX, c.BaseSIX)
	u := q.URL(base)
	body, err := c.get(ctx, u)
	if err != nil {
		return SIXResult{}, err
	}
	res, err := ParseSIX(bytes.NewReader(body), &SIXQuery{Currency: q.currency()})
	if err != nil {
		return SIXResult{}, fmt.Errorf("%s: %w", shortURL(u), err)
	}
	return res, nil
}
