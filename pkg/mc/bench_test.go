package mc_test

import (
	"runtime"
	"testing"

	"github.com/konimarti/fixedincome/pkg/instrument/option"
	"github.com/konimarti/fixedincome/pkg/mc"
	"github.com/konimarti/fixedincome/pkg/mc/model/holee"
	"github.com/konimarti/fixedincome/pkg/mc/model/stock"
	"github.com/konimarti/fixedincome/pkg/term"
)

var benchCurve = term.NSS{B0: 0.03, B1: -0.02, B2: 0.01, B3: 0.005, Tau1: 2, Tau2: 8}

// benchPrice runs Price with the given number of paths per operation and
// reports the throughput (paths/s) and the allocations per path.
func benchPrice(b *testing.B, m mc.Model, pay mc.Payoff, g *mc.Grid, paths int) {
	for _, w := range []int{1, runtime.GOMAXPROCS(0)} {
		b.Run("workers="+itoa(w), func(b *testing.B) {
			o := mc.Options{Paths: paths, Workers: w, Seed: 1}
			var ms0, ms1 runtime.MemStats
			runtime.ReadMemStats(&ms0)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := mc.Price(m, pay, g, o); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			runtime.ReadMemStats(&ms1)
			total := float64(b.N) * float64(paths)
			b.ReportMetric(total/b.Elapsed().Seconds(), "paths/s")
			b.ReportMetric(float64(ms1.Mallocs-ms0.Mallocs)/total, "allocs/path")
		})
	}
}

func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return itoa(i/10) + string(rune('0'+i%10))
}

// BenchmarkHoLeeZeroBond: 10y zero bond, 120 steps (240 normals per path).
func BenchmarkHoLeeZeroBond(b *testing.B) {
	m, _ := holee.New(benchCurve, 0.01)
	g, _ := mc.NewUniformGrid(10, 120)
	benchPrice(b, m, mc.PayoffFunc(func(p *mc.Path) float64 { return p.Df[len(p.Df)-1] }), g, 1<<16)
}

// BenchmarkGBMAsian: arithmetic Asian call, 52 weekly steps.
func BenchmarkGBMAsian(b *testing.B) {
	m, _ := stock.New(100, 0.3, 0.02, benchCurve)
	g, _ := mc.NewUniformGrid(1, 52)
	benchPrice(b, m, stock.Asian{Type: option.Call, Strike: 100}, g, 1<<16)
}

// BenchmarkLSM: American put, 50 exercise dates, 2^15 training and 2^15
// pricing paths.
func BenchmarkLSM(b *testing.B) {
	m, _ := stock.New(36, 0.2, 0, term.Flat{Rate: 0.06})
	g, _ := mc.NewUniformGrid(1, 50)
	for _, w := range []int{1, runtime.GOMAXPROCS(0)} {
		b.Run("workers="+itoa(w), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := mc.PriceAmerican(m, stock.American{Type: option.Put, Strike: 40}, g, mc.Options{Paths: 1 << 15, Workers: w, Seed: 1}); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.N)*(1<<16)/b.Elapsed().Seconds(), "paths/s")
		})
	}
}
