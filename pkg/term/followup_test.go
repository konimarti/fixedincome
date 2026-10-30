package term_test

import (
	"math"
	"testing"

	"github.com/konimarti/fixedincome/pkg/term"
)

// TestNSS_InvalidNaN checks that invalid parameters give NaN (as documented)
func TestNSS_InvalidNaN(t *testing.T) {
	for _, n := range []term.NSS{
		{B0: 0.01, Tau1: 0, Tau2: 1},
		{B0: 0.01, Tau1: 1, Tau2: -1},
		{B0: 0.01, Tau1: math.Inf(1), Tau2: 1},
		{B0: math.Inf(1), Tau1: 1, Tau2: 1},
		{B0: math.NaN(), Tau1: 1, Tau2: 1},
		{},
	} {
		for _, x := range []float64{0, 1, 10} {
			if r, z, f := n.ZeroRate(x), n.Discount(x), n.InstForward(x); !math.IsNaN(r) || !math.IsNaN(z) || !math.IsNaN(f) {
				t.Errorf("%+v at %v: got %v %v %v, expected NaN", n, x, r, z, f)
			}
		}
		if _, err := term.Marshal(n, term.Meta{}); err == nil {
			t.Errorf("%+v: Marshal must validate", n)
		}
	}
}

// TestNSS_Gradient compares the analytic gradients with central differences
func TestNSS_Gradient(t *testing.T) {
	n := snb20210331
	p := n.Params()
	grad := make([]float64, term.NumParams)
	dgrad := make([]float64, term.NumParams)
	for _, x := range []float64{0, 1e-5, 0.003, 0.5, 2, 10, 30} {
		n.ZeroRateGrad(x, grad)
		n.DiscountGrad(x, dgrad)
		for i := range p {
			h := 1e-6 * math.Max(1, math.Abs(p[i]))
			up, dn := append([]float64{}, p...), append([]float64{}, p...)
			up[i] += h
			dn[i] -= h
			cu, err := n.WithParams(up)
			if err != nil {
				t.Fatal(err)
			}
			cd, _ := n.WithParams(dn)
			fd := (cu.ZeroRate(x) - cd.ZeroRate(x)) / (2 * h)
			if math.Abs(grad[i]-fd) > 1e-7 {
				t.Errorf("ZeroRateGrad(%v)[%d] = %v, finite difference %v", x, i, grad[i], fd)
			}
			fdz := (cu.Discount(x) - cd.Discount(x)) / (2 * h)
			if math.Abs(dgrad[i]-fdz) > 1e-7 {
				t.Errorf("DiscountGrad(%v)[%d] = %v, finite difference %v", x, i, dgrad[i], fdz)
			}
		}
	}
	if _, err := n.WithParams([]float64{1, 2}); err == nil {
		t.Errorf("expected error for wrong number of parameters")
	}
	if _, err := n.WithParams([]float64{0, 0, 0, 0, 0, 1}); err == nil {
		t.Errorf("expected error for tau = 0")
	}
	var _ term.Parametric = n
}

func TestDiscountGrid(t *testing.T) {
	ts := []float64{0, 0.5, 1, 10}
	out := make([]float64, len(ts))
	term.DiscountGrid(snb20210331, ts, out)
	for i, x := range ts {
		if out[i] != snb20210331.Discount(x) {
			t.Errorf("DiscountGrid[%d]", i)
		}
	}
}

func TestSpline_ZeroValue(t *testing.T) {
	var nilSpline *term.Spline
	for _, s := range []*term.Spline{{}, nilSpline} {
		if !math.IsNaN(s.Discount(1)) || !math.IsNaN(s.ZeroRate(1)) || !math.IsNaN(s.InstForward(1)) || s.Knots() != nil {
			t.Errorf("zero-value/nil spline must return NaN")
		}
		if _, err := term.Marshal(s, term.Meta{}); err == nil {
			t.Errorf("Marshal of a zero-value spline must fail")
		}
	}
}

func TestSpline_MinSpacing(t *testing.T) {
	if _, err := term.NewSpline([]float64{1, 1 + 1e-9}, []float64{0.99, 0.99}); err == nil {
		t.Errorf("expected error for knots closer than 1e-8")
	}
	if _, err := term.NewSpline([]float64{1e-9, 1}, []float64{1, 0.99}); err == nil {
		t.Errorf("expected error for a knot closer than 1e-8 to the anchor")
	}
	if _, err := term.NewSpline([]float64{1, 1 + 1e-7}, []float64{0.99, 0.99}); err != nil {
		t.Errorf("unexpected error %v", err)
	}
}

func TestWithSpread_EdgeCases(t *testing.T) {
	c := term.WithSpread(term.WithSpread(snb20210331, 0.1+0.2), -0.3)
	if _, ok := c.(term.NSS); !ok {
		t.Errorf("rounding residual not snapped to the base curve: %T", c)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Errorf("expected panic for nil curve")
			}
		}()
		term.WithSpread(nil, 0.01)
	}()
}

func TestParse_Extensions(t *testing.T) {
	for _, data := range []string{
		`{"version":1,"comment":"SNB 2021-03-31","model":"NSS","params":{"b0":-0.266372,"b1":-0.471343,"b2":5.68789,"b3":-5.12324,"t1":5.74881,"t2":4.14426},"x-fitted-by":"termfit","spread_bp":0}`,
		`{"model":" Nss ","params":{"b0":-0.266372,"b1":-0.471343,"b2":5.68789,"b3":-5.12324,"t1":5.74881,"t2":4.14426}}`,
	} {
		c, _, err := term.Parse([]byte(data))
		if err != nil {
			t.Errorf("%s: %v", data, err)
			continue
		}
		if math.Abs(c.ZeroRate(5)-snb20210331.ZeroRate(5)) > 1e-15 {
			t.Errorf("%s: wrong curve", data)
		}
	}
	for _, data := range []string{
		`{"version":2,"model":"flat","params":{"r":1}}`,
		`{"model":"flat","params":{"r":1},"y-ext":1}`,
	} {
		if _, _, err := term.Parse([]byte(data)); err == nil {
			t.Errorf("expected error for %s", data)
		}
	}
	data, err := term.Marshal(term.Flat{Rate: 0.01}, term.Meta{})
	if err != nil {
		t.Fatal(err)
	}
	if string(data[:20]) != "{\n  \"version\": 1,\n  " {
		t.Errorf("Marshal does not write the version: %s", data)
	}
	if _, err := term.Marshal(term.Flat{Rate: math.NaN()}, term.Meta{}); err == nil {
		t.Errorf("expected error for NaN rate")
	}
}
