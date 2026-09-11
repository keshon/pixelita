package resize

import (
	"image"
	"image/color"
	"testing"
)

func fill(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

// Averaging a flat colour must give that colour back, whatever the filter.
func TestFlatColourSurvives(t *testing.T) {
	want := color.NRGBA{37, 140, 219, 255}
	src := fill(64, 64, want)
	for _, f := range []Filter{Nearest, Box, Triangle, CatmullRom, Lanczos3} {
		for _, size := range []int{7, 63, 64, 65, 200} {
			got := Resize(src, size, size, f).NRGBAAt(size/2, size/2)
			if got != want {
				t.Errorf("%s to %d: got %v, want %v", f.Name, size, got, want)
			}
		}
	}
}

// The gamma test. Half the pixels black and half white is half the light, and
// half the light is 188 in sRGB, not 128. A resampler working in sRGB values
// gives 128 here and darkens every image it touches.
func TestDownscaleIsGammaCorrect(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			v := uint8(0)
			if (x+y)%2 == 0 {
				v = 255
			}
			src.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
		}
	}

	got := Resize(src, 8, 8, Box).NRGBAAt(4, 4)
	if got.R < 186 || got.R > 190 {
		t.Errorf("got %d, want about 188 — anything near 128 means the "+
			"average was taken in sRGB instead of linear light", got.R)
	}
}

// Colour must not leak out of fully transparent pixels.
func TestTransparentColourDoesNotBleed(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			if x < 32 {
				src.SetNRGBA(x, y, color.NRGBA{255, 0, 0, 255})
			} else {
				src.SetNRGBA(x, y, color.NRGBA{0, 255, 0, 0}) // invisible green
			}
		}
	}

	out := Resize(src, 32, 32, Triangle)
	for x := 0; x < 32; x++ {
		p := out.NRGBAAt(x, 16)
		if p.A > 0 && p.G > 8 {
			t.Fatalf("x=%d: %v — green from transparent pixels leaked in", x, p)
		}
	}
}

func TestFit(t *testing.T) {
	cases := []struct {
		sw, sh, w, h int
		mode         string
		wantW, wantH int
	}{
		{1000, 500, 400, 400, "inside", 400, 200},
		{1000, 500, 400, 400, "outside", 800, 400},
		{1000, 500, 400, 400, "exact", 400, 400},
		{1000, 500, 400, 0, "inside", 400, 200},
		{1000, 500, 0, 250, "inside", 500, 250},
		{1000, 500, 0, 0, "inside", 1000, 500},
	}
	for _, c := range cases {
		w, h := Fit(c.sw, c.sh, c.w, c.h, c.mode)
		if w != c.wantW || h != c.wantH {
			t.Errorf("Fit(%d,%d,%d,%d,%q) = %d,%d, want %d,%d",
				c.sw, c.sh, c.w, c.h, c.mode, w, h, c.wantW, c.wantH)
		}
	}
}
