package hullwhite_test

import (
	"math"
	"os"
	"strconv"
	"testing"

	"github.com/konimarti/fixedincome/pkg/mc"
	"github.com/konimarti/fixedincome/pkg/mc/model/hullwhite"
	"github.com/konimarti/fixedincome/pkg/term"
)

var curve = term.NSS{
	B0: 0.03, B1: -0.02, B2: 0.01, B3: 0.005, Tau1: 2, Tau2: 8,
}

// seed adds the offset 1000·$MC_SEED_OFFSET to a test seed (to check the
// robustness of the statistical tests over many seeds).
func seed(s uint64) uint64 {
	off, _ := strconv.Atoi(os.Getenv("MC_SEED_OFFSET"))
	return s + 1000*uint64(off)
}

// maxDev is the tolerance (in standard errors) for checks of all grid
// nodes at once: the standardised deviations of correlated nodes behave
// like a Brownian path, and its maximum over up to 1000 nodes exceeds 4
// far more often than a single node (observed 4.0-4.1 for 2 of 20 seed
// offsets with N = 1000). 4.5 keeps the family-wise false alarm rate
// below about 1%.
const maxDev = 4.5

func mustGrid(t testing.TB, T float64, n int) *mc.Grid {
	t.Helper()
	g, err := mc.NewUniformGrid(T, n)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestHullWhite_Sigma0(t *testing.T) {
	m, err := hullwhite.New(curve, 0.1, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, T := range []float64{1, 10, 30} {
		for _, n := range []int{3, 50, 500} {
			g := mustGrid(t, T, n)
			res, err := mc.PriceMulti(m, mc.ZeroBonds{Grid: g}, g, mc.Options{Paths: 2, Seed: seed(1)})
			if err != nil {
				t.Fatal(err)
			}
			for i, r := range res {
				if z := curve.Discount(g.Times[i]); math.Abs(r.Mean-z) > 1e-12 {
					t.Fatalf("T=%v N=%d node %d: %v vs %v", T, n, i, r.Mean, z)
				}
			}
		}
	}
}

// TestHullWhite_ZeroBonds: the model reproduces the curve (arbitrage-free)
// for every grid within 4 standard errors.
func TestHullWhite_ZeroBonds(t *testing.T) {
	base := uint64(200)
	for _, a := range []float64{0.03, 0.5} {
		for _, sigma := range []float64{0.005, 0.015} {
			m, _ := hullwhite.New(curve, a, sigma)
			for _, c := range []struct {
				T float64
				n int
			}{{1, 4}, {10, 10}, {30, 60}, {5, 500}} {
				base++
				g := mustGrid(t, c.T, c.n)
				res, err := mc.PriceMulti(m, mc.ZeroBonds{Grid: g}, g, mc.Options{Paths: 20000, Seed: seed(base)})
				if err != nil {
					t.Fatal(err)
				}
				for i, r := range res[1:] {
					z := curve.Discount(g.Times[i+1])
					if math.Abs(r.Mean-z) > maxDev*r.StdErr {
						t.Errorf("a=%v σ=%v T=%v N=%d t=%v: %v ± %v vs %v", a, sigma, c.T, c.n, g.Times[i+1], r.Mean, r.StdErr, z)
					}
				}
			}
		}
	}
}

// TestHullWhite_Z checks Z(R0;0,T) = Z(0,T), the tower property
// E[Df(t) Z(r(t);t,T)] = Z(0,T) and E[r(t)] = φ(t), Var r(t) = σ² B(2a,t).
func TestHullWhite_Z(t *testing.T) {
	for _, a := range []float64{0, 1e-9, 0.1, 2} {
		m, _ := hullwhite.New(curve, a, 0.01)
		for _, T := range []float64{0.25, 3, 30} {
			if got, want := m.Z(m.R0(), 0, T), curve.Discount(T); math.Abs(got-want) > 1e-12 {
				t.Errorf("a=%v: Z(R0;0,%v)=%v, want %v", a, T, got, want)
			}
		}
		for _, c := range []struct{ t, T float64 }{{1, 2}, {3, 10}, {10, 30}} {
			g := mustGrid(t, c.t, 10)
			res, err := mc.Price(m, mc.PayoffFunc(func(p *mc.Path) float64 {
				n := len(p.Df) - 1
				return p.Df[n] * m.Z(p.State[0][n], c.t, c.T)
			}), g, mc.Options{Paths: 20000, Seed: seed(11), Antithetic: true})
			if err != nil {
				t.Fatal(err)
			}
			if want := curve.Discount(c.T); math.Abs(res.Mean-want) > 4*res.StdErr {
				t.Errorf("a=%v t=%v T=%v: E[Df Z]=%v ± %v, want %v", a, c.t, c.T, res.Mean, res.StdErr, want)
			}
			r, _ := mc.Price(m, mc.PayoffFunc(func(p *mc.Path) float64 { return p.State[0][len(p.Df)-1] }), g, mc.Options{Paths: 20000, Seed: seed(12)})
			if want := m.Phi(c.t); math.Abs(r.Mean-want) > 4*r.StdErr {
				t.Errorf("a=%v: E[r(%v)]=%v ± %v, want %v", a, c.t, r.Mean, r.StdErr, want)
			}
			b2 := c.t
			if a > 0 {
				b2 = (1 - math.Exp(-2*a*c.t)) / (2 * a)
			}
			if want := 0.01 * math.Sqrt(b2/20000); math.Abs(r.StdErr-want) > 0.05*want {
				t.Errorf("a=%v: sd r(%v)=%v, want %v", a, c.t, r.StdErr*math.Sqrt(20000), want*math.Sqrt(20000))
			}
		}
	}
	// a -> 0 is continuous
	m0, _ := hullwhite.New(curve, 0, 0.02)
	m1, _ := hullwhite.New(curve, 1e-13, 0.02)
	for _, T := range []float64{1, 10, 30} {
		if d := m0.Z(0.01, 2, T) - m1.Z(0.01, 2, T); math.Abs(d) > 1e-9 {
			t.Errorf("a->0: Z differs by %v", d)
		}
		if d := m0.IntPhi(T) - m1.IntPhi(T); math.Abs(d) > 1e-9 {
			t.Errorf("a->0: IntPhi differs by %v", d)
		}
	}
}

// TestHullWhite_Theta: for a flat curve θ(t) = a r + σ²/(2a)(1-e^{-2at}).
func TestHullWhite_Theta(t *testing.T) {
	a, sigma, r := 0.2, 0.01, 0.03
	m, _ := hullwhite.New(term.Flat{Rate: r}, a, sigma)
	for _, tt := range []float64{0, 0.5, 5, 20} {
		want := a*r + sigma*sigma/(2*a)*(1-math.Exp(-2*a*tt))
		if got := m.Theta(tt); math.Abs(got-want) > 1e-9 {
			t.Errorf("θ(%v)=%v, want %v", tt, got, want)
		}
	}
}

func TestHullWhite_Validation(t *testing.T) {
	for _, c := range []struct {
		c        term.Curve
		a, sigma float64
	}{{nil, 0.1, 0.01}, {curve, -0.1, 0.01}, {curve, 0.1, -0.01}, {curve, math.NaN(), 0.01}, {curve, 0.1, math.Inf(1)}} {
		if _, err := hullwhite.New(c.c, c.a, c.sigma); err == nil {
			t.Errorf("New(%v, %v, %v): expected error", c.c, c.a, c.sigma)
		}
	}
	bad := &hullwhite.HullWhite{Curve: term.Flat{Rate: math.NaN()}, A: 0.1, Sigma: 0.01}
	if _, err := bad.Discretize(mustGrid(t, 1, 2)); err == nil {
		t.Error("NaN curve: expected error")
	}
}
