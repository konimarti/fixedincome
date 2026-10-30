package source

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Default endpoints and limits of the Client.
const (
	// DefaultBaseSNB is the CSV download of the SNB cube rendopar.
	DefaultBaseSNB = "https://data.snb.ch/api/cube/rendopar/data/csv/en"
	// DefaultBaseECB is the yield curve dataflow of the ECB data portal.
	DefaultBaseECB = "https://data-api.ecb.europa.eu/service/data/YC"
	// DefaultTimeout is the default Client.Timeout.
	DefaultTimeout = 30 * time.Second
	// DefaultMaxBytes is the default Client.MaxBytes (the full SNB history is 1.5 MB).
	DefaultMaxBytes int64 = 64 << 20
	// DefaultUserAgent is the default Client.UserAgent.
	DefaultUserAgent = "fixedincome-source (+https://github.com/konimarti/fixedincome)"
)

// ECB issuer sets for ECBOptions.Issuers.
const (
	// IssuersAAA is the curve of the AAA rated euro area central
	// government bonds (instrument G_N_A), the default.
	IssuersAAA = "aaa"
	// IssuersAll is the curve of all euro area central government bonds
	// (instrument G_N_C).
	IssuersAll = "all"
)

// ECBOptions are the options of Client.FetchECB.
type ECBOptions struct {
	// Issuers is IssuersAAA (default, also if empty) or IssuersAll.
	Issuers string
}

// Client downloads the NSS parameters. The zero value works with the
// default endpoints, a 30 s timeout and a 64 MiB size limit.
//
// The Client does not retry and does not honour Retry-After. The proxies of
// the environment (HTTP_PROXY, HTTPS_PROXY, NO_PROXY) are used when HTTP is
// nil or has the default transport. Redirects to other than https URLs are
// refused; redirects to other hosts are followed (at most 5).
type Client struct {
	// HTTP is the http client (default: a new http.Client). It is copied;
	// its CheckRedirect is replaced. Compressed responses are decoded by
	// net/http as long as the transport's DisableCompression is false.
	HTTP *http.Client
	// BaseSNB and BaseECB are the endpoints (defaults DefaultBaseSNB and
	// DefaultBaseECB); overridable for tests.
	BaseSNB, BaseECB string
	// BaseSIX is the SIX reference data endpoint (default DefaultBaseSIX).
	BaseSIX string
	// UserAgent is sent with each request (default DefaultUserAgent).
	UserAgent string
	// MaxBytes limits the size of a response body (default 64 MiB); a larger
	// body is an error.
	MaxBytes int64
	// Timeout is the limit of a whole fetch including reading the body
	// (default 30 s; negative: none besides the context).
	Timeout time.Duration
	// Now returns the current time, used to choose the download window of
	// "latest" (default time.Now).
	Now func() time.Time
}

// latestWindow is how far back the download for "latest" reaches before the
// client falls back to the full history (SNB) .
const latestWindow = 60 * 24 * time.Hour

// dateWindow is how many days before a requested date are downloaded so that
// the error for a missing date can name the nearest earlier date.
const dateWindow = 14

func (c *Client) base(def, v string) string {
	if v == "" {
		return def
	}
	return strings.TrimRight(v, "/")
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// FetchSNB downloads and parses the SNB parameters of the date asof
// (zero: the latest complete date). For a date the download is restricted to
// the 14 days up to asof (query parameters fromDate and toDate); for the
// latest date to the last 60 days, and if that holds no complete date (the
// cube may no longer be updated) the full history is downloaded.
func (c *Client) FetchSNB(ctx context.Context, asof time.Time) (Curve, error) {
	base := c.base(DefaultBaseSNB, c.BaseSNB)
	if asof.IsZero() {
		from := c.now().Add(-latestWindow).Format(dateLayout)
		cv, err := c.fetchSNB(ctx, base+"?"+url.Values{"fromDate": {from}}.Encode(), asof)
		if err == nil {
			return cv, nil
		}
		var pe *parseError
		if !errors.As(err, &pe) {
			return Curve{}, err
		}
		return c.fetchSNB(ctx, base, asof)
	}
	q := url.Values{
		"fromDate": {asof.AddDate(0, 0, -dateWindow).Format(dateLayout)},
		"toDate":   {asof.Format(dateLayout)},
	}
	return c.fetchSNB(ctx, base+"?"+q.Encode(), asof)
}

const dateLayout = "2006-01-02"

// parseError marks an error of the parser (as opposed to the transport).
type parseError struct{ err error }

func (e *parseError) Error() string { return e.err.Error() }
func (e *parseError) Unwrap() error { return e.err }

func (c *Client) fetchSNB(ctx context.Context, u string, asof time.Time) (Curve, error) {
	body, err := c.get(ctx, u)
	if err != nil {
		return Curve{}, err
	}
	cv, err := ParseSNB(bytes.NewReader(body), asof)
	if err != nil {
		var de *DateError
		if errors.As(err, &de) && !asof.IsZero() {
			err = fmt.Errorf("%s; no complete date in the %d days up to %s (downloaded window), earlier data may exist (try --date latest)",
				err, dateWindow, asof.Format(dateLayout))
		}
		return Curve{}, &parseError{fmt.Errorf("%s: %w", shortURL(u), err)}
	}
	return cv, nil
}

// FetchECB downloads and parses the ECB euro area yield curve parameters
// (Svensson model, continuous compounding) of the date asof (zero: the
// latest complete date, taken from the last five observations of each
// parameter). For a date the 14 days up to asof are requested; the ECB
// publishes on TARGET business days, so other dates are an error that names
// the nearest earlier date.
func (c *Client) FetchECB(ctx context.Context, asof time.Time, o ECBOptions) (Curve, error) {
	inst := "G_N_A"
	switch strings.ToLower(o.Issuers) {
	case "", IssuersAAA:
	case IssuersAll:
		inst = "G_N_C"
	default:
		return Curve{}, fmt.Errorf("ECB: invalid issuers %q (%s or %s)", o.Issuers, IssuersAAA, IssuersAll)
	}
	q := url.Values{"format": {"csvdata"}}
	if asof.IsZero() {
		q.Set("lastNObservations", "5")
	} else {
		q.Set("startPeriod", asof.AddDate(0, 0, -dateWindow).Format(dateLayout))
		q.Set("endPeriod", asof.Format(dateLayout))
	}
	u := c.base(DefaultBaseECB, c.BaseECB) + "/B.U2.EUR.4F." + inst + ".SV_C_YM.BETA0+BETA1+BETA2+BETA3+TAU1+TAU2?" + q.Encode()
	period := "the last 5 observations"
	if !asof.IsZero() {
		period = q.Get("startPeriod") + " to " + q.Get("endPeriod")
	}
	body, err := c.get(ctx, u)
	var se *statusError
	if err != nil {
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return Curve{}, fmt.Errorf("ECB returned no data for %s (%v)", period, err)
		}
		return Curve{}, err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return Curve{}, fmt.Errorf("ECB returned no data for %s (%s)", period, shortURL(u))
	}
	cv, err := ParseECB(bytes.NewReader(body), asof)
	if err != nil {
		var de *DateError
		if errors.As(err, &de) && !asof.IsZero() {
			err = fmt.Errorf("%s; no complete date in the %d days up to %s (downloaded window)", err, dateWindow, asof.Format(dateLayout))
		}
		return Curve{}, fmt.Errorf("%s: %w", shortURL(u), err)
	}
	return cv, nil
}

// statusError is the error for a status other than 200.
type statusError struct {
	Code int
	msg  string
}

func (e *statusError) Error() string { return e.msg }

// shortURL returns scheme, host and path of u (no query).
func shortURL(u string) string {
	p, err := url.Parse(u)
	if err != nil {
		return "(invalid URL)"
	}
	return p.Scheme + "://" + p.Host + p.Path
}

// get downloads u and returns the body (status 200 only).
func (c *Client) get(ctx context.Context, u string) ([]byte, error) {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	max := c.MaxBytes
	if max <= 0 {
		max = DefaultMaxBytes
	}
	ua := c.UserAgent
	if ua == "" {
		ua = DefaultUserAgent
	}
	var hc http.Client
	if c.HTTP != nil {
		hc = *c.HTTP
	}
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("stopped after 5 redirects")
		}
		if req.URL.Scheme != "https" {
			return fmt.Errorf("refusing redirect to %s (only https)", req.URL.Redacted())
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", shortURL(u), err)
	}
	u = shortURL(u) // user-facing errors: host and path only
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "text/csv, */*;q=0.1")
	resp, err := hc.Do(req)
	if err != nil {
		if ctx.Err() != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("GET %s: timeout after %v", u, timeout)
		}
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("GET %s: %w", u, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		msg := strings.Join(strings.Fields(strings.ToValidUTF8(string(b), "?")), " ")
		if msg != "" {
			msg = ": " + msg
		}
		return nil, &statusError{Code: resp.StatusCode, msg: fmt.Sprintf("GET %s: HTTP %d%s", u, resp.StatusCode, msg)}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("GET %s: timeout after %v while reading the response", u, timeout)
		}
		return nil, fmt.Errorf("GET %s: reading the response: %w", u, err)
	}
	if int64(len(body)) > max {
		return nil, fmt.Errorf("GET %s: response larger than %d bytes", u, max)
	}
	return body, nil
}
