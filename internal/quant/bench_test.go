package quant

import (
	"image"
	"image/png"
	"os"
	"testing"
)

// BenchmarkQuantizeFile runs the whole pipeline over a real image. Point
// QUANT_BENCH_IMAGE at a PNG to use it.
func BenchmarkQuantizeFile(b *testing.B) {
	path := os.Getenv("QUANT_BENCH_IMAGE")
	if path == "" {
		b.Skip("set QUANT_BENCH_IMAGE")
	}
	f, err := os.Open(path)
	if err != nil {
		b.Fatal(err)
	}
	src, err := png.Decode(f)
	f.Close()
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Quantize(src, Options{MaxColors: 256, Dither: 1, Effort: 6})
	}
}

// BenchmarkPhases reports where the time actually goes.
func BenchmarkPhases(b *testing.B) {
	path := os.Getenv("QUANT_BENCH_IMAGE")
	if path == "" {
		b.Skip("set QUANT_BENCH_IMAGE")
	}
	f, _ := os.Open(path)
	src, err := png.Decode(f)
	f.Close()
	if err != nil {
		b.Fatal(err)
	}
	var img *image.NRGBA
	var items []histItem
	var pal []colorf

	b.Run("toNRGBA", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			img = toNRGBA(src)
		}
	})
	b.Run("histogram", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			items, _ = buildHistogram(img)
		}
	})
	b.Logf("histogram entries: %d", len(items))
	b.Run("mediancut", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			pal = medianCut(items, 256)
		}
	})
	b.Run("refine", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			refine(items, pal, 12)
		}
	})
	pal = refine(items, pal, 12)
	b.Run("remap", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			remap(img, pal, 1)
		}
	})
}
