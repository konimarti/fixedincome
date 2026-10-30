package bond

import (
	"errors"
	"fmt"
	"math"
)

// ErrNoFixing is returned if the index rate of the current coupon period of
// a floating-rate note is required but not set.
var ErrNoFixing = errors.New("bond: current fixing required for the coupon period containing the settlement date")

// DefaultFace is the face value set by NewStraight and NewFloating unless
// WithFace is given.
const DefaultFace = 100.0

// Option is an option of NewStraight and NewFloating.
type Option func(*options) error

type options struct {
	face       float64
	redemption *float64
	fixing     *float64
}

// WithFace sets the face value (notional, > 0). Default: DefaultFace.
func WithFace(face float64) Option {
	return func(o *options) error {
		o.face = face
		return nil
	}
}

// WithRedemption sets the amount repaid at maturity (currency units, >= 0;
// 0 for a pure coupon stream). Default: the face value.
func WithRedemption(redemption float64) Option {
	return func(o *options) error {
		o.redemption = &redemption
		return nil
	}
}

// WithFixing sets the current fixing of a floating-rate note (decimal); see
// Floating.CurrentFixing. Not applicable to straight bonds.
func WithFixing(rate float64) Option {
	return func(o *options) error {
		o.fixing = &rate
		return nil
	}
}

func applyOptions(opts []Option) (options, error) {
	o := options{face: DefaultFace}
	for _, opt := range opts {
		if err := opt(&o); err != nil {
			return o, err
		}
	}
	if o.redemption == nil {
		r := o.face
		o.redemption = &r
	}
	return o, nil
}

// param is a named parameter for validation in a deterministic order.
type param struct {
	name  string
	value float64
}

// checkFinite returns an error for the first non-finite parameter.
func checkFinite(params ...param) error {
	for _, p := range params {
		if math.IsNaN(p.value) || math.IsInf(p.value, 0) {
			return fmt.Errorf("bond: %s is not finite: %v", p.name, p.value)
		}
	}
	return nil
}

// checkAmounts validates face value and redemption.
func checkAmounts(face, redemption float64) error {
	if !(face > 0) {
		return fmt.Errorf("bond: face value %v must be positive", face)
	}
	if redemption < 0 {
		return fmt.Errorf("bond: negative redemption %v", redemption)
	}
	return nil
}
