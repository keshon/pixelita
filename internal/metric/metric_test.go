package metric

import (
	"image"
	"image/color"
	"testing"
)

// P95 and P99 exist to separate one stray pixel from a shift across the frame.
// MaxDelta alone cannot: it reports the same 150 whether one pixel is wrong or
// a tenth of the picture is, and those call for opposite answers.
func TestPercentilesIgnoreALoneOutlier(t *testing.T) {
	const w, h = 100, 100
	a := image.NewNRGBA(image.Rect(0, 0, w, h))
	b := image.NewNRGBA(a.Rect)
	for i := 0; i < len(a.Pix); i += 4 {
		a.Pix[i], a.Pix[i+1], a.Pix[i+2], a.Pix[i+3] = 100, 100, 100, 255
		b.Pix[i], b.Pix[i+1], b.Pix[i+2], b.Pix[i+3] = 102, 102, 102, 255
	}
	b.SetNRGBA(50, 50, color.NRGBA{250, 250, 250, 255}) // the one outlier

	res, err := Compare(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if res.MaxDelta != 150 {
		t.Errorf("MaxDelta = %d, want 150 — the outlier must still be reported", res.MaxDelta)
	}
	if res.P95 != 2 || res.P99 != 2 {
		t.Errorf("p95/p99 = %d/%d, want 2/2 — one pixel in ten thousand moved them",
			res.P95, res.P99)
	}
}

// The mirror case: when the error really is everywhere, the percentiles have to
// say so rather than staying low.
func TestPercentilesFollowAWidespreadShift(t *testing.T) {
	const w, h = 100, 100
	a := image.NewNRGBA(image.Rect(0, 0, w, h))
	b := image.NewNRGBA(a.Rect)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a.SetNRGBA(x, y, color.NRGBA{100, 100, 100, 255})
			v := uint8(102)
			if y >= 90 { // a tenth of the frame, badly wrong
				v = 160
			}
			b.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
		}
	}

	res, err := Compare(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if res.P95 != 60 {
		t.Errorf("p95 = %d, want 60 — a tenth of the frame is above the 95th percentile", res.P95)
	}
	if res.P99 != 60 {
		t.Errorf("p99 = %d, want 60", res.P99)
	}
}
