package quant

import (
	"image"
	"image/color"
	"math"
	"math/rand"
	"testing"
)

// The kd-tree is only worth having if it agrees with the obvious answer.
func TestNearestMatchesBruteForce(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, n := range []int{1, 2, 7, 64, 256} {
		pal := make([]colorf, n)
		for i := range pal {
			pal[i] = clampPremul(colorf{rng.Float32(), rng.Float32(), rng.Float32(), rng.Float32()})
		}
		tree := newKDTree(pal)

		for i := 0; i < 5000; i++ {
			q := clampPremul(colorf{rng.Float32(), rng.Float32(), rng.Float32(), rng.Float32()})

			want, wantDist := 0, float32(math.MaxFloat32)
			for j, p := range pal {
				if d := q.diff(p); d < wantDist {
					want, wantDist = j, d
				}
			}
			_, gotDist := tree.nearest(q)
			if gotDist != wantDist {
				t.Fatalf("palette %d, query %v: tree found distance %v, brute force %v (index %d)",
					n, q, gotDist, wantDist, want)
			}
		}
	}
}

// Fewer distinct colours than palette slots must round-trip untouched.
func TestFewColoursAreLossless(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	want := []color.NRGBA{
		{255, 0, 0, 255}, {0, 128, 0, 255}, {0, 0, 255, 128}, {0, 0, 0, 0},
	}
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			src.SetNRGBA(x, y, want[(x/7+y/5)%len(want)])
		}
	}

	res := Quantize(src, Options{MaxColors: 256, Dither: 1, Effort: 5})
	if !math.IsInf(res.PSNR, 1) {
		t.Errorf("PSNR = %.1f dB, want lossless", res.PSNR)
	}
	if res.Colors != len(want) {
		t.Errorf("Colors = %d, want %d", res.Colors, len(want))
	}
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			got := color.NRGBAModel.Convert(res.Image.At(x, y)).(color.NRGBA)
			w := want[(x/7+y/5)%len(want)]
			if w.A == 0 {
				w = color.NRGBA{} // transparent has no colour to preserve
			}
			if got != w {
				t.Fatalf("pixel %d,%d = %v, want %v", x, y, got, w)
			}
		}
	}
}

// A gradient has far more colours than the palette; dithering should still keep
// the average close to the original. pngquant scores 27.5 dB on this exact
// image, so the bar is set just under that: this guards against a regression,
// not against being a shade behind the reference.
func TestGradientStaysClose(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			src.SetNRGBA(x, y, color.NRGBA{uint8(x), uint8(y), uint8((x + y) / 2), 255})
		}
	}
	res := Quantize(src, Options{MaxColors: 64, Dither: 1, Effort: 8})
	if res.PSNR < 26 {
		t.Errorf("PSNR = %.1f dB with 64 colours, expected at least 26", res.PSNR)
	}
}
