package holee_test

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"testing"

	"github.com/konimarti/fixedincome/pkg/mc"
	"github.com/konimarti/fixedincome/pkg/mc/model/holee"
	"github.com/konimarti/fixedincome/pkg/term"
)

// curve is the NSS curve of the original Ho-Lee test (decimal parameters).
var curve = term.NSS{
	B0: -0.0043381, B1: -0.00308942, B2: 0.0483643, B3: -0.0410991,
	Tau1: 4.65211, Tau2: 3.33637,
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

// TestHoLee_Sigma0 checks that for σ = 0 the simulated discount factors
// reproduce the curve on every node (deterministically, no MC error).
func TestHoLee_Sigma0(t *testing.T) {
	m, err := holee.New(curve, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, T := range []float64{1, 5, 10, 30} {
		for _, n := range []int{10, 50, 1000} {
			g := mustGrid(t, T, n)
			res, err := mc.PriceMulti(m, mc.ZeroBonds{Grid: g}, g, mc.Options{Paths: 2, Seed: seed(1)})
			if err != nil {
				t.Fatal(err)
			}
			for i, r := range res {
				z := curve.Discount(g.Times[i])
				if math.Abs(r.Mean-z) > 1e-9 || r.StdErr > 1e-12 {
					t.Fatalf("T=%v N=%d node %d: E[Df]=%v (se %v), Z=%v", T, n, i, r.Mean, r.StdErr, z)
				}
			}
		}
	}
}

// TestHoLee_ZeroBonds checks E[exp(-∫r)] = Z(0,t_i) within maxDev standard
// errors on all nodes for several horizons, step counts and volatilities.
// Every case uses its own seed. σ = 0.02 is not tested at T = 30: there
// sd(∫r) ≈ 1.9 and the lognormal discount factor is so skewed that the
// sample standard error is not a reliable error measure.
func TestHoLee_ZeroBonds(t *testing.T) {
	base := uint64(100)
	for _, sigma := range []float64{0.005, 0.01, 0.02} {
		m, err := holee.New(curve, sigma)
		if err != nil {
			t.Fatal(err)
		}
		for _, T := range []float64{1, 5, 10, 30} {
			if sigma == 0.02 && T == 30 {
				continue
			}
			for _, n := range []int{10, 50, 1000} {
				base++
				paths := 20000
				if n == 1000 {
					paths = 4000
				}
				if testing.Short() {
					paths /= 4
				}
				g := mustGrid(t, T, n)
				res, err := mc.PriceMulti(m, mc.ZeroBonds{Grid: g}, g, mc.Options{Paths: paths, Seed: seed(base)})
				if err != nil {
					t.Fatal(err)
				}
				worst := 0.0
				for i, r := range res[1:] {
					z := curve.Discount(g.Times[i+1])
					dev := (r.Mean - z) / r.StdErr
					if math.Abs(dev) > math.Abs(worst) {
						worst = dev
					}
					if math.Abs(dev) > maxDev {
						t.Errorf("σ=%v T=%v N=%d t=%v: E[Df]=%v ± %v, Z=%v (%.1f SE)", sigma, T, n, g.Times[i+1], r.Mean, r.StdErr, z, dev)
					}
				}
				last := res[n]
				t.Logf("σ=%v T=%2v N=%4d: 100·E[Df_T]=%.4f ± %.4f, 100·Z=%.4f; worst node %.2f SE", sigma, T, n, 100*last.Mean, 100*last.StdErr, 100*curve.Discount(T), worst)
			}
		}
	}
}

// TestHoLee_Z checks the analytic bond price: Z(R0;0,T) is the curve and
// E[Df(t) Z(r(t);t,T)] = Z(0,T) (tower property) by Monte Carlo.
func TestHoLee_Z(t *testing.T) {
	for _, sigma := range []float64{0, 0.01, 0.03} {
		m, err := holee.New(curve, sigma)
		if err != nil {
			t.Fatal(err)
		}
		for _, T := range []float64{0.5, 2, 10, 30} {
			if got, want := m.Z(m.R0(), 0, T), curve.Discount(T); math.Abs(got-want) > 1e-12 {
				t.Errorf("Z(R0;0,%v)=%v, want %v", T, got, want)
			}
			if got := m.Z(0.03, T, T); math.Abs(got-1) > 1e-15 {
				t.Errorf("Z(r;T,T)=%v", got)
			}
		}
		for _, c := range []struct{ t, T float64 }{{1, 3}, {2, 7}, {5, 30}} {
			g := mustGrid(t, c.t, 20)
			res, err := mc.Price(m, mc.PayoffFunc(func(p *mc.Path) float64 {
				n := len(p.Df) - 1
				return p.Df[n] * m.Z(p.State[0][n], c.t, c.T)
			}), g, mc.Options{Paths: 20000, Seed: seed(3)})
			if err != nil {
				t.Fatal(err)
			}
			want := curve.Discount(c.T)
			if math.Abs(res.Mean-want) > 4*res.StdErr+1e-12 {
				t.Errorf("σ=%v t=%v T=%v: E[Df Z]=%v ± %v, want %v", sigma, c.t, c.T, res.Mean, res.StdErr, want)
			}
		}
	}
}

// TestHoLee_ShortRate checks E[r(t)] = φ(t) and Var r(t) = σ²t.
func TestHoLee_ShortRate(t *testing.T) {
	sigma := 0.01
	m, _ := holee.New(curve, sigma)
	g := mustGrid(t, 10, 40)
	n := g.Steps()
	mean, err := mc.Price(m, mc.PayoffFunc(func(p *mc.Path) float64 { return p.State[0][n] }), g, mc.Options{Paths: 20000, Seed: seed(5)})
	if err != nil {
		t.Fatal(err)
	}
	if want := m.Phi(10); math.Abs(mean.Mean-want) > 4*mean.StdErr {
		t.Errorf("E[r(10)]=%v ± %v, want %v", mean.Mean, mean.StdErr, want)
	}
	if want := sigma * math.Sqrt(10) / math.Sqrt(20000); math.Abs(mean.StdErr-want) > 0.05*want {
		t.Errorf("SE of r(10)=%v, want %v", mean.StdErr, want)
	}
	if got := m.R0(); math.Abs(got-curve.ZeroRate(0)) > 1e-8 {
		t.Errorf("R0=%v, want %v", got, curve.ZeroRate(0))
	}
	// θ for a flat curve is σ²t
	flat, _ := holee.New(term.Flat{Rate: 0.02}, sigma)
	if got := flat.Theta(3); math.Abs(got-sigma*sigma*3) > 1e-9 {
		t.Errorf("θ(3)=%v, want %v", got, sigma*sigma*3)
	}
}

// TestHoLee_Coverage checks that the 95% confidence interval covers the
// exact price in about 95% of 200 independent runs.
func TestHoLee_Coverage(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	m, _ := holee.New(curve, 0.02)
	g := mustGrid(t, 10, 20)
	want := curve.Discount(10)
	pay := mc.PayoffFunc(func(p *mc.Path) float64 { return p.Df[len(p.Df)-1] })
	covered := 0
	const runs = 200
	for s := uint64(1); s <= runs; s++ {
		res, err := mc.Price(m, pay, g, mc.Options{Paths: 2000, Seed: seed(s)})
		if err != nil {
			t.Fatal(err)
		}
		if lo, hi := res.CI95(); lo <= want && want <= hi {
			covered++
		}
	}
	frac := float64(covered) / runs
	t.Logf("coverage of the 95%% CI: %d/%d = %.3f", covered, runs, frac)
	if frac < 0.90 || frac > 0.99 {
		t.Errorf("coverage %v outside [0.90, 0.99]", frac)
	}
}

func TestHoLee_Validation(t *testing.T) {
	for _, c := range []struct {
		c     term.Curve
		sigma float64
	}{{nil, 0.01}, {curve, -0.01}, {curve, math.NaN()}, {curve, math.Inf(1)}} {
		if _, err := holee.New(c.c, c.sigma); err == nil {
			t.Errorf("New(%v, %v): expected error", c.c, c.sigma)
		}
	}
}

func TestHoLee_ZeroAllocs(t *testing.T) {
	m, _ := holee.New(curve, 0.01)
	g := mustGrid(t, 5, 50)
	sim, err := m.Discretize(g)
	if err != nil {
		t.Fatal(err)
	}
	p := mc.NewPath(g, sim.Factors())
	z := make([]float64, sim.Dim())
	rng := mc.NewRand(1, 0)
	for i := range z {
		z[i] = rng.NormFloat64()
	}
	if a := testing.AllocsPerRun(100, func() { sim.Simulate(z, p) }); a != 0 {
		t.Errorf("Simulate allocates %v times per path", a)
	}
}

func ExampleHoLee_Z() {
	m, _ := holee.New(term.Flat{Rate: 0.02}, 0.01)
	fmt.Printf("%.6f %.6f\n", m.Z(m.R0(), 0, 5), math.Exp(-0.02*5))
	// Output: 0.904837 0.904837
}
