package solve_test

import (
	"errors"
	"math"
	"testing"

	"github.com/konimarti/fixedincome/internal/solve"
)

func TestBrent(t *testing.T) {
	tests := []struct {
		Name string
		F    func(float64) float64
		A, B float64
		Root float64
	}{
		{"linear", func(x float64) float64 { return x - 0.3 }, 0, 1, 0.3},
		{"cubic", func(x float64) float64 { return x*x*x - 2 }, 0, 2, math.Cbrt(2)},
		{"exp", func(x float64) float64 { return math.Exp(-x) - 0.5 }, -10, 10, math.Ln2},
		{"cos", math.Cos, 1, 2, math.Pi / 2},
	}
	for _, test := range tests {
		x, err := solve.Brent(test.F, test.A, test.B, 1e-14, 0)
		if err != nil {
			t.Errorf("%s: %v", test.Name, err)
			continue
		}
		if math.Abs(x-test.Root) > 1e-12 {
			t.Errorf("%s: got %v, expected %v", test.Name, x, test.Root)
		}
	}
}

// TestBrent_Endpoints checks that roots exactly at an endpoint are returned
// (and not an uninitialised zero)
func TestBrent_Endpoints(t *testing.T) {
	f := func(x float64) float64 { return x - 20.0 }
	for _, bracket := range [][2]float64{{-20, 20}, {20, 50}} {
		x, err := solve.Brent(f, bracket[0], bracket[1], 0, 0)
		if err != nil || x != 20.0 {
			t.Errorf("bracket %v: got %v, %v; expected 20", bracket, x, err)
		}
	}
}

func TestBrent_Errors(t *testing.T) {
	if _, err := solve.Brent(func(x float64) float64 { return x*x + 1 }, -1, 1, 0, 0); !errors.Is(err, solve.ErrNotBracketed) {
		t.Errorf("unbracketed: got %v", err)
	}
	if _, err := solve.Brent(func(x float64) float64 { return math.NaN() }, -1, 1, 0, 0); !errors.Is(err, solve.ErrNaN) {
		t.Errorf("NaN: got %v", err)
	}
	nanInside := func(x float64) float64 {
		if math.Abs(x) < 0.9 {
			return math.NaN()
		}
		return x
	}
	if _, err := solve.Brent(nanInside, -1, 1, 0, 0); !errors.Is(err, solve.ErrNaN) {
		t.Errorf("NaN inside: got %v", err)
	}
}

func TestFindRoot(t *testing.T) {
	f := func(x float64) float64 { return x - 300.0 }
	brackets := [][2]float64{{-20, 20}, {-50, 100}, {-99, 500}}
	x, err := solve.FindRoot(f, brackets, 0)
	if err != nil || math.Abs(x-300) > 1e-10 {
		t.Errorf("got %v, %v; expected 300", x, err)
	}
	// endpoint roots of later brackets
	x, err = solve.FindRoot(func(x float64) float64 { return x - 100 }, brackets, 0)
	if err != nil || x != 100 {
		t.Errorf("endpoint: got %v, %v", x, err)
	}
	// NaN intervals are skipped
	g := func(x float64) float64 {
		if x < -60 {
			return math.NaN()
		}
		return x - 50
	}
	x, err = solve.FindRoot(g, [][2]float64{{-99, 100}, {0, 100}}, 0)
	if err != nil || math.Abs(x-50) > 1e-10 {
		t.Errorf("NaN skip: got %v, %v", x, err)
	}
	if _, err := solve.FindRoot(f, [][2]float64{{0, 1}}, 0); !errors.Is(err, solve.ErrNotBracketed) {
		t.Errorf("expected ErrNotBracketed, got %v", err)
	}
}

func TestNewton(t *testing.T) {
	fdf := func(x float64) (float64, float64) { return x*x - 2, 2 * x }
	x, err := solve.Newton(fdf, 1, 0, 10, 1e-14, 0)
	if err != nil || math.Abs(x-math.Sqrt2) > 1e-14 {
		t.Errorf("got %v, %v", x, err)
	}
	// leaves the admissible interval
	if _, err := solve.Newton(fdf, 1e-9, 0, 10, 0, 0); err == nil {
		t.Errorf("expected error when leaving the interval")
	}
}
