package ops

import (
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestPreflightDistinguishesDestinationFailures(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.png"), filepath.Join(dir, "b.png")
	out := filepath.Join(dir, "out.png")
	if err := os.WriteFile(a, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := PreflightDestinations([]Destination{{Source: a, Output: out}, {Source: b, Output: out}}, false, false)
	var pe *PlanError
	if !errors.As(err, &pe) || pe.Code != "output_collision" {
		t.Fatalf("collision = %#v", err)
	}

	if err := os.WriteFile(out, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = PreflightDestinations([]Destination{{Source: a, Output: out}}, false, false)
	if !errors.As(err, &pe) || pe.Code != "destination_exists" {
		t.Fatalf("existing = %#v", err)
	}

	err = PreflightDestinations([]Destination{{Source: a, Output: a}}, true, false)
	if !errors.As(err, &pe) || pe.Code != "source_replacement_requires_permission" {
		t.Fatalf("replacement = %#v", err)
	}
}

func TestPreflightRejectsOutputOverlappingAnotherInput(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.png")
	b := filepath.Join(dir, "a-min.png")
	for _, path := range []string{a, b} {
		if err := os.WriteFile(path, []byte(path), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	err := PreflightDestinations([]Destination{
		{Source: a, Output: b},
		{Source: b, Output: filepath.Join(dir, "a-min-min.png")},
	}, true, false)
	var pe *PlanError
	if !errors.As(err, &pe) || pe.Code != "output_overlaps_input" {
		t.Fatalf("overlap = %#v", err)
	}
}

func TestAtomicWriteVerificationFailurePreservesDestination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.bin")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := AtomicWrite(path, []byte("candidate"), true, func([]byte) error { return errors.New("reject") })
	if err == nil {
		t.Fatal("expected verification failure")
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil || string(got) != "original" {
		t.Fatalf("destination changed: %q, %v", got, readErr)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".out.bin.tmp-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary files remain: %v, %v", matches, err)
	}
}

func TestEvaluateCandidateSharedVerdict(t *testing.T) {
	p := CandidatePolicy{MinGain: 10, MinPSNR: 30}
	if got := EvaluateCandidate(Candidate{BytesBefore: 100, BytesAfter: 95}, p); got.Code != "gain_below_minimum" {
		t.Fatalf("gain verdict = %#v", got)
	}
	if got := EvaluateCandidate(Candidate{BytesBefore: 100, BytesAfter: 50, PSNR: 20, HasPSNR: true}, p); got.Code != "fidelity_below_minimum" {
		t.Fatalf("fidelity verdict = %#v", got)
	}
	if got := EvaluateCandidate(Candidate{BytesBefore: 100, BytesAfter: 50, PSNR: 40, HasPSNR: true}, p); !got.Accept {
		t.Fatalf("accepted verdict = %#v", got)
	}
}

func TestScanRecommendationMatchesDirectConverter(t *testing.T) {
	source := filepath.Join(t.TempDir(), "candidate.png")
	img := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	colors := []color.NRGBA{{R: 20, G: 30, B: 40, A: 255}, {R: 230, G: 40, B: 30, A: 255}, {R: 30, G: 200, B: 80, A: 255}, {R: 60, G: 80, B: 220, A: 255}}
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			img.SetNRGBA(x, y, colors[(x/16+y/16)%len(colors)])
		}
	}
	f, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	scan := Scan(source, DefaultScan())
	switch scan.Str("best") {
	case "quant":
		o := DefaultQuant()
		o.DryRun = true
		if got := Quant(source, o); got.Status != "would" {
			t.Fatalf("scan recommended quant but converter returned %#v", got)
		}
	case "webp":
		o := DefaultWebP()
		o.DryRun = true
		if scan.Reason == PaletteCaveat {
			o.SkipPalette = false
		}
		if got := WebP(source, o); got.Status != "would" {
			t.Fatalf("scan recommended webp but converter returned %#v", got)
		}
	default:
		t.Fatalf("fixture produced no recommendation: %#v", scan)
	}
}

func TestWebPDeletesSourceOnlyWhenExplicit(t *testing.T) {
	makeSource := func(name string) string {
		path := filepath.Join(t.TempDir(), name)
		img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
		for y := 0; y < 64; y++ {
			for x := 0; x < 64; x++ {
				img.SetNRGBA(x, y, color.NRGBA{uint8(x * 3), uint8(y * 3), 80, 255})
			}
		}
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		return path
	}

	source := makeSource("keep.png")
	o := DefaultWebP()
	o.SkipPalette, o.MinGain, o.MinPSNR = false, -100, 0
	o.Output = filepath.Join(t.TempDir(), "keep.webp")
	if got := WebP(source, o); got.Status != "done" {
		t.Fatalf("keep conversion = %#v", got)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source removed without permission: %v", err)
	}

	source = makeSource("delete.png")
	o.Output, o.DeleteSource = filepath.Join(t.TempDir(), "delete.webp"), true
	if got := WebP(source, o); got.Status != "done" {
		t.Fatalf("delete conversion = %#v", got)
	}
	if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source was not deleted: %v", err)
	}
}

func TestResizeKeepDoesNotChangeUnsupportedFormat(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	if _, _, err := EncodeAs(img, "webp", DefaultResize(), nil); err == nil {
		t.Fatal("keep silently changed WebP to another format")
	}
}
