package ops

import (
	"image"
	"image/color"
	"testing"

	"github.com/keshon/pixelita/internal/report"
	"github.com/keshon/pixelita/internal/resize"
)

func testImage(w, h int) *image.NRGBA {
	src := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			src.SetNRGBA(x, y, color.NRGBA{uint8(x * 7), uint8(y * 11), uint8(x + y), 255})
		}
	}
	return src
}

// The whole claim of -zoom is that it shows the pixel as it is. A resampler
// that merely looks like it does would be worse than none: it would put a
// plausible picture in front of someone inspecting a suspected rounding bug.
func TestMagnifyIsExact(t *testing.T) {
	src := testImage(9, 7)
	const n = 4
	got := magnify(src, n)

	if got.Rect.Dx() != 9*n || got.Rect.Dy() != 7*n {
		t.Fatalf("got %dx%d, want %dx%d", got.Rect.Dx(), got.Rect.Dy(), 9*n, 7*n)
	}
	for y := 0; y < 7*n; y++ {
		for x := 0; x < 9*n; x++ {
			want := src.NRGBAAt(x/n, y/n)
			if c := got.NRGBAAt(x, y); c != want {
				t.Fatalf("at %d,%d got %v, want %v (source pixel %d,%d)",
					x, y, c, want, x/n, y/n)
			}
		}
	}
}

func TestMagnifyBelowTwoIsIdentity(t *testing.T) {
	src := testImage(4, 4)
	for _, n := range []int{0, 1} {
		if got := magnify(src, n); got != src {
			t.Errorf("magnify(src, %d) copied when it should not have", n)
		}
	}
}

// The version this replaced returned the untouched image when the rectangle
// missed, so a typo in the coordinates was answered with a confident
// measurement of everything. That is the failure worth a test.
func TestCropOutsideImageIsAnError(t *testing.T) {
	src := testImage(20, 20)

	if _, err := cropTo(src, image.Rect(50, 50, 60, 60)); err == nil {
		t.Fatal("a rectangle wholly outside the image was accepted")
	}

	got, err := cropTo(src, image.Rect(15, 15, 35, 35))
	if err != nil {
		t.Fatalf("a partly overlapping rectangle should clip, not fail: %v", err)
	}
	if got.Rect.Dx() != 5 || got.Rect.Dy() != 5 {
		t.Errorf("clipped to %dx%d, want 5x5", got.Rect.Dx(), got.Rect.Dy())
	}
	if c, want := got.NRGBAAt(0, 0), src.NRGBAAt(15, 15); c != want {
		t.Errorf("the crop starts at %v, want %v", c, want)
	}
}

func TestStatsIgnoreFullyTransparentPixels(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 4, 1))
	src.SetNRGBA(0, 0, color.NRGBA{100, 100, 100, 255})
	src.SetNRGBA(1, 0, color.NRGBA{200, 200, 200, 255})
	// Two pixels nothing can see, carrying a colour that would wreck the mean.
	src.SetNRGBA(2, 0, color.NRGBA{0, 0, 255, 0})
	src.SetNRGBA(3, 0, color.NRGBA{255, 0, 0, 0})

	item := report.Item{Metrics: map[string]any{}}
	addStats(&item, src)

	// Averaged as light, 100 and 200 sit at 158, not at the 150 an arithmetic
	// mean of the code values would give. The number to hold on to is that the
	// two invisible pixels moved neither channel.
	rgb, ok := item.Metrics["meanRGB"].([]int)
	if !ok {
		t.Fatalf("meanRGB is %T", item.Metrics["meanRGB"])
	}
	if rgb[0] != rgb[1] || rgb[1] != rgb[2] {
		t.Errorf("mean is %v, want a neutral grey — an invisible colour leaked in", rgb)
	}
	if rgb[0] <= 150 {
		t.Errorf("mean is %v; averaged in linear light two greys of 100 and 200 "+
			"land above their arithmetic mean", rgb)
	}
	if luma, ok := item.Metrics["luma"].([]int); !ok || luma[0] != 100 || luma[1] != 200 {
		t.Errorf("luma range is %v, want 100..200", item.Metrics["luma"])
	}
}

// The mean has to be the same number the toolkit's own resampler would give for
// the same region. Stated in a comment it is a hope; stated here it is a fact,
// and if either side ever changes this fails rather than quietly disagreeing.
func TestStatsAgreeWithResizingToOnePixel(t *testing.T) {
	src := testImage(37, 23)

	item := report.Item{Metrics: map[string]any{}}
	addStats(&item, src)
	got := item.Metrics["meanRGB"].([]int)

	one := resize.Resize(src, 1, 1, resize.Box)
	c := one.NRGBAAt(0, 0)
	want := []int{int(c.R), int(c.G), int(c.B)}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("channel %d: stats say %d, img-resize to 1x1 says %d", i, got[i], want[i])
		}
	}
}

func TestStatsOfAFullyTransparentRegionSaysSo(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 3, 3))
	item := report.Item{Metrics: map[string]any{}}
	addStats(&item, src)
	if got := item.Metrics["mean"]; got != "fully transparent" {
		t.Errorf("mean is %v, want it to say the region is empty", got)
	}
}
