package term

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// DateFormat is the layout of the "asof" date in curve files.
const DateFormat = "2006-01-02"

// Meta is the metadata of a curve file.
type Meta struct {
	// AsOf is the curve date (time t = 0); zero if unknown
	AsOf time.Time
	// Currency is the currency of the curve, e.g. "CHF"
	Currency string
	// Source describes the origin of the curve, e.g. "SNB"
	Source string
}

// file is the JSON schema of a curve file. Rates and betas are in PERCENT,
// the spread in BASIS POINTS, maturities and decay parameters in years.
type file struct {
	Version         int                `json:"version,omitempty"`
	Comment         string             `json:"comment,omitempty"`
	Model           string             `json:"model"`
	AsOf            string             `json:"asof,omitempty"`
	Currency        string             `json:"currency,omitempty"`
	Source          string             `json:"source,omitempty"`
	Params          map[string]float64 `json:"params,omitempty"`
	Knots           []float64          `json:"knots,omitempty"`
	DiscountFactors []float64          `json:"discount_factors,omitempty"`
	SpreadBP        float64            `json:"spread_bp"`
}

// Model names of the curve file schema.
const (
	ModelNSS    = "nss"
	ModelSpline = "spline"
	ModelFlat   = "flat"
)

// Parse returns the curve and its metadata from the JSON curve file data.
// The z-spread of the file is applied with WithSpread (use Spread to unwrap
// it). Every call returns a new, independent curve.
//
// The schema has an explicit model discriminator; rates and betas are in
// percent, the spread in basis points (I/O units), maturities in years:
//
//	{"model":"nss","asof":"2021-08-31","currency":"CHF","source":"SNB",
//	 "params":{"b0":..,"b1":..,"b2":..,"b3":..,"t1":..,"t2":..},"spread_bp":0}
//	{"model":"spline","asof":"2021-08-31","knots":[...],"discount_factors":[...],"spread_bp":0}
//	{"model":"flat","params":{"r":2.0},"spread_bp":0}
//
// The optional "version" must be 1 (FileVersion) and the optional "comment"
// is ignored, as are keys prefixed "x-" (extensions); other unknown fields
// are rejected. Model names are case-insensitive. Files without a "model"
// field are parsed in
// the legacy format, in which the model is determined by its keys (in this
// order): Nelson-Siegel-Svensson ("b0".."b3", "t1", "t2" in percent/years),
// spline ("maturities", "discountfactors"; maturities may be unsorted) and
// flat ("r" in percent); the optional "spread" is in basis points.
func Parse(data []byte) (Curve, Meta, error) {
	raw := make(map[string]json.RawMessage)
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, Meta{}, fmt.Errorf("term: invalid curve file: %w", err)
	}
	if _, ok := raw["model"]; !ok {
		c, err := parseLegacy(raw)
		return c, Meta{}, err
	}

	for key := range raw {
		if !knownKeys[key] && !strings.HasPrefix(key, "x-") {
			return nil, Meta{}, fmt.Errorf("term: invalid curve file: unknown field %q", key)
		}
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, Meta{}, fmt.Errorf("term: invalid curve file: %w", err)
	}
	if _, ok := raw["version"]; ok && f.Version != FileVersion {
		return nil, Meta{}, fmt.Errorf("term: unsupported curve file version %d (supported: %d)", f.Version, FileVersion)
	}
	f.Model = strings.ToLower(strings.TrimSpace(f.Model))
	var meta Meta
	if f.AsOf != "" {
		d, err := time.Parse(DateFormat, f.AsOf)
		if err != nil {
			return nil, Meta{}, fmt.Errorf("term: invalid asof date: %w", err)
		}
		meta.AsOf = d
	}
	meta.Currency, meta.Source = f.Currency, f.Source

	var c Curve
	switch f.Model {
	case ModelNSS:
		p, err := params(f.Params, "b0", "b1", "b2", "b3", "t1", "t2")
		if err != nil {
			return nil, Meta{}, err
		}
		n, err := NewNSS(p[0]/100, p[1]/100, p[2]/100, p[3]/100, p[4], p[5])
		if err != nil {
			return nil, Meta{}, err
		}
		c = n
	case ModelFlat:
		p, err := params(f.Params, "r")
		if err != nil {
			return nil, Meta{}, err
		}
		c = Flat{Rate: p[0] / 100}
	case ModelSpline:
		if len(f.Params) > 0 {
			return nil, Meta{}, fmt.Errorf("term: spline curve does not take params")
		}
		s, err := NewSpline(f.Knots, f.DiscountFactors)
		if err != nil {
			return nil, Meta{}, err
		}
		c = s
	default:
		return nil, Meta{}, fmt.Errorf("term: unknown curve model %q", f.Model)
	}
	if f.Model != ModelSpline && (len(f.Knots) > 0 || len(f.DiscountFactors) > 0) {
		return nil, Meta{}, fmt.Errorf("term: %s curve does not take knots or discount factors", f.Model)
	}
	if err := finite(f.SpreadBP, "spread_bp"); err != nil {
		return nil, Meta{}, err
	}
	return WithSpread(c, f.SpreadBP/1e4), meta, nil
}

// FileVersion is the version of the curve file schema.
const FileVersion = 1

// knownKeys are the fields of the curve file schema.
var knownKeys = map[string]bool{
	"version": true, "comment": true, "model": true, "asof": true, "currency": true, "source": true,
	"params": true, "knots": true, "discount_factors": true, "spread_bp": true,
}

// params returns the values of the given keys; all keys are required and no
// other keys are allowed.
func params(m map[string]float64, keys ...string) ([]float64, error) {
	values := make([]float64, len(keys))
	for i, key := range keys {
		v, ok := m[key]
		if !ok {
			return nil, fmt.Errorf("term: missing parameter %q", key)
		}
		if err := finite(v, key); err != nil {
			return nil, err
		}
		values[i] = v
	}
	if len(m) != len(keys) {
		for key := range m {
			found := false
			for _, k := range keys {
				found = found || k == key
			}
			if !found {
				return nil, fmt.Errorf("term: unknown parameter %q", key)
			}
		}
	}
	return values, nil
}

func finite(v float64, name string) error {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Errorf("term: %s is not finite", name)
	}
	return nil
}

// parseLegacy parses the legacy key-sniffing format.
func parseLegacy(raw map[string]json.RawMessage) (Curve, error) {
	has := func(keys ...string) bool {
		for _, key := range keys {
			if _, ok := raw[key]; !ok {
				return false
			}
		}
		return true
	}
	number := func(key string) (float64, error) {
		var v float64
		if err := json.Unmarshal(raw[key], &v); err != nil {
			return 0, fmt.Errorf("term: invalid %q: %w", key, err)
		}
		return v, nil
	}
	spread := 0.0
	if has("spread") {
		s, err := number("spread")
		if err != nil {
			return nil, err
		}
		spread = s / 1e4
	}

	switch {
	case has("b0", "b1", "b2", "b3", "t1", "t2"):
		p := make([]float64, 6)
		for i, key := range []string{"b0", "b1", "b2", "b3", "t1", "t2"} {
			v, err := number(key)
			if err != nil {
				return nil, err
			}
			p[i] = v
		}
		n, err := NewNSS(p[0]/100, p[1]/100, p[2]/100, p[3]/100, p[4], p[5])
		if err != nil {
			return nil, err
		}
		return WithSpread(n, spread), nil
	case has("maturities", "discountfactors"):
		var t, z []float64
		if err := json.Unmarshal(raw["maturities"], &t); err != nil {
			return nil, fmt.Errorf("term: invalid \"maturities\": %w", err)
		}
		if err := json.Unmarshal(raw["discountfactors"], &z); err != nil {
			return nil, fmt.Errorf("term: invalid \"discountfactors\": %w", err)
		}
		if len(t) != len(z) {
			return nil, fmt.Errorf("term: number of maturities (%d) and discount factors (%d) differ", len(t), len(z))
		}
		// the legacy format allows unsorted maturities
		idx := make([]int, len(t))
		for i := range idx {
			idx[i] = i
		}
		sort.SliceStable(idx, func(a, b int) bool { return t[idx[a]] < t[idx[b]] })
		ts, zs := make([]float64, len(t)), make([]float64, len(t))
		for i, j := range idx {
			ts[i], zs[i] = t[j], z[j]
		}
		s, err := NewSpline(ts, zs)
		if err != nil {
			return nil, err
		}
		return WithSpread(s, spread), nil
	case has("r"):
		r, err := number("r")
		if err != nil {
			return nil, err
		}
		return WithSpread(Flat{Rate: r / 100}, spread), nil
	}
	return nil, fmt.Errorf("term: parsing into yield curve failed: unknown curve format")
}

// Marshal returns the curve file (JSON, indented) of the curve c with the
// metadata meta in the schema described at Parse. A z-spread applied with
// WithSpread is written as "spread_bp". Supported are Flat, NSS and *Spline
// curves (with or without spread); the curve is validated (an invalid NSS,
// a non-finite rate or spread or a zero-value Spline is an error).
func Marshal(c Curve, meta Meta) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("term: cannot marshal a nil curve")
	}
	base, s := Spread(c)
	if err := finite(s, "spread"); err != nil {
		return nil, err
	}
	f := file{Version: FileVersion, SpreadBP: s * 1e4, Currency: meta.Currency, Source: meta.Source}
	if !meta.AsOf.IsZero() {
		f.AsOf = meta.AsOf.Format(DateFormat)
	}
	switch b := base.(type) {
	case NSS:
		if err := b.Validate(); err != nil {
			return nil, err
		}
		f.Model = ModelNSS
		f.Params = map[string]float64{
			"b0": b.B0 * 100, "b1": b.B1 * 100, "b2": b.B2 * 100, "b3": b.B3 * 100,
			"t1": b.Tau1, "t2": b.Tau2,
		}
	case *NSS:
		return Marshal(WithSpread(*b, s), meta)
	case Flat:
		if err := finite(b.Rate, "rate"); err != nil {
			return nil, err
		}
		f.Model = ModelFlat
		f.Params = map[string]float64{"r": b.Rate * 100}
	case *Flat:
		return Marshal(WithSpread(*b, s), meta)
	case *Spline:
		if !b.ok() {
			return nil, fmt.Errorf("term: cannot marshal an uninitialised spline (use NewSpline)")
		}
		f.Model = ModelSpline
		f.Knots = b.Knots()
		f.DiscountFactors = b.DiscountFactors()
	default:
		return nil, fmt.Errorf("term: cannot marshal curve of type %T", base)
	}
	return json.MarshalIndent(f, "", "  ")
}
