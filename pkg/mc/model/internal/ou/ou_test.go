package ou

import (
	"math"
	"testing"

	"github.com/konimarti/fixedincome/pkg/mc"
)

func TestB(t *testing.T) {
	if B(0, 2) != 2 || math.Abs(B(1e-12, 2)-2) > 1e-11 {
		t.Errorf("B(0,2)=%v, B(1e-12,2)=%v", B(0, 2), B(1e-12, 2))
	}
	if got, want := B(0.5, 3), (1-math.Exp(-1.5))/0.5; math.Abs(got-want) > 1e-15 {
		t.Errorf("B=%v, want %v", got, want)
	}
}

// TestG compares the series with the closed form where the closed form is
// still accurate, and both branches at the switch point.
func TestG(t *testing.T) {
	closed := func(x float64) float64 { return x + 2*math.Expm1(-x) - 0.5*math.Expm1(-2*x) }
	for _, x := range []float64{0.2, 0.35, 0.5, 0.500001} {
		if got, want := g(x), closed(x); math.Abs(got-want) > 1e-12*want {
			t.Errorf("g(%v)=%v, closed form %v", x, got, want)
		}
	}
	for _, x := range []float64{1e-6, 1e-3} {
		want := x * x * x * (1.0/3 - x/4 + 7*x*x/60)
		if got := g(x); math.Abs(got-want) > 1e-9*want {
			t.Errorf("g(%v)=%v, want %v", x, got, want)
		}
	}
	// VarIntegral is continuous across the branches and in a -> 0
	s, tt := 0.01, 10.0
	for _, a := range []float64{0, 1e-12, 1e-9, 0.9e-9, 1.1e-9, 0.04999, 0.05001, 1} {
		v := VarIntegral(a, s, tt)
		x := a * tt
		// series reference for small a (the closed form cancels)
		ref := s * s * tt * tt * tt * (1.0/3 - x/4 + 7*x*x/60)
		if x > 0.1 {
			ref = s * s / (a * a) * (tt - 2*B(a, tt) + B(2*a, tt))
		}
		if math.Abs(v-ref) > 1e-6*ref {
			t.Errorf("VarIntegral(a=%v)=%v, reference %v", a, v, ref)
		}
	}
}

// TestSimLinearMap: Simulate is affine in z, r = r(0) + A z and
// ln Df = ln Df(0) + C z, so the exact means and covariances of the
// simulated (r_i, ln Df_i) follow from the zero vector and the unit
// vectors. They are compared with the analytic moments of the OU process
// on a non-uniform grid:
//
//	E[r_i] = φ(t_i), E[ln Df_i] = -Φ(t_i)
//	Cov(x_i, x_j) = e^{-a(t_j-t_i)} σ² B(2a,t_i)                       (i <= j)
//	Cov(x_i, Y_j) = Cov(x_i,Y_i) + Var(x_i) B(a,t_j-t_i)                (i <= j)
//	Cov(x_i, Y_j) = e^{-a(t_i-t_j)} Cov(x_j,Y_j), Cov(x_t,Y_t) = σ²B(a,t)²/2 (i > j)
//	Var Y_i = VarIntegral(a, σ, t_i),  E[Df_i] = exp(-Φ(t_i) + Var Y_i/2)
func TestSimLinearMap(t *testing.T) {
	g, err := mc.NewGrid([]float64{0, 0.1, 0.35, 1, 2.5, 2.6, 7, 10})
	if err != nil {
		t.Fatal(err)
	}
	phi := func(t float64) float64 { return 0.02 + 0.01*t }
	Phi := func(t float64) float64 { return 0.02*t + 0.005*t*t }
	n := g.Steps()
	for _, a := range []float64{0, 1e-7, 0.3, 2} {
		sigma := 0.015
		s := NewSim(g, a, sigma, phi, Phi)
		dim := s.Dim()
		p := mc.NewPath(g, 1)
		z := make([]float64, dim)
		s.Simulate(z, p)
		r0 := append([]float64(nil), p.State[0]...)
		l0 := make([]float64, n+1)
		for i := range l0 {
			l0[i] = math.Log(p.Df[i])
		}
		A := make([][]float64, n+1) // A[i][k] = ∂r_i/∂z_k
		C := make([][]float64, n+1) // C[i][k] = ∂ln Df_i/∂z_k
		for i := range A {
			A[i] = make([]float64, dim)
			C[i] = make([]float64, dim)
		}
		for k := 0; k < dim; k++ {
			z[k] = 1
			s.Simulate(z, p)
			z[k] = 0
			for i := 0; i <= n; i++ {
				A[i][k] = p.State[0][i] - r0[i]
				C[i][k] = math.Log(p.Df[i]) - l0[i]
			}
		}
		cov := func(u, v []float64) float64 {
			c := 0.0
			for k := range u {
				c += u[k] * v[k]
			}
			return c
		}
		vx := func(t float64) float64 { return sigma * sigma * B(2*a, t) }
		cxy := func(t float64) float64 { return 0.5 * sigma * sigma * B(a, t) * B(a, t) }
		// errors relative to sqrt(Var·Var) (correlation scale): the
		// unit-vector differences carry absolute rounding errors of order
		// 1e-17, which matter for covariances that decay like e^{-20}
		rel := func(got, want, scale float64) float64 {
			return math.Abs(got-want) / math.Max(scale, 1e-30)
		}
		for i := 0; i <= n; i++ {
			ti := g.Times[i]
			if d := math.Abs(r0[i] - phi(ti)); d > 1e-15 {
				t.Errorf("a=%v: E[r_%d] off by %v", a, i, d)
			}
			if d := math.Abs(l0[i] + Phi(ti)); d > 1e-15 {
				t.Errorf("a=%v: E[ln Df_%d] off by %v", a, i, d)
			}
			vy := VarIntegral(a, sigma, ti)
			if e := rel(cov(C[i], C[i]), vy, vy); e > 1e-12 {
				t.Errorf("a=%v: Var ln Df_%d rel. error %v", a, i, e)
			}
			if got, want := math.Exp(l0[i]+0.5*cov(C[i], C[i])), math.Exp(-Phi(ti)+0.5*vy); math.Abs(got/want-1) > 1e-12 {
				t.Errorf("a=%v: E[Df_%d] %v vs %v", a, i, got, want)
			}
			for j := 0; j <= n; j++ {
				tj := g.Times[j]
				var wantRR, wantXY float64
				if i <= j {
					wantRR = math.Exp(-a*(tj-ti)) * vx(ti)
					wantXY = cxy(ti) + vx(ti)*B(a, tj-ti)
				} else {
					wantRR = math.Exp(-a*(ti-tj)) * vx(tj)
					wantXY = math.Exp(-a*(ti-tj)) * cxy(tj)
				}
				if e := rel(cov(A[i], A[j]), wantRR, math.Sqrt(vx(ti)*vx(tj))); e > 1e-12 {
					t.Errorf("a=%v: Cov(r_%d, r_%d) rel. error %v", a, i, j, e)
				}
				// ln Df_j = -Φ_j - Y_j
				if e := rel(-cov(A[i], C[j]), wantXY, math.Sqrt(vx(ti)*VarIntegral(a, sigma, tj))); e > 1e-12 {
					t.Errorf("a=%v: Cov(r_%d, ln Df_%d) rel. error %v", a, i, j, e)
				}
			}
		}
	}
}
