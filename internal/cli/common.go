package cli

import (
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"time"

	"github.com/konimarti/fixedincome/pkg/term"
)

// defaultCurveFile is the default of --curve (overridden by $FIC_CURVE).
const defaultCurveFile = "./curve.json"

// curveEnv returns the default curve file: $FIC_CURVE, else ./curve.json.
func curveEnv() string {
	if v := os.Getenv(curveEnvVar); v != "" {
		return v
	}
	return defaultCurveFile
}

// curveFlags are the flags that select and shift the curve.
type curveFlags struct {
	path, date string
	spread     float64
}

// register adds --curve, --spread and (if withDate) --date to fs.
func (c *curveFlags) register(fs *flag.FlagSet, withDate bool) {
	fs.StringVar(&c.path, "curve", curveEnv(), "curve JSON file (env FIC_CURVE; - for stdin)")
	fs.Float64Var(&c.spread, "spread", 0, "z-spread in bp ADDED to the spread_bp of the curve file")
	if withDate {
		fs.StringVar(&c.date, "date", "", "valuation date YYYY-MM-DD or DD.MM.YYYY (default: asof date of the curve file, else today)")
	}
}

// loaded is a curve with its metadata and the valuation date.
type loaded struct {
	// curve includes the spread_bp of the file and --spread
	curve term.Curve
	// base is the curve without the --spread flag (file spread included)
	base   term.Curve
	meta   term.Meta
	settle time.Time
	path   string
}

// readCurveFile reads and parses a curve file.
func readCurveFile(e *env, path string) (term.Curve, term.Meta, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(e.stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, term.Meta{}, fmt.Errorf("reading curve: %w (create a curve file with \"fic curve template\")", err)
	}
	c, meta, err := term.Parse(data)
	if err != nil {
		return nil, term.Meta{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, meta, nil
}

// load reads the curve, adds the --spread and determines the valuation
// date: --date, else the asof date of the curve file, else today. A
// differing --date and asof date give a warning.
func (c *curveFlags) load(e *env) (*loaded, error) {
	if math.IsNaN(c.spread) || math.IsInf(c.spread, 0) {
		return nil, usagef("invalid --spread %v", c.spread)
	}
	curve, meta, err := readCurveFile(e, c.path)
	if err != nil {
		return nil, err
	}
	l := &loaded{base: curve, meta: meta, path: c.path}
	l.curve = term.WithSpread(curve, fromBP(c.spread))
	for _, t := range []float64{1, 10, 50} {
		if d := l.curve.Discount(t); !(d > 0) || math.IsInf(d, 0) {
			return nil, fmt.Errorf("curve discount factor underflows at t=%g years (check --spread %g)", t, c.spread)
		}
	}
	switch {
	case c.date != "":
		d, err := parseDate(c.date)
		if err != nil {
			return nil, usagef("invalid --date: %v", err)
		}
		l.settle = d
		if !meta.AsOf.IsZero() && !meta.AsOf.Equal(d) {
			e.warnf("--date %s differs from the asof date %s of %s", formatDate(d), formatDate(meta.AsOf), c.path)
		}
	case !meta.AsOf.IsZero():
		l.settle = meta.AsOf
	default:
		l.settle = today()
	}
	return l, nil
}

// formatFlag registers --format and -o.
type formatFlags struct{ format, out string }

func (f *formatFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&f.format, "format", "", "output format text|csv|json (default: text on a terminal, csv otherwise for tables)")
	fs.StringVar(&f.out, "o", "-", "output file (- for stdout)")
}
