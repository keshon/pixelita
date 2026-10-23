package quant

import (
	"image"
	"image/png"
	"os"
	"slices"
	"testing"
)

func benchmarkImage(b *testing.B) image.Image {
	b.Helper()
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

	return src
}

// BenchmarkQuantizeFile excludes file decoding and PNG encoding.
func BenchmarkQuantizeFile(b *testing.B) {
	src := benchmarkImage(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Quantize(src, Options{MaxColors: 256, Dither: 1, Effort: 6})
	}
}

// BenchmarkPhases reports where the time actually goes.
func BenchmarkPhases(b *testing.B) {
	src := benchmarkImage(b)
	img := toNRGBA(src)
	items, _ := buildHistogram(img)
	ordered := slices.Clone(items)
	pal := medianCut(ordered, 256)

	b.Run("toNRGBA", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			toNRGBA(src)
		}
	})
	b.Run("histogram", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			buildHistogram(img)
		}
	})
	b.Logf("histogram entries: %d", len(items))
	b.Run("mediancut", func(b *testing.B) {
		work := make([]histItem, len(items))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Each iteration starts in histogram order, as the real pipeline does.
			b.StopTimer()
			copy(work, items)
			b.StartTimer()
			medianCut(work, 256)
		}
	})
	b.Run("refine", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			refine(ordered, pal, 12)
		}
	})
	pal = refine(ordered, pal, 12)
	b.Run("remap", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			remap(img, pal, 1)
		}
	})
}
