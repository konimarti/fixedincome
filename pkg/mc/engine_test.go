package mc_test

import (
	"errors"
	"math"
	"os"
	"runtime"
	"strconv"
	"testing"

	"github.com/konimarti/fixedincome/pkg/mc"
)

// seed adds the offset 1000·$MC_SEED_OFFSET to a test seed (to check the
// robustness of the statistical tests over many seeds).
func seed(s uint64) uint64 {
	off, _ := strconv.Atoi(os.Getenv("MC_SEED_OFFSET"))
	return s + 1000*uint64(off)
}

// bm is a standard Brownian motion W (State[0]) with Df = 1.
type bm struct{}

type bmSim struct{ sq []float64 }

func (bm) Discretize(g *mc.Grid) (mc.Simulator, error) {
	s := bmSim{sq: make([]float64, g.Steps())}
	for i := range s.sq {
		s.sq[i] = math.Sqrt(g.Dt(i))
	}
	return s, nil
}
func (s bmSim) Factors() int { return 1 }
func (s bmSim) Dim() int     { return len(s.sq) }
func (s bmSim) Simulate(z []float64, p *mc.Path) {
	w := p.State[0]
	w[0] = 0
	for i := range s.sq {
		w[i+1] = w[i] + s.sq[i]*z[i]
		p.Df[i+1] = 1
	}
	p.Df[0] = 1
}

func grid(t testing.TB, T float64, n int) *mc.Grid {
	t.Helper()
	g, err := mc.NewUniformGrid(T, n)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func last(p *mc.Path) float64 { return p.State[0][len(p.State[0])-1] }

func TestGrid(t *testing.T) {
	g := grid(t, 3, 7)
	if len(g.Times) != 8 || g.Times[0] != 0 || g.Times[7] != 3 || g.Steps() != 7 || g.T() != 3 {
		t.Fatalf("uniform grid: %v", g.Times)
	}
	if i, ok := g.Index(3.0 * 2 / 7); !ok || i != 2 {
		t.Errorf("Index: %v %v", i, ok)
	}
	if _, ok := g.Index(0.1); ok {
		t.Errorf("Index(0.1) should not be a node")
	}
	for _, c := range []struct {
		T float64
		n int
	}{{0, 10}, {-1, 10}, {math.NaN(), 10}, {math.Inf(1), 10}, {1, 0}, {1, -3}} {
		if _, err := mc.NewUniformGrid(c.T, c.n); err == nil {
			t.Errorf("NewUniformGrid(%v, %d): expected error", c.T, c.n)
		}
	}
	for _, ts := range [][]float64{nil, {0}, {0.1, 1}, {0, 1, 1}, {0, 2, 1}, {0, math.NaN()}} {
		if _, err := mc.NewGrid(ts); err == nil {
			t.Errorf("NewGrid(%v): expected error", ts)
		}
	}
	if g, err := mc.NewGrid([]float64{0, 0.5, 2}); err != nil || g.Dt(1) != 1.5 {
		t.Errorf("NewGrid: %v %v", g, err)
	}
}

func TestOptionsValidation(t *testing.T) {
	g := grid(t, 1, 4)
	pay := mc.PayoffFunc(last)
	for _, o := range []mc.Options{
		{Paths: 0}, {Paths: -5}, {Paths: 1}, {Paths: 10, Workers: -1},
		{Paths: 11, Antithetic: true}, {Paths: 2, Antithetic: true},
		{Paths: 10, Control: &mc.Control{}},
		{Paths: 10, Control: &mc.Control{X: last, Mean: math.NaN()}},
		{Paths: 2, Control: &mc.Control{X: last}},
		{Paths: mc.MinControlSamples - 1, Control: &mc.Control{X: last}},
		{Paths: 2*mc.MinControlSamples - 2, Antithetic: true, Control: &mc.Control{X: last}},
		{Paths: 10, TrainingPaths: -1},
	} {
		if _, err := mc.Price(bm{}, pay, g, o); err == nil {
			t.Errorf("Price with %+v: expected error", o)
		}
	}
	if _, err := mc.Price(nil, pay, g, mc.Options{Paths: 10}); err == nil {
		t.Error("nil model: expected error")
	}
	if _, err := mc.Price(bm{}, nil, g, mc.Options{Paths: 10}); err == nil {
		t.Error("nil payoff: expected error")
	}
	if _, err := mc.Price(bm{}, pay, nil, mc.Options{Paths: 10}); err == nil {
		t.Error("nil grid: expected error")
	}
	if _, err := mc.Price(bm{}, pay, &mc.Grid{Times: []float64{0, 1, 0.5}}, mc.Options{Paths: 10}); err == nil {
		t.Error("invalid grid literal: expected error")
	}
	_, err := mc.Price(bm{}, mc.PayoffFunc(func(*mc.Path) float64 { return math.NaN() }), g, mc.Options{Paths: 10})
	if !errors.Is(err, mc.ErrNonFinite) {
		t.Errorf("NaN payoff: got %v", err)
	}
}

// TestReproducible checks that the result depends on the seed only, not on
// the number of workers.
func TestReproducible(t *testing.T) {
	g := grid(t, 1, 8)
	pay := mc.PayoffFunc(func(p *mc.Path) float64 { return math.Exp(last(p)) })
	for _, anti := range []bool{false, true} {
		var ref mc.Result
		for i, w := range []int{1, 2, 3, 8, 0} {
			res, err := mc.Price(bm{}, pay, g, mc.Options{Paths: 10*mc.BlockSize + 2, Workers: w, Seed: 42, Antithetic: anti})
			if err != nil {
				t.Fatal(err)
			}
			if i == 0 {
				ref = res
			} else if res != ref {
				t.Errorf("workers=%d: %+v, workers=1: %+v", w, res, ref)
			}
		}
		other, _ := mc.Price(bm{}, pay, g, mc.Options{Paths: 10*mc.BlockSize + 2, Seed: 43, Antithetic: anti})
		if other.Mean == ref.Mean {
			t.Errorf("different seeds give the same result")
		}
	}
}

// TestStatistics compares the Welford/Chan accumulation with a two-pass
// computation on the same samples (serial payoff calls with one worker).
func TestStatistics(t *testing.T) {
	g := grid(t, 1, 3)
	var ys []float64
	pay := mc.PayoffFunc(func(p *mc.Path) float64 {
		y := math.Exp(last(p)) + 1e6 // large offset tests numerical stability
		ys = append(ys, y)
		return y
	})
	n := 5*mc.BlockSize + 17
	res, err := mc.Price(bm{}, pay, g, mc.Options{Paths: n, Workers: 1, Seed: seed(9)})
	if err != nil {
		t.Fatal(err)
	}
	mean := 0.0
	for _, y := range ys {
		mean += y
	}
	mean /= float64(n)
	v := 0.0
	for _, y := range ys {
		v += (y - mean) * (y - mean)
	}
	se := math.Sqrt(v / float64(n-1) / float64(n))
	if len(ys) != n || res.N != n || res.Paths != n {
		t.Fatalf("samples: %d %d %d", len(ys), res.N, res.Paths)
	}
	if math.Abs(res.Mean-mean) > 1e-13*mean || math.Abs(res.StdErr-se) > 1e-9*se {
		t.Errorf("mean %v vs %v, se %v vs %v", res.Mean, mean, res.StdErr, se)
	}
	if lo, hi := res.CI95(); math.Abs((hi-lo)/(2*res.StdErr)-1.959964) > 1e-6 {
		t.Errorf("CI95 half width %v", (hi-lo)/2)
	}
	// E[exp(W_1)] = e^{1/2}
	if want := math.Exp(0.5) + 1e6; math.Abs(res.Mean-want) > 4*res.StdErr {
		t.Errorf("E[exp(W)] = %v ± %v, want %v", res.Mean, res.StdErr, want)
	}
}

func TestAntitheticAndControl(t *testing.T) {
	g := grid(t, 1, 4)
	// linear payoff: antithetic pairs are exact
	res, err := mc.Price(bm{}, mc.PayoffFunc(func(p *mc.Path) float64 { return 2 + 3*last(p) }), g, mc.Options{Paths: 1000, Seed: 1, Antithetic: true})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(res.Mean-2) > 1e-12 || res.StdErr > 1e-12 || res.N != 500 || res.Paths != 1000 {
		t.Errorf("antithetic linear payoff: %+v", res)
	}
	// payoff exp(W) with control W (mean 0): β = Cov(e^W, W)/Var W = e^{1/2}.
	// exp(W) has a kurtosis of about 114, so the relative error of a
	// sample SE is sqrt(113/(4n)): 1e6 paths give ±0.005 on the SE ratio.
	pay := mc.PayoffFunc(func(p *mc.Path) float64 { return math.Exp(last(p)) })
	plain, _ := mc.Price(bm{}, pay, g, mc.Options{Paths: 1000000, Seed: seed(2)})
	cv, err := mc.Price(bm{}, pay, g, mc.Options{Paths: 1000000, Seed: seed(2), Control: &mc.Control{X: last, Mean: 0}})
	if err != nil {
		t.Fatal(err)
	}
	want := math.Exp(0.5)
	for _, r := range []mc.Result{plain, cv} {
		if math.Abs(r.Mean-want) > 4*r.StdErr {
			t.Errorf("E[exp(W)] = %v ± %v, want %v", r.Mean, r.StdErr, want)
		}
	}
	// β from the pilot sample (256 samples): sd(β) ≈ 0.09 for Gaussian
	// residuals, more for the heavy-tailed exp(W) (observed 1.29 for one
	// of 20 seed offsets)
	if math.Abs(cv.Beta-want) > 0.6 {
		t.Errorf("β = %v, want %v", cv.Beta, want)
	}
	if cv.N != 1000000-256 || cv.Paths != 1000000 {
		t.Errorf("pilot: N=%d Paths=%d", cv.N, cv.Paths)
	}
	// Var reduction: 1 - ρ² with ρ = e^{1/2}/sqrt(e^2-e)
	rho2 := math.E / (math.E*math.E - math.E)
	if ratio := cv.StdErr / plain.StdErr; math.Abs(ratio-math.Sqrt(1-rho2)) > 0.03 {
		t.Errorf("SE ratio %v, want %v", ratio, math.Sqrt(1-rho2))
	}
	t.Logf("exp(W): plain SE %.5f, control variate SE %.5f (ratio %.3f)", plain.StdErr, cv.StdErr, cv.StdErr/plain.StdErr)
}

func TestPriceMulti(t *testing.T) {
	g := grid(t, 2, 4)
	res, err := mc.PriceMulti(bm{}, mc.ZeroBonds{Grid: g}, g, mc.Options{Paths: 10, Seed: 1})
	if err != nil || len(res) != 5 {
		t.Fatalf("%v %v", res, err)
	}
	for _, r := range res {
		if r.Mean != 1 || r.StdErr != 0 {
			t.Errorf("deterministic Df: %+v", r)
		}
	}
}

// TestAllocs checks that the number of allocations does not grow with the
// number of paths (per-worker buffers, no per-path allocations).
func TestAllocs(t *testing.T) {
	g := grid(t, 1, 16)
	pay := mc.PayoffFunc(last)
	for _, w := range []int{1, 4} {
		allocs := func(paths int) float64 {
			return testing.AllocsPerRun(5, func() {
				if _, err := mc.Price(bm{}, pay, g, mc.Options{Paths: paths, Workers: w, Seed: 1, Antithetic: true}); err != nil {
					t.Fatal(err)
				}
			})
		}
		small, large := allocs(4*mc.BlockSize), allocs(128*mc.BlockSize)
		t.Logf("workers %d: allocations per Price call: %v (%d paths), %v (%d paths)", w, small, 4*mc.BlockSize, large, 128*mc.BlockSize)
		// a few runtime allocations (goroutine parking) may occur, but
		// nothing per path
		if perPath := (large - small) / float64(124*mc.BlockSize); perPath > 1e-3 {
			t.Errorf("allocations grow with the number of paths: %v -> %v", small, large)
		}
	}
}

func TestPi(t *testing.T) {
	g := grid(t, 1, 2)
	// z1, z2 standard normal: P(Φ(z1)²+Φ(z2)² <= 1) = π/4
	phi := func(x float64) float64 { return 0.5 * math.Erfc(-x/math.Sqrt2) }
	res, err := mc.Price(bm{}, mc.PayoffFunc(func(p *mc.Path) float64 {
		w := p.State[0]
		x, y := phi(w[1]/math.Sqrt(0.5)), phi((w[2]-w[1])/math.Sqrt(0.5))
		if x*x+y*y <= 1 {
			return 1
		}
		return 0
	}), g, mc.Options{Paths: 200000, Seed: seed(99)})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(res.Mean-math.Pi/4) > 4*res.StdErr {
		t.Errorf("π/4 = %v ± %v", res.Mean, res.StdErr)
	}
	if want := math.Sqrt(math.Pi / 4 * (1 - math.Pi/4) / 200000); math.Abs(res.StdErr-want) > 0.01*want {
		t.Errorf("SE %v, want %v", res.StdErr, want)
	}
}

// TestMemory checks that the memory use does not grow with the number of
// paths (ordered streaming merge of the block statistics): before, the
// statistics of all blocks were kept (2^20 paths, 1001 components: 58.7 MB).
func TestMemory(t *testing.T) {
	g := grid(t, 1, 1000)
	alloc := func(paths int) float64 {
		runtime.GC()
		var a, b runtime.MemStats
		runtime.ReadMemStats(&a)
		if _, err := mc.PriceMulti(bm{}, mc.ZeroBonds{Grid: g}, g, mc.Options{Paths: paths, Workers: 4, Seed: 1}); err != nil {
			t.Fatal(err)
		}
		runtime.ReadMemStats(&b)
		return float64(b.TotalAlloc - a.TotalAlloc)
	}
	small, large := alloc(16*mc.BlockSize), alloc(128*mc.BlockSize)
	t.Logf("total allocation: %.2f MB (%d paths), %.2f MB (%d paths)", small/1e6, 16*mc.BlockSize, large/1e6, 128*mc.BlockSize)
	// bound: 2 paths, normals and (window + workers) = 12 buffers of 1001
	// moments per worker plus slack
	if large > 4e6 || large > 1.5*small+1e5 {
		t.Errorf("allocation grows with the number of paths: %v -> %v bytes", small, large)
	}
}

// TestPanic checks that panics in payoffs and simulators are returned as
// errors (and do not deadlock the ordered merge).
func TestPanic(t *testing.T) {
	g := grid(t, 1, 4)
	for _, w := range []int{1, 3, 8} {
		_, err := mc.Price(bm{}, mc.PayoffFunc(func(p *mc.Path) float64 {
			if p.State[0][4] > 2.5 {
				panic("boom")
			}
			return 1
		}), g, mc.Options{Paths: 50 * mc.BlockSize, Workers: w, Seed: 1})
		if !errors.Is(err, mc.ErrPanic) {
			t.Errorf("workers=%d: got %v, want ErrPanic", w, err)
		}
	}
	if _, err := mc.Price(panicModel{}, mc.PayoffFunc(last), g, mc.Options{Paths: 10}); !errors.Is(err, mc.ErrPanic) {
		t.Errorf("panicking Discretize: got %v", err)
	}
	if _, err := mc.PriceAmerican(bm{}, panicOption{}, g, mc.Options{Paths: 100}); !errors.Is(err, mc.ErrPanic) {
		t.Errorf("panicking option: got %v", err)
	}
	// grid mismatch of ZeroBonds
	if _, err := mc.PriceMulti(bm{}, mc.ZeroBonds{Grid: grid(t, 2, 4)}, g, mc.Options{Paths: 10}); err == nil {
		t.Error("ZeroBonds on another grid: expected error")
	}
	if _, err := mc.PriceMulti(bm{}, &mc.ZeroBonds{}, g, mc.Options{Paths: 10}); err == nil {
		t.Error("ZeroBonds without grid: expected error")
	}
}

type panicModel struct{}

func (panicModel) Discretize(*mc.Grid) (mc.Simulator, error) { panic("discretize") }

type panicOption struct{}

func (panicOption) CanExercise(int) bool                 { return true }
func (panicOption) Exercise(p *mc.Path, i int) float64   { return 1 }
func (panicOption) NumBasis() int                        { return 1 }
func (panicOption) Basis(p *mc.Path, i int, o []float64) { panic("basis") }

// european is an Exercisable that can only be exercised at maturity, with
// payoff max(W_T, 0); there is nothing to regress.
type european struct{ n int }

func (e european) CanExercise(i int) bool             { return i == e.n }
func (e european) Exercise(p *mc.Path, i int) float64 { return math.Max(p.State[0][i], 0) }
func (european) NumBasis() int                        { return 1 }
func (european) Basis(p *mc.Path, i int, o []float64) { o[0] = 1 }

// TestAmericanOutOfSample checks that PriceAmerican prices on paths that
// are independent of the training paths (streams 0, 1, ... for pricing as
// in Price, separate streams for training), so the pricing estimate is not
// the in-sample estimate even with equal path counts.
func TestAmericanOutOfSample(t *testing.T) {
	g := grid(t, 1, 4)
	o := mc.Options{Paths: 4096, TrainingPaths: 4096, Seed: 3}
	res, err := mc.PriceAmerican(bm{}, european{4}, g, o)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := mc.Price(bm{}, mc.PayoffFunc(func(p *mc.Path) float64 { return math.Max(last(p), 0) }), g, o)
	if err != nil {
		t.Fatal(err)
	}
	if res.Result != ref {
		t.Errorf("pricing pass %+v, Price %+v", res.Result, ref)
	}
	if res.Mean == res.InSample {
		t.Errorf("pricing estimate equals the in-sample estimate %v", res.InSample)
	}
	// both estimate E[max(W,0)] = 1/sqrt(2π)
	want := 1 / math.Sqrt(2*math.Pi)
	if math.Abs(res.InSample-want) > 4*res.InSampleStdErr || math.Abs(res.Mean-want) > 4*res.StdErr {
		t.Errorf("in-sample %v ± %v, out-of-sample %v ± %v, want %v", res.InSample, res.InSampleStdErr, res.Mean, res.StdErr, want)
	}
}

// TestCoverage checks the coverage of the 95% confidence interval for small
// samples with antithetic variates and with a control variate (β from the
// pilot sample): 2000 runs of 100 (pairs of) samples each.
func TestCoverage(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	g := grid(t, 1, 2)
	// Y = exp(0.3 W(1/2)) + max(W(1) - W(1/2), 0) with the control
	// X = W(1/2): the residual Y - βX is dominated by the independent
	// noise, so the coverage measures the estimator, not the skewness of
	// the payoff (with Y = exp(W/2), X = W the residual is chi-square like
	// and all estimators under-cover by ~2.5%).
	pay := mc.PayoffFunc(func(p *mc.Path) float64 {
		w := p.State[0]
		return math.Exp(0.3*w[1]) + math.Max(w[2]-w[1], 0)
	})
	want := math.Exp(0.3*0.3*0.5/2) + math.Sqrt(0.5/(2*math.Pi))
	cv := &mc.Control{X: func(p *mc.Path) float64 { return p.State[0][1] }, Mean: 0}
	const runs = 2000
	cov := map[string]float64{}
	for _, c := range []struct {
		name, base string // base: the same estimator without control variate
		o          mc.Options
	}{
		{"plain", "", mc.Options{Paths: 100}},
		{"antithetic", "", mc.Options{Paths: 200, Antithetic: true}},
		{"control", "plain", mc.Options{Paths: 100, Control: cv}},
		{"antithetic+control", "antithetic", mc.Options{Paths: 200, Antithetic: true, Control: cv}},
	} {
		covered := 0
		for s := uint64(1); s <= runs; s++ {
			o := c.o
			o.Seed = seed(s)
			res, err := mc.Price(bm{}, pay, g, o)
			if err != nil {
				t.Fatal(err)
			}
			if lo, hi := res.CI95(); lo <= want && want <= hi {
				covered++
			}
		}
		frac := float64(covered) / runs
		t.Logf("%s: coverage %.3f", c.name, frac)
		// sd of the coverage over 2000 runs: 0.005; observed 0.934-0.953
		// for 20 seed offsets
		if frac < 0.925 || frac > 0.965 {
			t.Errorf("%s: coverage %v outside [0.925, 0.965]", c.name, frac)
		}
		// with the pilot β the control variate does not under-cover
		// relative to the same estimator without it (in-sample β: 0.883
		// with 100 pairs); sd of the difference 0.007
		cov[c.name] = frac
		if c.base != "" && frac < cov[c.base]-0.025 {
			t.Errorf("%s: coverage %v, %s %v", c.name, frac, c.base, cov[c.base])
		}
	}
}
