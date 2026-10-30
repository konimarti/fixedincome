package fit

import (
	"errors"
	"math"
)

// lmProblem is a nonlinear least-squares problem min 1/2 |r(x)|² with box
// constraints lower <= x <= upper (±Inf for unbounded parameters).
type lmProblem struct {
	nx, nr int
	// eval writes the residuals r(x) to r and, if jac is not nil, the
	// Jacobian dr/dx (row-major, nr x nx) to jac. It returns false if x is
	// not admissible (e.g. the curve is invalid).
	eval         func(x, r, jac []float64) bool
	lower, upper []float64
}

// lmSettings are the termination criteria of the Levenberg-Marquardt solver.
type lmSettings struct {
	maxIter int
	// xtol: relative step size; ftol: relative cost reduction; gtol: max
	// abs gradient component of the cost
	xtol, ftol, gtol float64
}

// lmResult is the outcome of a Levenberg-Marquardt run.
type lmResult struct {
	cost        float64 // 1/2 |r|²
	iterations  int
	evaluations int
	converged   bool
}

var errLMStart = errors.New("fit: the starting point is not admissible")

// lmWork are the work buffers of the solver (reused across runs).
type lmWork struct {
	r, rTrial, jac, jtj, jtr, a, step, xTrial, chol []float64
	free                                            []int
}

func newLMWork(nx, nr int) *lmWork {
	return &lmWork{
		r: make([]float64, nr), rTrial: make([]float64, nr), jac: make([]float64, nr*nx),
		jtj: make([]float64, nx*nx), jtr: make([]float64, nx), a: make([]float64, nx*nx),
		step: make([]float64, nx), xTrial: make([]float64, nx), chol: make([]float64, nx*nx),
		free: make([]int, 0, nx),
	}
}

func sumSq(r []float64) float64 {
	s := 0.0
	for _, v := range r {
		s += v * v
	}
	return s
}

func clampBox(x, lower, upper []float64) {
	for i := range x {
		if lower != nil && x[i] < lower[i] {
			x[i] = lower[i]
		}
		if upper != nil && x[i] > upper[i] {
			x[i] = upper[i]
		}
	}
}

// levenbergMarquardt minimises 1/2 |r(x)|² starting from x (modified in
// place) with a bound-constrained Levenberg-Marquardt method (Marquardt
// scaling of the damping, Nielsen's update of the damping parameter).
//
// Bounds are handled with an active set: in each iteration the variables
// at a bound whose cost gradient points out of the box (and the variables
// with lower == upper) are held fixed and the damped normal equations are
// solved for the free variables only (reduced system). A trial point that
// still leaves the box is projected onto it; such a clipped step is never
// taken as convergence by the step-size or cost-reduction criteria, only the
// projected gradient (KKT) criterion applies. The method is deterministic.
func levenbergMarquardt(p lmProblem, x []float64, s lmSettings, w *lmWork) (lmResult, error) {
	nx, nr := p.nx, p.nr
	if w == nil {
		w = newLMWork(nx, nr)
	}
	clampBox(x, p.lower, p.upper)
	res := lmResult{}
	res.evaluations++
	if !p.eval(x, w.r, w.jac) {
		return res, errLMStart
	}
	cost := 0.5 * sumSq(w.r)
	res.cost = cost
	mu, nu := 1e-3, 2.0
	free := w.free[:0]
	for res.iterations < s.maxIter {
		res.iterations++
		// normal equations J'J and gradient J'r
		for i := 0; i < nx; i++ {
			g := 0.0
			for k := 0; k < nr; k++ {
				g += w.jac[k*nx+i] * w.r[k]
			}
			w.jtr[i] = g
			for j := 0; j <= i; j++ {
				v := 0.0
				for k := 0; k < nr; k++ {
					v += w.jac[k*nx+i] * w.jac[k*nx+j]
				}
				w.jtj[i*nx+j], w.jtj[j*nx+i] = v, v
			}
		}
		// active set: variables at a bound with the descent direction -g
		// pointing outward are fixed
		free = free[:0]
		gmax, dmax := 0.0, 0.0
		for i := 0; i < nx; i++ {
			gi := w.jtr[i]
			atLower := p.lower != nil && x[i] <= p.lower[i]
			atUpper := p.upper != nil && x[i] >= p.upper[i]
			if (atLower && atUpper) || (atLower && gi > 0) || (atUpper && gi < 0) {
				continue
			}
			free = append(free, i)
			gmax = math.Max(gmax, math.Abs(gi))
			dmax = math.Max(dmax, w.jtj[i*nx+i])
		}
		if len(free) == 0 || gmax <= s.gtol || cost == 0 {
			res.converged = true
			break
		}
		nf := len(free)
		xnorm := 0.0
		for _, v := range x {
			xnorm += v * v
		}
		xnorm = math.Sqrt(xnorm)
		accepted := false
		for inner := 0; inner < 60 && !accepted; inner++ {
			// reduced system (J_f'J_f + mu*D_f) step_f = -J_f'r with
			// D = diag(J'J) (floored)
			for a, i := range free {
				for b, j := range free {
					w.a[a*nf+b] = w.jtj[i*nx+j]
				}
				w.a[a*nf+a] += mu * math.Max(w.jtj[i*nx+i], 1e-12*dmax+1e-300)
				w.step[a] = -w.jtr[i]
			}
			if !cholSolve(w.a[:nf*nf], w.step[:nf], nf, w.chol[:nf*nf]) {
				mu *= nu
				nu *= 2
				continue
			}
			snorm := 0.0
			for a := 0; a < nf; a++ {
				snorm += w.step[a] * w.step[a]
			}
			snorm = math.Sqrt(snorm)
			if snorm <= s.xtol*(xnorm+s.xtol) {
				// the full (unclipped) step is negligible
				res.converged = true
				res.cost = cost
				return res, nil
			}
			copy(w.xTrial, x)
			for a, i := range free {
				w.xTrial[i] += w.step[a]
			}
			clampBox(w.xTrial, p.lower, p.upper)
			clipped := false
			for a, i := range free {
				if w.xTrial[i] != x[i]+w.step[a] {
					clipped = true
					break
				}
			}
			res.evaluations++
			if !p.eval(w.xTrial, w.rTrial, nil) {
				mu *= nu
				nu *= 2
				continue
			}
			trial := 0.5 * sumSq(w.rTrial)
			if trial < cost && !math.IsNaN(trial) {
				// predicted reduction of the linear model for the
				// unclipped step: 1/2 s'(mu D s - g)
				pred := 0.0
				for a, i := range free {
					d := math.Max(w.jtj[i*nx+i], 1e-12*dmax+1e-300)
					pred += 0.5 * w.step[a] * (mu*d*w.step[a] - w.jtr[i])
				}
				rho := (cost - trial) / pred
				reduction := (cost - trial) / cost
				copy(x, w.xTrial)
				res.evaluations++
				p.eval(x, w.r, w.jac)
				cost = 0.5 * sumSq(w.r)
				accepted = true
				switch {
				case clipped:
					// the model of the clipped step is unreliable: keep mu
				case pred > 0 && !math.IsNaN(rho):
					mu *= math.Max(1.0/3.0, 1-math.Pow(2*rho-1, 3))
				default:
					mu /= 3
				}
				nu = 2
				if reduction < s.ftol && !clipped {
					res.converged = true
					res.cost = cost
					return res, nil
				}
			} else {
				mu *= nu
				nu *= 2
			}
			if mu > 1e30 {
				// no descent possible within rounding: local minimum
				res.converged = true
				res.cost = cost
				return res, nil
			}
		}
		if !accepted {
			break
		}
	}
	res.cost = cost
	return res, nil
}

// cholSolve solves the symmetric positive definite system a x = b in place
// (b is overwritten with x) using the work buffer l (n*n). It returns false
// if a is not positive definite.
func cholSolve(a, b []float64, n int, l []float64) bool {
	for i := 0; i < n; i++ {
		for j := 0; j <= i; j++ {
			s := a[i*n+j]
			for k := 0; k < j; k++ {
				s -= l[i*n+k] * l[j*n+k]
			}
			if i == j {
				if !(s > 0) {
					return false
				}
				l[i*n+i] = math.Sqrt(s)
			} else {
				l[i*n+j] = s / l[j*n+j]
			}
		}
	}
	for i := 0; i < n; i++ {
		s := b[i]
		for k := 0; k < i; k++ {
			s -= l[i*n+k] * b[k]
		}
		b[i] = s / l[i*n+i]
	}
	for i := n - 1; i >= 0; i-- {
		s := b[i]
		for k := i + 1; k < n; k++ {
			s -= l[k*n+i] * b[k]
		}
		b[i] = s / l[i*n+i]
	}
	return true
}

// invertSPD returns the inverse of the symmetric positive definite matrix
// a (n x n, row-major), or false if it is not positive definite.
func invertSPD(a []float64, n int) ([]float64, bool) {
	inv := make([]float64, n*n)
	l := make([]float64, n*n)
	col := make([]float64, n)
	for j := 0; j < n; j++ {
		for i := range col {
			col[i] = 0
		}
		col[j] = 1
		if !cholSolve(a, col, n, l) {
			return nil, false
		}
		for i := 0; i < n; i++ {
			inv[i*n+j] = col[i]
		}
	}
	return inv, true
}
