package stock_test

import (
	"math"
	"os"
	"strconv"
	"testing"

	"github.com/konimarti/fixedincome/pkg/instrument/option"
	"github.com/konimarti/fixedincome/pkg/mc"
	"github.com/konimarti/fixedincome/pkg/mc/model/stock"
	"github.com/konimarti/fixedincome/pkg/term"
)

func mustGrid(t testing.TB, T float64, n int) *mc.Grid {
	t.Helper()
	g, err := mc.NewUniformGrid(T, n)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func mustGBM(t testing.TB, s0, sigma, q float64, c term.Curve) *stock.GBM {
	t.Helper()
	m, err := stock.New(s0, sigma, q, c)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// seed adds the offset 1000·$MC_SEED_OFFSET to a test seed (to check the
// robustness of the statistical tests over many seeds).
func seed(s uint64) uint64 {
	off, _ := strconv.Atoi(os.Getenv("MC_SEED_OFFSET"))
	return s + 1000*uint64(off)
}

var nss = term.NSS{B0: 0.03, B1: -0.02, B2: 0.01, B3: 0.005, Tau1: 2, Tau2: 8}

// TestEuropean compares European calls and puts (with dividend yield and a
// non-flat curve) with Black-Scholes; plain, antithetic and with the
// terminal stock price as control variate.
func TestEuropean(t *testing.T) {
	base := uint64(400)
	for _, c := range []struct {
		curve    term.Curve
		s0, k, q float64
		sigma, T float64
		steps    int
		typ      option.Type
	}{
		{term.Flat{Rate: 0.02}, 110, 100, 0, 0.3, 2, 1, option.Call},
		{term.Flat{Rate: 0.02}, 110, 100, 0.03, 0.3, 2, 8, option.Put},
		{nss, 100, 105, 0.01, 0.2, 5, 20, option.Call},
		{nss, 100, 105, 0.01, 0.2, 5, 20, option.Put},
	} {
		m := mustGBM(t, c.s0, c.sigma, c.q, c.curve)
		g := mustGrid(t, c.T, c.steps)
		want, err := m.European(c.typ, c.k, c.T)
		if err != nil {
			t.Fatal(err)
		}
		pay := stock.European{Type: c.typ, Strike: c.k}
		var se [3]float64
		for i, o := range []mc.Options{
			{Paths: 100000},
			{Paths: 100000, Antithetic: true},
			{Paths: 100000, Control: m.TerminalControl(g)},
		} {
			base++
			o.Seed = seed(base)
			res, err := mc.Price(m, pay, g, o)
			if err != nil {
				t.Fatal(err)
			}
			se[i] = res.StdErr
			if math.Abs(res.Mean-want) > 4*res.StdErr {
				t.Errorf("%v K=%v q=%v (%d): %v ± %v, Black-Scholes %v", c.typ, c.k, c.q, i, res.Mean, res.StdErr, want)
			}
		}
		t.Logf("%v S=%v K=%v q=%v T=%v: BS %.4f; SE plain %.4f, antithetic/plain %.3f, control/plain %.3f (same number of paths)",
			c.typ, c.s0, c.k, c.q, c.T, want, se[0], se[1]/se[0], se[2]/se[0])
	}
}

// TestMartingale: for σ = 0 the discounted stock price is deterministic,
// Df S = S0 e^{-qt} on every node, and E[Df S] = S0 e^{-qt} for σ > 0.
func TestMartingale(t *testing.T) {
	for _, sigma := range []float64{0, 0.4} {
		m := mustGBM(t, 50, sigma, 0.02, nss)
		g, _ := mc.NewGrid([]float64{0, 0.1, 0.5, 2, 7, 10})
		res, err := mc.PriceMulti(m, multi{g}, g, mc.Options{Paths: 50000, Seed: seed(1)})
		if err != nil {
			t.Fatal(err)
		}
		for i, r := range res {
			want := 50 * math.Exp(-0.02*g.Times[i])
			tol := 4 * r.StdErr
			if sigma == 0 {
				tol = 1e-11
			}
			if math.Abs(r.Mean-want) > tol {
				t.Errorf("σ=%v t=%v: E[Df S]=%v ± %v, want %v", sigma, g.Times[i], r.Mean, r.StdErr, want)
			}
		}
	}
}

type multi struct{ g *mc.Grid }

func (m multi) Len() int { return len(m.g.Times) }
func (m multi) Values(p *mc.Path, out []float64) {
	for i := range out {
		out[i] = p.Df[i] * p.State[0][i]
	}
}

// TestGeometricAsian checks the closed form: with one monitoring date it is
// the Black-Scholes price; Monte Carlo agrees within 4 standard errors.
func TestGeometricAsian(t *testing.T) {
	m := mustGBM(t, 100, 0.25, 0.01, nss)
	for _, typ := range []option.Type{option.Call, option.Put} {
		g1 := mustGrid(t, 2, 1)
		got, err := m.GeometricAsian(typ, 95, g1)
		if err != nil {
			t.Fatal(err)
		}
		want, _ := m.European(typ, 95, 2)
		if math.Abs(got-want) > 1e-10 {
			t.Errorf("%v: geometric Asian with one date %v, Black-Scholes %v", typ, got, want)
		}
		g := mustGrid(t, 1, 12)
		cf, _ := m.GeometricAsian(typ, 95, g)
		res, err := mc.Price(m, stock.Asian{Type: typ, Strike: 95, Geometric: true}, g, mc.Options{Paths: 100000, Seed: seed(5)})
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(res.Mean-cf) > 4*res.StdErr {
			t.Errorf("%v: geometric Asian MC %v ± %v, closed form %v", typ, res.Mean, res.StdErr, cf)
		}
	}
	// Kemna and Vorst (1990) continuous-average limit: σ_G = σ/√3
	flat := mustGBM(t, 100, 0.2, 0, term.Flat{Rate: 0.05})
	g := mustGrid(t, 1, 20000)
	got, _ := flat.GeometricAsian(option.Call, 100, g)
	b := 0.5 * (0.05 - 0.2*0.2/6)
	ref := option.European{Type: option.Call, S: 100, K: 100, T: 1, Vol: 0.2 / math.Sqrt(3), Q: 0.05 - b}
	want, _ := ref.Price(term.Flat{Rate: 0.05})
	if math.Abs(got-want) > 1e-3 {
		t.Errorf("continuous geometric Asian %v, Kemna-Vorst %v", got, want)
	}
}

// TestArithmeticAsian: the geometric control variate gives the same price
// as plain Monte Carlo with a much smaller standard error.
func TestArithmeticAsian(t *testing.T) {
	m := mustGBM(t, 100, 0.3, 0.02, term.Flat{Rate: 0.05})
	g := mustGrid(t, 1, 52)
	for _, typ := range []option.Type{option.Call, option.Put} {
		pay := stock.Asian{Type: typ, Strike: 100}
		cv, err := m.GeometricAsianControl(typ, 100, g)
		if err != nil {
			t.Fatal(err)
		}
		plain, _ := mc.Price(m, pay, g, mc.Options{Paths: 100000, Seed: seed(21)})
		ctl, err := mc.Price(m, pay, g, mc.Options{Paths: 100000, Seed: seed(22), Control: cv})
		if err != nil {
			t.Fatal(err)
		}
		geo, _ := m.GeometricAsian(typ, 100, g)
		diff := plain.Mean - ctl.Mean
		if math.Abs(diff) > 4*math.Hypot(plain.StdErr, ctl.StdErr) {
			t.Errorf("%v: plain %v ± %v, control variate %v ± %v", typ, plain.Mean, plain.StdErr, ctl.Mean, ctl.StdErr)
		}
		// AM >= GM pathwise: call above, put below the geometric price
		if (typ == option.Call && ctl.Mean < geo) || (typ == option.Put && ctl.Mean > geo) {
			t.Errorf("%v: arithmetic %v vs geometric %v", typ, ctl.Mean, geo)
		}
		if ctl.StdErr > 0.1*plain.StdErr {
			t.Errorf("%v: control variate SE %v not much smaller than %v", typ, ctl.StdErr, plain.StdErr)
		}
		t.Logf("arithmetic Asian %v: plain %.4f ± %.4f, geometric CV %.5f ± %.5f (SE ratio %.4f, β=%.3f), geometric %.4f",
			typ, plain.Mean, plain.StdErr, ctl.Mean, ctl.StdErr, ctl.StdErr/plain.StdErr, ctl.Beta, geo)
	}
}

// crr prices an option on a Cox-Ross-Rubinstein tree with n steps and
// exercise allowed every `every` steps (never at t = 0); every = 1 is the
// American option.
func crr(typ option.Type, s, k, r, q, sigma, T float64, n, every int) float64 {
	dt := T / float64(n)
	u := math.Exp(sigma * math.Sqrt(dt))
	d := 1 / u
	p := (math.Exp((r-q)*dt) - d) / (u - d)
	disc := math.Exp(-r * dt)
	payoff := func(x float64) float64 {
		if typ == option.Put {
			return math.Max(k-x, 0)
		}
		return math.Max(x-k, 0)
	}
	v := make([]float64, n+1)
	x := s * math.Pow(u, -float64(n))
	for j := 0; j <= n; j++ {
		v[j] = payoff(x)
		x *= u * u
	}
	for i := n - 1; i >= 0; i-- {
		ex := i > 0 && i%every == 0
		x := s * math.Pow(u, -float64(i))
		for j := 0; j <= i; j++ {
			v[j] = disc * (p*v[j+1] + (1-p)*v[j])
			if ex {
				v[j] = math.Max(v[j], payoff(x))
			}
			x *= u * u
		}
	}
	return v[0]
}

// bermudan prices the option exercisable on `dates` equally spaced dates
// t_1..t_N (like stock.American on a uniform grid with N = dates steps):
// the average of trees with per and 2·per steps between exercise dates
// (damps the odd-even oscillation of the CRR tree).
func bermudan(typ option.Type, s, k, r, q, sigma, T float64, dates, per int) float64 {
	return 0.5 * (crr(typ, s, k, r, q, sigma, T, dates*per, per) + crr(typ, s, k, r, q, sigma, T, dates*2*per, 2*per))
}

// americanCases are the puts of Longstaff and Schwartz (2001, Table 1;
// K = 40, r = 0.06) and a put and a call with a dividend yield, all with
// 50 exercise dates per year; ref is the Bermudan value (independent
// reference trees of the review: 4.4779, 2.3140, 1.1099, 8.5069, put
// 4.4469, call 4.7711).
var americanCases = []struct {
	typ        option.Type
	s, k, r, q float64
	sigma, T   float64
	ref        float64
}{
	{option.Put, 36, 40, 0.06, 0, 0.2, 1, 4.4779},
	{option.Put, 40, 40, 0.06, 0, 0.2, 1, 2.3140},
	{option.Put, 44, 40, 0.06, 0, 0.2, 1, 1.1099},
	{option.Put, 36, 40, 0.06, 0, 0.4, 2, 8.5069},
	{option.Put, 40, 40, 0.05, 0.04, 0.3, 1, 4.4469},
	{option.Call, 40, 40, 0.05, 0.04, 0.3, 1, 4.7711},
}

// TestBermudanTree checks the test's own tree against the references.
func TestBermudanTree(t *testing.T) {
	for _, c := range americanCases {
		dates := int(50 * c.T)
		got := bermudan(c.typ, c.s, c.k, c.r, c.q, c.sigma, c.T, dates, 40)
		t.Logf("%v S=%v: tree %.5f, reference %.4f", c.typ, c.s, got, c.ref)
		if math.Abs(got-c.ref) > 6e-4 {
			t.Errorf("%v S=%v: tree %.5f, reference %.4f", c.typ, c.s, got, c.ref)
		}
	}
}

// TestAmerican compares Longstaff-Schwartz (European value as basis
// function and continuation bound; European payoff as control variate)
// with the Bermudan tree values. The
// out-of-sample estimate is low-biased (sub-optimal rule): it must not
// exceed the reference by more than 4 SE and may be below it by at most
// 0.01 + 4 SE.
func TestAmerican(t *testing.T) {
	paths := 100000
	if testing.Short() {
		paths = 20000
	}
	for i, c := range americanCases {
		m := mustGBM(t, c.s, c.sigma, c.q, term.Flat{Rate: c.r})
		g := mustGrid(t, c.T, int(50*c.T))
		cv, err := m.EuropeanControl(c.typ, c.k, g)
		if err != nil {
			t.Fatal(err)
		}
		res, err := mc.PriceAmerican(m, m.American(c.typ, c.k), g, mc.Options{Paths: paths, Seed: seed(uint64(500 + i)), Control: cv})
		if err != nil {
			t.Fatal(err)
		}
		eu, _ := m.European(c.typ, c.k, c.T)
		t.Logf("%v S=%v σ=%v T=%v q=%v: LSM %.4f ± %.4f (in-sample %.4f ± %.4f), Bermudan %.4f, European %.4f",
			c.typ, c.s, c.sigma, c.T, c.q, res.Mean, res.StdErr, res.InSample, res.InSampleStdErr, c.ref, eu)
		if res.Mean > c.ref+4*res.StdErr || res.Mean < c.ref-0.01-4*res.StdErr {
			t.Errorf("%v S=%v: LSM %v ± %v, Bermudan %v", c.typ, c.s, res.Mean, res.StdErr, c.ref)
		}
		if math.Abs(res.InSample-c.ref) > 0.01+4*res.InSampleStdErr {
			t.Errorf("%v S=%v: LSM in-sample %v, Bermudan %v", c.typ, c.s, res.InSample, c.ref)
		}
		if res.Mean < eu-4*res.StdErr {
			t.Errorf("%v S=%v: LSM %v below the European price %v", c.typ, c.s, res.Mean, eu)
		}
	}
}

// TestAmericanCallNoDividend: with the European continuation bound a call
// without dividends is never exercised early, so the LSM estimate equals
// the Monte Carlo price of the European call on the same paths exactly
// (the review found LSM < European - 4 SE for 3 of 60 seeds without it).
func TestAmericanCallNoDividend(t *testing.T) {
	m := mustGBM(t, 100, 0.25, 0, term.Flat{Rate: 0.05})
	g := mustGrid(t, 1, 25)
	eu, _ := m.European(option.Call, 100, 1)
	for s := uint64(1); s <= 5; s++ {
		o := mc.Options{Paths: 20000, Seed: seed(s), Control: m.TerminalControl(g)}
		a := m.American(option.Call, 100)
		a.Degree = 2
		res, err := mc.PriceAmerican(m, a, g, o)
		if err != nil {
			t.Fatal(err)
		}
		ref, err := mc.Price(m, stock.European{Type: option.Call, Strike: 100}, g, o)
		if err != nil {
			t.Fatal(err)
		}
		if res.Result != ref {
			t.Errorf("seed %d: American call %+v, European MC %+v", s, res.Result, ref)
		}
		if math.Abs(res.Mean-eu) > 4*res.StdErr {
			t.Errorf("seed %d: American call %v ± %v, Black-Scholes %v", s, res.Mean, res.StdErr, eu)
		}
	}
}

func TestAmericanValidation(t *testing.T) {
	m := mustGBM(t, 100, 0.25, 0, term.Flat{Rate: 0.05})
	g := mustGrid(t, 1, 25)
	if _, err := mc.PriceAmerican(m, nil, g, mc.Options{Paths: 100}); err == nil {
		t.Error("nil option: expected error")
	}
	if _, err := mc.PriceAmerican(m, m.American(option.Put, 100), g, mc.Options{Paths: 100, TrainingPaths: 3}); err == nil {
		t.Error("too few training paths: expected error")
	}
	for _, a := range []stock.American{
		{Type: option.Put, Strike: 0}, {Type: option.Put, Strike: -1}, {Type: 0, Strike: 100},
		{Type: option.Put, Strike: math.Inf(1)}, {Type: option.Put, Strike: 100, Degree: 11},
		{Type: option.Put, Strike: 100, Model: &stock.GBM{}},
	} {
		if _, err := mc.PriceAmerican(m, a, g, mc.Options{Paths: 100}); err == nil {
			t.Errorf("%+v: expected error", a)
		}
	}
}

func TestValidation(t *testing.T) {
	for _, c := range []struct {
		s0, sigma, q float64
		c            term.Curve
	}{{0, 0.2, 0, nss}, {-1, 0.2, 0, nss}, {100, -0.2, 0, nss}, {100, 0.2, math.NaN(), nss}, {100, 0.2, 0, nil}, {math.Inf(1), 0.2, 0, nss}} {
		if _, err := stock.New(c.s0, c.sigma, c.q, c.c); err == nil {
			t.Errorf("New(%v, %v, %v, %v): expected error", c.s0, c.sigma, c.q, c.c)
		}
	}
	m := mustGBM(t, 100, 0.2, 0, nss)
	g := mustGrid(t, 1, 4)
	if _, err := m.GeometricAsian(0, 100, g); err == nil {
		t.Error("invalid option type: expected error")
	}
	if _, err := m.GeometricAsian(option.Call, 0, g); err == nil {
		t.Error("zero strike: expected error")
	}
	if _, err := m.GeometricAsian(option.Call, 100, nil); err == nil {
		t.Error("nil grid: expected error")
	}
	if _, err := m.GeometricAsianControl(option.Call, 100, &mc.Grid{Times: []float64{0, 1, 0.5}}); err == nil {
		t.Error("invalid grid: expected error")
	}
}

func TestZeroAllocs(t *testing.T) {
	m := mustGBM(t, 100, 0.2, 0, nss)
	g := mustGrid(t, 1, 50)
	sim, err := m.Discretize(g)
	if err != nil {
		t.Fatal(err)
	}
	p := mc.NewPath(g, sim.Factors())
	z := make([]float64, sim.Dim())
	pay := stock.Asian{Type: option.Call, Strike: 100}
	if a := testing.AllocsPerRun(100, func() { sim.Simulate(z, p); _ = pay.Value(p) }); a != 0 {
		t.Errorf("%v allocations per path", a)
	}
}
