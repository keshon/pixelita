package ops

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/report"
)

func writePNG(t *testing.T, dir, name string, w, h int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x % 251), uint8(y % 241), uint8((x + y) % 239), 255})
		}
	}
	data, err := imgio.EncodePNG(img)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// The scan reported, correctly, what every codec would save on a folder and
// said nothing about the files being twice as wide as anything would display
// them — which turned out to be the larger of the two wins. Dimensions are a
// lever and the tool has to say so.
func TestScanReportsWidthsWorthServing(t *testing.T) {
	dir := t.TempDir()
	wide := writePNG(t, dir, "wide.png", 2016, 1134)

	o := DefaultScan()
	item := Scan(wide, o)

	if _, ok := item.Num("widthOver"); !ok {
		t.Fatal("a 2016px file was not flagged as wider than the widths asked for")
	}
	at, ok := item.Metrics["atWidth"].(map[string]int64)
	if !ok {
		t.Fatalf("atWidth is %T", item.Metrics["atWidth"])
	}
	for _, w := range []string{"640", "1280", "1920"} {
		if at[w] == 0 {
			t.Errorf("no measurement at %spx", w)
		}
	}
	// Smaller has to mean smaller, or the curve is not a curve.
	if !(at["640"] < at["1280"] && at["1280"] < at["1920"]) {
		t.Errorf("sizes do not fall with width: %v", at)
	}
}

// Upscaling is not a saving, and offering it as one would be the tool inventing
// a number rather than measuring one.
func TestScanDoesNotOfferWidthsAboveTheImage(t *testing.T) {
	dir := t.TempDir()
	small := writePNG(t, dir, "small.png", 800, 600)

	item := Scan(small, DefaultScan())
	if _, ok := item.Num("widthOver"); ok {
		t.Error("an 800px file was flagged as oversize against a 1920px maximum")
	}
	if at, ok := item.Metrics["atWidth"].(map[string]int64); ok {
		if _, offered := at["1280"]; offered {
			t.Error("offered to resize an 800px image up to 1280px")
		}
		if _, offered := at["1920"]; offered {
			t.Error("offered to resize an 800px image up to 1920px")
		}
	}
}

// A per-width total that counted only the wide files would compare different
// sets of files at each width and quietly understate the narrow ones.
func TestWidthTotalsCoverEveryFile(t *testing.T) {
	dir := t.TempDir()
	o := DefaultScan()

	rep := report.New("img-scan", "would be improved", false)
	rep.Add(Scan(writePNG(t, dir, "wide.png", 2016, 1134), o))
	rep.Add(Scan(writePNG(t, dir, "narrow.png", 400, 300), o))
	ScanSummary(rep, o)
	rep.Finish()

	var lines int
	for _, n := range rep.Notes {
		if len(n) > 3 && n[:3] == "at " {
			lines++
		}
	}
	if lines == 0 {
		t.Fatalf("no per-width lines in %v", rep.Notes)
	}
}
