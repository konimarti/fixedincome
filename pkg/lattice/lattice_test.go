package lattice_test

import (
	"math"
	"strings"
	"testing"

	"github.com/konimarti/fixedincome/pkg/lattice"
	"github.com/konimarti/fixedincome/pkg/term"
)

var nss = term.NSS{B0: -0.00266372, B1: -0.00471343, B2: 0.0568789, B3: -0.0512324, Tau1: 5.74881, Tau2: 4.14426}

func normCDF(x float64) float64 { return 0.5 * math.Erfc(-x/math.Sqrt2) }

// hwCall returns the Hull-White price of a call with expiry T and strike k
// on the zero bond Z(T,S).
func hwCall(c term.Curve, a, sigma, T, S, k float64) float64 {
	sp := sigma * math.Sqrt((1-math.Exp(-2*a*T))/(2*a)) * (1 - math.Exp(-a*(S-T))) / a
	zT, zS := c.Discount(T), c.Discount(S)
	h := math.Log(zS/(k*zT))/sp + sp/2
	return zS*normCDF(h) - k*zT*normCDF(h-sp)
}

func TestTree_ZeroBondRepricing(t *testing.T) {
	curves := map[string]term.Curve{"flat": term.Flat{Rate: 0.03}, "nss": nss}
	for name, c := range curves {
		for _, p := range []struct{ a, sigma float64 }{{0, 0.01}, {0.001, 0.01}, {0.05, 0.01}, {0.1, 0.02}, {0.5, 0.005}, {2, 0.01}} {
			tr, err := lattice.New(c, p.a, p.sigma, 10, lattice.WithTimes(0.5, 1.3, 3.7, 7.9))
			if err != nil {
				t.Fatalf("%s %+v: %v", name, p, err)
			}
			for _, T := range []float64{0.5, 1.3, 3.7, 7.9, 10} {
				v, err := tr.Value([]lattice.Flow{{T: T, Amount: 1}}, nil, 0)
				if err != nil {
					t.Fatal(err)
				}
				if want := c.Discount(T); math.Abs(v/want-1) > 1e-9 {
					t.Errorf("%s %+v T=%v: got %.12f, want %.12f", name, p, T, v, want)
				}
			}
			// spread
			v, _ := tr.Value([]lattice.Flow{{T: 7.9, Amount: 1}}, nil, 0.01)
			if want := c.Discount(7.9) * math.Exp(-0.079); math.Abs(v/want-1) > 1e-9 {
				t.Errorf("%s %+v spread: got %v, want %v", name, p, v, want)
			}
		}
	}
}

func TestTree_EuropeanCall(t *testing.T) {
	// Z(0,S) - call on Z(T,S): an issuer-callable zero bond
	T, S := 2.0, 5.0
	for _, p := range []struct{ a, sigma float64 }{{0.05, 0.01}, {0.1, 0.015}, {0.3, 0.02}} {
		for _, strike := range []float64{0.8, 0.9, 0.95} {
			want := nss.Discount(S) - hwCall(nss, p.a, p.sigma, T, S, strike)
			for _, spy := range []int{25, 50, 100, 200} {
				tr, err := lattice.New(nss, p.a, p.sigma, S, lattice.WithTimes(T), lattice.WithStepsPerYear(spy))
				if err != nil {
					t.Fatal(err)
				}
				got, err := tr.Value([]lattice.Flow{{T: S, Amount: 1}}, []lattice.Exercise{{T: T, Strike: strike, Side: lattice.Call}}, 0)
				if err != nil {
					t.Fatal(err)
				}
				e := math.Abs(got - want)
				t.Logf("a=%v sigma=%v K=%v spy=%d: got %.8f want %.8f err %.2e", p.a, p.sigma, strike, spy, got, want, e)
				if spy >= 50 && e > 1e-3 {
					t.Errorf("a=%v sigma=%v K=%v spy=%d: error %v", p.a, p.sigma, strike, spy, e)
				}
				if spy == 200 && e > 1e-4 {
					t.Errorf("a=%v sigma=%v K=%v: not converged, error %v", p.a, p.sigma, strike, e)
				}
			}
			// put-call parity: Z(0,S) + put = call + K Z(0,T)
			tr, _ := lattice.New(nss, p.a, p.sigma, S, lattice.WithTimes(T), lattice.WithStepsPerYear(100))
			put, _ := tr.Value([]lattice.Flow{{T: S, Amount: 1}}, []lattice.Exercise{{T: T, Strike: strike, Side: lattice.Put}}, 0)
			call, _ := tr.Value([]lattice.Flow{{T: S, Amount: 1}}, []lattice.Exercise{{T: T, Strike: strike, Side: lattice.Call}}, 0)
			// max(Z,K) + min(Z,K) = Z + K
			if d := put + call - nss.Discount(S) - strike*nss.Discount(T); math.Abs(d) > 1e-9 {
				t.Errorf("put-call parity violated by %v", d)
			}
		}
	}
}

func TestTree_FlowAndExerciseSameTime(t *testing.T) {
	// coupon 5 at T and call at 100: value = 5 Z(T) + min(100 Z(T,S), 100) in
	// expectation; with a huge strike the call is never exercised
	tr, err := lattice.New(term.Flat{Rate: 0.02}, 0.1, 0.01, 3, lattice.WithTimes(1))
	if err != nil {
		t.Fatal(err)
	}
	flows := []lattice.Flow{{T: 1, Amount: 5}, {T: 3, Amount: 100}}
	v0, _ := tr.Value(flows, nil, 0)
	v1, _ := tr.Value(flows, []lattice.Exercise{{T: 1, Strike: 1e9, Side: lattice.Call}}, 0)
	v2, _ := tr.Value(flows, []lattice.Exercise{{T: 1, Strike: 0, Side: lattice.Call}}, 0)
	if math.Abs(v0-v1) > 1e-12 {
		t.Errorf("never exercised call changes value: %v vs %v", v0, v1)
	}
	// strike 0: only the coupon at the call date remains
	if want := 5 * math.Exp(-0.02); math.Abs(v2-want) > 1e-9 {
		t.Errorf("got %v, want %v (coupon paid on the call date)", v2, want)
	}
	// exercise at time 0
	v3, err := tr.Value(flows, []lattice.Exercise{{T: 0, Strike: 50, Side: lattice.Call}}, 0)
	if err != nil || math.Abs(v3-50) > 1e-12 {
		t.Errorf("exercise at 0: got %v, %v", v3, err)
	}
}

func TestTree_Errors(t *testing.T) {
	c := term.Flat{Rate: 0.02}
	bad := []struct {
		name         string
		c            term.Curve
		a, sigma, hz float64
		opts         []lattice.Option
	}{
		{"nil curve", nil, 0.1, 0.01, 1, nil},
		{"negative a", c, -0.1, 0.01, 1, nil},
		{"nan a", c, math.NaN(), 0.01, 1, nil},
		{"zero sigma", c, 0.1, 0, 1, nil},
		{"zero horizon", c, 0.1, 0.01, 0, nil},
		{"event beyond horizon", c, 0.1, 0.01, 1, []lattice.Option{lattice.WithTimes(2)}},
		{"negative event", c, 0.1, 0.01, 1, []lattice.Option{lattice.WithTimes(-1)}},
		{"steps", c, 0.1, 0.01, 1, []lattice.Option{lattice.WithStepsPerYear(0)}},
		{"min steps", c, 0.1, 0.01, 1, []lattice.Option{lattice.WithMinSteps(0)}},
	}
	for _, b := range bad {
		if _, err := lattice.New(b.c, b.a, b.sigma, b.hz, b.opts...); err == nil {
			t.Errorf("%s: expected error", b.name)
		}
	}
	tr, err := lattice.New(c, 0.1, 0.01, 1, lattice.WithTimes(0.5))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Value([]lattice.Flow{{T: 0.3333, Amount: 1}}, nil, 0); err == nil {
		t.Error("flow off the grid: expected error")
	}
	if _, err := tr.Value(nil, []lattice.Exercise{{T: 0.5, Strike: -1}}, 0); err == nil {
		t.Error("negative strike: expected error")
	}
	if _, err := tr.Value(nil, []lattice.Exercise{{T: 0.5, Strike: 1}, {T: 0.5, Strike: 2}}, 0); err == nil {
		t.Error("duplicate exercise: expected error")
	}
	if _, err := tr.Value(nil, nil, math.NaN()); err == nil {
		t.Error("nan spread: expected error")
	}
	if got := tr.Horizon(); got != 1 {
		t.Errorf("horizon %v", got)
	}
}

func TestTree_TinySigma(t *testing.T) {
	for _, a := range []float64{0, 0.1} {
		tr, err := lattice.New(term.Flat{Rate: 0.02}, a, 1e-170, 3, lattice.WithTimes(1))
		if err != nil {
			t.Fatal(err)
		}
		v, err := tr.Value([]lattice.Flow{{T: 3, Amount: 1}}, nil, 0)
		if want := math.Exp(-0.06); err != nil || math.IsNaN(v) || math.Abs(v/want-1) > 1e-9 {
			t.Errorf("a=%v: got %v, %v", a, v, err)
		}
	}
}

func TestTree_LargeMeanReversion(t *testing.T) {
	// either a valid tree or an error that says what to do, never garbage
	tr, err := lattice.New(term.Flat{Rate: 0.02}, 15, 0.01, 2, lattice.WithTimes(0.333))
	if err != nil {
		if !strings.Contains(err.Error(), "raise the steps per year") {
			t.Errorf("unhelpful error: %v", err)
		}
		return
	}
	v, err := tr.Value([]lattice.Flow{{T: 0.333, Amount: 1}}, nil, 0)
	if want := math.Exp(-0.02 * 0.333); err != nil || math.Abs(v/want-1) > 1e-9 {
		t.Errorf("got %v, %v", v, err)
	}
	// more steps per year always work
	if _, err := lattice.New(term.Flat{Rate: 0.02}, 15, 0.01, 2, lattice.WithTimes(0.333), lattice.WithStepsPerYear(500)); err != nil {
		t.Error(err)
	}
}

func TestTree_SizeLimit(t *testing.T) {
	_, err := lattice.New(term.Flat{Rate: 0.02}, 0, 0.01, 2000)
	if err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Errorf("expected size limit error, got %v", err)
	}
}
