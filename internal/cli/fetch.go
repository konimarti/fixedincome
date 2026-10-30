package cli

import (
	"context"
	"flag"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/konimarti/fixedincome/pkg/source"
	"github.com/konimarti/fixedincome/pkg/term"
)

// newSourceClient creates the client for "curve fetch" (replaced in tests to
// point to a test server).
var newSourceClient = func() *source.Client { return &source.Client{} }

// userAgent is the User-Agent of the downloads.
func userAgent() string {
	return "fic/" + Version + " (+https://github.com/konimarti/fixedincome)"
}

// maxFetchSpreadBP bounds --spread of curve fetch.
const maxFetchSpreadBP = 100000

func buildCurveFetch(e *env) (*flag.FlagSet, func() error) {
	fs := flag.NewFlagSet("curve fetch", flag.ContinueOnError)
	src := fs.String("source", "", "data source snb|ecb (required)")
	date := fs.String("date", "latest", "as-of date YYYY-MM-DD or DD.MM.YYYY, or latest (the latest complete date)")
	file := fs.String("file", "", "parse this downloaded file instead of the network (- for stdin)")
	issuers := fs.String("ecb-issuers", "aaa", "ecb: aaa (AAA rated government bonds) or all (all euro area government bonds)")
	maxAge := fs.Int("max-age", 14, "warn (with --strict: fail) if the latest parameters are older than this many days (0: no check)")
	timeout := fs.Duration("timeout", source.DefaultTimeout, "time limit of the download")
	strict := fs.Bool("strict", false, "with --date latest: exit 1 and write nothing if the data is older than --max-age days")
	spread := fs.Float64("spread", 0, "z-spread in bp (-100000..100000) written to the curve file (spread_bp)")
	out := fs.String("o", "-", "output curve JSON file (- for stdout)")
	return fs, func() error {
		set := setFlags(fs)
		name := strings.ToLower(*src)
		switch name {
		case "":
			return usagef("--source is required (snb or ecb)")
		case "snb", "ecb":
		default:
			return usagef("invalid --source %q (snb, ecb)", *src)
		}
		var asof time.Time
		if d := strings.TrimSpace(*date); d != "" && !strings.EqualFold(d, "latest") {
			var err error
			if asof, err = parseDate(d); err != nil {
				return usagef("invalid --date: %v", err)
			}
		}
		iss := strings.ToLower(*issuers)
		if iss != source.IssuersAAA && iss != source.IssuersAll {
			return usagef("invalid --ecb-issuers %q (aaa, all)", *issuers)
		}
		if set["ecb-issuers"] && name != "ecb" {
			return usagef("--ecb-issuers applies to --source ecb only")
		}
		if *maxAge < 0 {
			return usagef("invalid --max-age %d: must be >= 0", *maxAge)
		}
		if *timeout <= 0 {
			return usagef("invalid --timeout %v: must be positive", *timeout)
		}
		if math.IsNaN(*spread) || math.Abs(*spread) > maxFetchSpreadBP {
			return usagef("invalid --spread %v (at most %g bp in absolute value)", *spread, float64(maxFetchSpreadBP))
		}
		if set["timeout"] && *file != "" {
			e.warnf("--timeout is ignored with --file")
		}
		o, err := openOutput(*out, e.stdout, *file)
		if err != nil {
			return err
		}

		var cv source.Curve
		switch {
		case *file != "":
			r, closeIn, err := openInput(e, *file)
			if err != nil {
				o.Discard()
				return err
			}
			if name == "snb" {
				cv, err = source.ParseSNB(r, asof)
			} else {
				cv, err = source.ParseECB(r, asof)
			}
			closeIn()
			if err != nil {
				o.Discard()
				return fmt.Errorf("%s: %w", *file, err)
			}
			if want := map[string]string{source.IssuersAAA: "ECB AAA", source.IssuersAll: "ECB all issuers"}[iss]; name == "ecb" &&
				set["ecb-issuers"] && cv.Meta.Source != "ECB" && cv.Meta.Source != want {
				o.Discard()
				return usagef("--ecb-issuers %s contradicts %s, which holds %q", iss, *file, cv.Meta.Source)
			}
		default:
			c := newSourceClient()
			c.UserAgent = userAgent()
			c.Timeout = *timeout
			if c.Now == nil {
				c.Now = clock
			}
			ctx, cancel := contextBackground()
			defer cancel()
			if name == "snb" {
				cv, err = c.FetchSNB(ctx, asof)
			} else {
				cv, err = c.FetchECB(ctx, asof, source.ECBOptions{Issuers: iss})
			}
			if err != nil {
				o.Discard()
				return err
			}
		}

		age := int(today().Sub(cv.Meta.AsOf).Hours() / 24)
		stale := asof.IsZero() && *maxAge > 0 && age > *maxAge
		if stale && *strict {
			o.Discard()
			return fmt.Errorf("data is stale: latest %s parameters are from %s (%d days old, --max-age %d)",
				strings.ToUpper(name), formatDate(cv.Meta.AsOf), age, *maxAge)
		}
		var c term.Curve = cv.NSS
		c = term.WithSpread(c, fromBP(*spread))
		data, err := term.Marshal(c, cv.Meta)
		if err == nil {
			_, err = o.w.Write(append(data, '\n'))
		}
		if err = o.finish(err); err != nil {
			return err
		}
		// warnings and the summary only after the output was written
		if stale {
			if *file != "" {
				e.warnf("data in %s is from %s (%d days old)", *file, formatDate(cv.Meta.AsOf), age)
			} else {
				e.warnf("latest available %s parameters are from %s; the source may no longer be updated",
					strings.ToUpper(name), formatDate(cv.Meta.AsOf))
			}
		}
		for _, n := range cv.Notes {
			e.warnf("%s", n)
		}
		tag := ""
		if stale {
			tag = fmt.Sprintf(" [STALE: %d days]", age)
		}
		e.infof("%s, as of %s%s, %s: zero rate (cc) 1y %.4f%%, 5y %.4f%%, 10y %.4f%%", cv.Meta.Source,
			formatDate(cv.Meta.AsOf), tag, cv.Meta.Currency, toPct(c.ZeroRate(1)), toPct(c.ZeroRate(5)), toPct(c.ZeroRate(10)))
		return nil
	}
}

// contextBackground returns a cancellable background context.
func contextBackground() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}
