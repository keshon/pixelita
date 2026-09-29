package ops

import (
	"image"
	"testing"

	"github.com/keshon/pixelita/internal/report"
)

// Lossy WebP without a fidelity floor ships silent damage: smaller and worse
// with nothing saying so. The converter must refuse below the floor and say
// why, in numbers a dry run shows.
func TestWebPRefusesBelowFloor(t *testing.T) {
	dir := t.TempDir()
	src := writePNG(t, dir, "photo.png", 256, 256)

	strict := DefaultWebP()
	strict.DryRun = true
	strict.MinPSNR = 200 // nothing lossy clears this
	got := WebP(src, strict)
	if got.Status != report.StatusSkipped {
		t.Fatalf("status %q, want skipped: %+v", got.Status, got)
	}
	if _, ok := got.Num("psnr"); !ok {
		t.Error("refusal carries no psnr measurement to show why")
	}

	loose := DefaultWebP()
	loose.DryRun = true
	loose.MinPSNR = 0
	loose.MinGain = -500 // synthetic gradients compress so well PNG wins; accept growth to test the path
	got = WebP(src, loose)
	if got.Status != report.StatusWould {
		t.Fatalf("status %q, want would: %+v", got.Status, got)
	}
}

// Resizing then checking is the normal audit flow, not an error. Different
// dimensions resample b to a and say so; -strict-size restores the failure.
func TestDiffResamplesResizedPair(t *testing.T) {
	dir := t.TempDir()
	big := writePNG(t, dir, "big.png", 128, 128)
	small := writePNG(t, dir, "small.png", 64, 64)

	got := Diff(big, small, DefaultDiff())
	if got.Status == report.StatusFailed {
		t.Fatalf("resized pair failed instead of resampling: %+v", got)
	}
	if got.Str("resampled") == "" {
		t.Error("resampled pair does not say it was resampled")
	}

	strict := DefaultDiff()
	strict.StrictSize = true
	got = Diff(big, small, strict)
	if got.Status != report.StatusFailed {
		t.Fatalf("strict status %q, want failed", got.Status)
	}
}

// -at -json must speak the shared schema: fixed metric keys an agent parses
// without reading prose.
func TestProbeItemsShape(t *testing.T) {
	dir := t.TempDir()
	src := writePNG(t, dir, "a.png", 32, 32)

	items := ProbeItems(src, []image.Point{{X: 0, Y: 0}, {X: 999, Y: 999}})
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	for _, k := range []string{"r", "g", "b", "a", "hex", "x", "y"} {
		if _, ok := items[0].Num(k); !ok && items[0].Str(k) == "" {
			t.Errorf("probe item missing metric %q: %+v", k, items[0].Metrics)
		}
	}
	if items[1].Status != report.StatusFailed {
		t.Error("out-of-bounds point should fail, not guess")
	}
}

// Per-width rollups must exist as data, not just note prose, or agents parse
// sentences for byte counts.
func TestScanTotalsStructured(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "wide.png", 2016, 256)

	rep := report.New("img-scan", "improved", true)
	rep.Add(Scan(dir+"/wide.png", DefaultScan()))
	ScanSummary(rep, DefaultScan())

	at, ok := rep.Totals["atWidth"]
	if !ok {
		t.Fatalf("no totals.atWidth; totals=%v notes=%v", rep.Totals, rep.Notes)
	}
	m, ok := at.(map[string]int64)
	if !ok || m["640"] == 0 {
		t.Fatalf("atWidth has no 640 figure: %v", at)
	}
}
