package cli

import (
	"flag"
	"fmt"
	"sort"
	"strings"

	"github.com/konimarti/fixedincome/pkg/source"
)

// bondFetchCols are the columns of the bond CSV written by "bond fetch".
var bondFetchCols = []string{"maturity", "coupon_pct", "price_clean", "id", "freq", "daycount", "issuer", "rating"}

func buildBondFetch(e *env) (*flag.FlagSet, func() error) {
	fs := flag.NewFlagSet("bond fetch", flag.ContinueOnError)
	src := fs.String("source", "", "data source six (required)")
	issuer := fs.String("issuer", "", "six: short issuer name contains this text, ignoring case (default: all issuers)")
	rating := fs.String("rating", "", "six: composite ratings, comma separated, e.g. AAA,AA (default: all, rated or not)")
	currency := fs.String("currency", "CHF", "six: trading currency, three upper-case letters")
	file := fs.String("file", "", "parse this downloaded SIX CSV instead of the network (- for stdin; the filters are applied to it)")
	timeout := fs.Duration("timeout", source.DefaultTimeout, "time limit of the download")
	format := fs.String("format", "csv", "output format (csv)")
	out := fs.String("o", "-", "output bond CSV file (- for stdout)")
	return fs, func() error {
		set := setFlags(fs)
		switch strings.ToLower(*src) {
		case "":
			return usagef("--source is required (six)")
		case "six":
		default:
			return usagef("invalid --source %q (six)", *src)
		}
		if !strings.EqualFold(*format, "csv") {
			return usagef("invalid --format %q: bond fetch writes csv only", *format)
		}
		q := source.SIXQuery{Issuer: *issuer, Currency: *currency}
		if set["issuer"] && strings.TrimSpace(*issuer) == "" {
			return usagef("invalid --issuer: empty")
		}
		if set["rating"] {
			for _, r := range strings.Split(*rating, ",") {
				q.Ratings = append(q.Ratings, strings.TrimSpace(r))
			}
		}
		if err := q.Validate(); err != nil {
			return usagef("%v", err)
		}
		if *timeout <= 0 {
			return usagef("invalid --timeout %v: must be positive", *timeout)
		}
		if set["timeout"] && *file != "" {
			e.warnf("--timeout is ignored with --file")
		}
		o, err := openOutput(*out, e.stdout, *file)
		if err != nil {
			return err
		}
		var res source.SIXResult
		if *file != "" {
			r, closeIn, err := openInput(e, *file)
			if err != nil {
				o.Discard()
				return err
			}
			// the file was not selected by the query: apply the filters
			res, err = source.ParseSIX(r, &q)
			closeIn()
			if err != nil {
				o.Discard()
				return fmt.Errorf("%s: %w", *file, err)
			}
		} else {
			c := newSourceClient()
			c.UserAgent = userAgent()
			c.Timeout = *timeout
			ctx, cancel := contextBackground()
			defer cancel()
			if res, err = c.FetchSIX(ctx, q); err != nil {
				o.Discard()
				return err
			}
		}
		if len(res.Bonds) == 0 {
			o.Discard()
			why := fmt.Sprintf("no bonds found for %s", q)
			if res.Rows > 0 && res.Rows == res.Filtered {
				why = fmt.Sprintf("no bonds found for %s (%d rows read, none match)", q, res.Rows)
			}
			if n := res.Rows - res.Filtered; n > 0 {
				why = fmt.Sprintf("no usable bonds for %s: all %d matching rows skipped (%s)", q, n, skipText(res))
			}
			return fmt.Errorf("%s", why)
		}
		t := &table{cols: bondFetchCols}
		ratings := map[string]int{}
		priced := 0
		for _, b := range res.Bonds {
			var price, freq any
			if b.HasPrice {
				price = b.PriceClean
				priced++
			}
			if b.Freq != 0 {
				freq = b.Freq
			}
			dc := ""
			if b.HasDayCount {
				dc = b.DayCount.String()
			}
			t.add(formatDate(b.Maturity), b.CouponPct, price, b.ISIN, freq, dc, b.Issuer, b.Rating)
			r := b.Rating
			if r == "" {
				r = "unrated"
			}
			ratings[r]++
		}
		if err := o.finish(writeTable(o.w, t, formatCSV)); err != nil {
			return err
		}
		// warnings and the summary only after the output was written
		for _, why := range res.SkipReasons() {
			e.warnf("skipped %d rows: %s", res.Skipped[why], why)
		}
		e.infof("SIX: %d bonds written (%s), %d rows skipped; by rating: %s", len(res.Bonds), q, res.SkippedTotal(), ratingText(ratings))
		e.infof("prices: %d of %d bonds have a price; these are delayed closing prices from SIX (check the terms of use before redistributing)", priced, len(res.Bonds))
		return nil
	}
}

func skipText(r source.SIXResult) string {
	var s []string
	for _, why := range r.SkipReasons() {
		s = append(s, fmt.Sprintf("%d %s", r.Skipped[why], why))
	}
	return strings.Join(s, "; ")
}

func ratingText(m map[string]int) string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var s []string
	for _, k := range keys {
		s = append(s, fmt.Sprintf("%s %d", k, m[k]))
	}
	return strings.Join(s, ", ")
}
