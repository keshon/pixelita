package jpegopt

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// decodePixels is the only check that matters: the promise is that nothing
// about the picture changes, so the two files must decode to the same bytes.
func decodePixels(t *testing.T, raw []byte) *image.NRGBA {
	t.Helper()
	img, err := jpeg.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.Set(x, y, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out
}

func sample() []byte {
	src := image.NewNRGBA(image.Rect(0, 0, 233, 149))
	for y := 0; y < 149; y++ {
		for x := 0; x < 233; x++ {
			src.SetNRGBA(x, y, color.NRGBA{
				R: uint8(x*7 + y*3),
				G: uint8(int(40*math.Sin(float64(x)/9)) + 128),
				B: uint8(y * 5),
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	jpeg.Encode(&buf, src, &jpeg.Options{Quality: 82})
	return buf.Bytes()
}

func TestOptimiseKeepsEveryPixel(t *testing.T) {
	raw := sample()
	out, err := Optimise(raw)
	if err != nil {
		t.Fatal(err)
	}

	before, after := decodePixels(t, raw), decodePixels(t, out)
	if !bytes.Equal(before.Pix, after.Pix) {
		t.Fatalf("the picture changed: %d bytes differ", countDiff(before.Pix, after.Pix))
	}
	if len(out) > len(raw) {
		t.Errorf("grew from %d to %d bytes", len(raw), len(out))
	}
	t.Logf("%d -> %d bytes (%.1f%%)", len(raw), len(out),
		(1-float64(len(out))/float64(len(raw)))*100)
}

// Odd sizes exercise the partial blocks at the right and bottom edges, where
// the coefficients live outside the visible image and are easy to lose.
func TestOptimiseAcrossSizes(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {7, 3}, {8, 8}, {9, 17}, {16, 16}, {31, 47}} {
		src := image.NewNRGBA(image.Rect(0, 0, size[0], size[1]))
		for y := 0; y < size[1]; y++ {
			for x := 0; x < size[0]; x++ {
				src.SetNRGBA(x, y, color.NRGBA{uint8(x * 11), uint8(y * 13), uint8(x + y), 255})
			}
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 90}); err != nil {
			t.Fatal(err)
		}

		out, err := Optimise(buf.Bytes())
		if err != nil {
			t.Fatalf("%dx%d: %v", size[0], size[1], err)
		}
		before, after := decodePixels(t, buf.Bytes()), decodePixels(t, out)
		if !bytes.Equal(before.Pix, after.Pix) {
			t.Errorf("%dx%d: the picture changed", size[0], size[1])
		}
	}
}

func TestProgressiveIsLeftAlone(t *testing.T) {
	// Go's encoder writes baseline, so build a progressive marker by hand: the
	// point is that the parser recognises SOF2 and declines rather than
	// producing something wrong.
	raw := []byte{0xFF, markerSOI, 0xFF, markerSOF2, 0x00, 0x0B, 8, 0, 8, 0, 8, 1, 1, 0x11, 0}
	if _, err := Optimise(raw); !errors.Is(err, ErrProgressive) {
		t.Errorf("got %v, want ErrProgressive", err)
	}
}

// BenchmarkCorpus measures real files. Point JPEGOPT_CORPUS at a directory.
func TestCorpus(t *testing.T) {
	dir := os.Getenv("JPEGOPT_CORPUS")
	if dir == "" {
		t.Skip("set JPEGOPT_CORPUS to a directory of JPEGs")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.jpg"))
	more, _ := filepath.Glob(filepath.Join(dir, "*.jpeg"))
	files = append(files, more...)

	var before, after int64
	var done, skipped int
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out, err := Optimise(raw)
		if err != nil {
			skipped++
			t.Logf("%-44s skipped: %v", filepath.Base(f), err)
			continue
		}

		a, b := decodePixels(t, raw), decodePixels(t, out)
		if !bytes.Equal(a.Pix, b.Pix) {
			t.Errorf("%s: the picture changed in %d bytes", filepath.Base(f), countDiff(a.Pix, b.Pix))
			continue
		}
		before += int64(len(raw))
		after += int64(len(out))
		done++
		t.Logf("%-44s %8d -> %8d  %+.1f%%", filepath.Base(f), len(raw), len(out),
			-(1-float64(len(out))/float64(len(raw)))*100)
	}
	if before > 0 {
		t.Logf("TOTAL %d files (%d skipped): %d -> %d bytes, %.2f%% saved",
			done, skipped, before, after, (1-float64(after)/float64(before))*100)
	}
}

func countDiff(a, b []byte) int {
	n := 0
	for i := range a {
		if i < len(b) && a[i] != b[i] {
			n++
		}
	}
	return n
}
