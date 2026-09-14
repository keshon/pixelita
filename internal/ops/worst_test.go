package ops

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/keshon/pixelita/internal/imgio"
)

// The contract the whole feature rests on: what -worst prints has to be what
// -crop reads. If these two ever spell a rectangle differently, the output
// stops being something to paste and goes back to being something to retype,
// which is the entire problem it was built to remove.
func TestRegionSpellingRoundTrips(t *testing.T) {
	for _, r := range []image.Rectangle{
		image.Rect(0, 0, 16, 16),
		image.Rect(1280, 2816, 1280+256, 2816+256),
		image.Rect(7, 13, 7+1, 13+1),
	} {
		s := Rect(r)
		got, err := ParseRect(s)
		if err != nil {
			t.Fatalf("Rect produced %q, which ParseRect rejects: %v", s, err)
		}
		if got != r {
			t.Errorf("%v -> %q -> %v", r, s, got)
		}
	}
}

// A search that cannot find damage deliberately placed in front of it is not a
// search. The rest of the image is identical, so there is exactly one right
// answer and no room for the ranking to be accidentally correct.
func TestWorstRegionsFindsTheDamage(t *testing.T) {
	dir := t.TempDir()
	const size = 512

	a := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			// Texture everywhere, so SSIM has structure to lose.
			v := uint8((x*7 + y*13) % 200)
			a.SetNRGBA(x, y, color.NRGBA{v, uint8(200 - int(v)/2), 128, 255})
		}
	}

	b := image.NewNRGBA(a.Rect)
	copy(b.Pix, a.Pix)
	// Flatten one 128x128 block: the texture there is gone entirely.
	for y := 256; y < 384; y++ {
		for x := 128; x < 256; x++ {
			b.SetNRGBA(x, y, color.NRGBA{100, 150, 128, 255})
		}
	}

	write := func(name string, img *image.NRGBA) string {
		p := filepath.Join(dir, name)
		data, err := imgio.EncodePNG(img)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	pa, pb := write("a.png", a), write("b.png", b)

	items, err := WorstRegions(pa, pb, 3, 128, RankSSIM, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d regions, want 3", len(items))
	}

	top := items[0].Str("crop")
	r, err := ParseRect(top)
	if err != nil {
		t.Fatalf("the worst region %q does not parse as a crop: %v", top, err)
	}
	damaged := image.Rect(128, 256, 256, 384)
	if !r.Overlaps(damaged) {
		t.Errorf("worst region is %v, but the damage is at %v", r, damaged)
	}

	// And it has to be ranked worst, not merely present.
	first, _ := items[0].Num("ssim")
	second, _ := items[1].Num("ssim")
	if first >= second {
		t.Errorf("regions are not sorted worst first: %.3f then %.3f", first, second)
	}
	if first > 0.9 {
		t.Errorf("the flattened block scored %.3f — the ranking is not seeing it", first)
	}

	// Levels travel with the fidelity figures so the table needs one call, not
	// two binaries for the same rectangle.
	before, ok1 := items[0].Metrics["levelsBefore"].([]int)
	after, ok2 := items[0].Metrics["levels"].([]int)
	if !ok1 || !ok2 {
		t.Fatalf("levels missing: %v %v", items[0].Metrics["levelsBefore"], items[0].Metrics["levels"])
	}
	if after[0] >= before[0] {
		t.Errorf("levels %v to %v — flattening a block must cost tonal levels", before, after)
	}
}

// Ranking by tonal headroom has to point somewhere different from ranking by
// structure, or the flag is decoration.
func TestRankingByLevelsFindsTheHeadroomLoss(t *testing.T) {
	dir := t.TempDir()
	const size = 384

	a := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			v := uint8((x * 2 % 256))
			a.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
		}
	}
	b := image.NewNRGBA(a.Rect)
	copy(b.Pix, a.Pix)
	// One block posterised to four levels: little structural change, a large
	// loss of headroom.
	for y := 128; y < 256; y++ {
		for x := 0; x < 128; x++ {
			c := b.NRGBAAt(x, y)
			q := uint8(int(c.R) / 64 * 64)
			b.SetNRGBA(x, y, color.NRGBA{q, q, q, 255})
		}
	}

	write := func(name string, img *image.NRGBA) string {
		p := filepath.Join(dir, name)
		data, err := imgio.EncodePNG(img)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	pa, pb := write("a.png", a), write("b.png", b)

	items, err := WorstRegions(pa, pb, 1, 128, RankLevels, true)
	if err != nil {
		t.Fatal(err)
	}
	r, err := ParseRect(items[0].Str("crop"))
	if err != nil {
		t.Fatal(err)
	}
	if !r.Overlaps(image.Rect(0, 128, 128, 256)) {
		t.Errorf("ranked %v worst, but the headroom was taken from 0,128,128,128", r)
	}
	before := items[0].Metrics["levelsBefore"].([]int)
	after := items[0].Metrics["levels"].([]int)
	if after[0] >= before[0] {
		t.Errorf("levels %v to %v — nothing was lost where the loss was placed", before, after)
	}
}

// A set of rectangles is the unit of the work, so the plural has to read what
// the singular writes.
func TestParseRectsReadsAList(t *testing.T) {
	got, err := ParseRects("0,0,16,16 100,200,32,48;7,7,1,1")
	if err != nil {
		t.Fatal(err)
	}
	want := []image.Rectangle{
		image.Rect(0, 0, 16, 16),
		image.Rect(100, 200, 132, 248),
		image.Rect(7, 7, 8, 8),
	}
	if len(got) != len(want) {
		t.Fatalf("got %d rectangles, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d: got %v, want %v", i, got[i], want[i])
		}
	}
}
