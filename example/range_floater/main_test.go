package main

import (
	"math"
	"os"
	"strconv"
	"testing"

	"github.com/konimarti/fixedincome/pkg/mc"
)

// TestRangeFloater checks that the Veronesi variant agrees with the value
// of the book (99.8385 ± 0.0505) and the other variants with the
// documented 200000-path values within 4 combined standard errors.
func TestRangeFloater(t *testing.T) {
	paths := 20000
	if testing.Short() {
		paths = 4000
	}
	res, err := price(mc.Options{Paths: paths, Seed: seed(2004)})
	if err != nil {
		t.Fatal(err)
	}
	ver, model := res[0], res[3]
	if se := math.Hypot(ver.StdErr, 0.0505); math.Abs(ver.Mean-99.8385) > 4*se {
		t.Errorf("Veronesi variant %v ± %v, book 99.8385 ± 0.0505", ver.Mean, ver.StdErr)
	}
	// references of the other variants (200000 paths, seed 1; documented
	// in main.go): a wrong LIBOR mapping or discounting fails
	refs := []float64{99.8116, 99.5759, 99.6022, 99.6070}
	for i := 1; i < 4; i++ {
		if se := math.Hypot(res[i].StdErr, 0.0085); math.Abs(res[i].Mean-refs[i]) > 4*se {
			t.Errorf("%s: %v ± %v, reference %v", names[i], res[i].Mean, res[i].StdErr, refs[i])
		}
	}
	t.Logf("Veronesi variant %.4f ± %.4f, model variant %.4f ± %.4f", ver.Mean, ver.StdErr, model.Mean, model.StdErr)
}

// seed adds the offset 1000·$MC_SEED_OFFSET to a test seed.
func seed(s uint64) uint64 {
	off, _ := strconv.Atoi(os.Getenv("MC_SEED_OFFSET"))
	return s + 1000*uint64(off)
}
