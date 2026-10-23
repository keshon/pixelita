package ops

import (
	"bytes"
	"errors"
	"github.com/keshon/pixelita/internal/imgio"
	"image"
	"image/color"
	"image/png"
	"math"
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
	if got := EvaluateCandidate(Candidate{BytesBefore: 100, BytesAfter: 95, PSNR: 40, HasPSNR: true}, p); got.Code != "gain_below_minimum" {
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
		uncompressPNG(t, path)
		return path
	}

	source := makeSource("keep.png")
	o := DefaultWebP()
	o.SkipPalette, o.MinGain, o.MinPSNR = false, 0, 0
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

func uncompressPNG(t *testing.T, path string) {
	t.Helper()
	img, _, _, err := imgio.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.NoCompression}
	if err := enc.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCandidateRequiresRequestedEvidence(t *testing.T) {
	for _, c := range []Candidate{
		{BytesBefore: 100, BytesAfter: 50},
		{BytesBefore: 100, BytesAfter: 50, HasPSNR: true, PSNR: math.NaN()},
	} {
		if got := EvaluateCandidate(c, CandidatePolicy{MinPSNR: 30}); got.Accept || got.Code != "fidelity_unavailable" {
			t.Fatalf("accepted missing evidence: %+v", got)
		}
	}
	for _, n := range []float64{math.NaN(), math.Inf(1), -1} {
		if (CandidatePolicy{MinPSNR: n}).Validate() == nil {
			t.Fatalf("accepted %v", n)
		}
	}
}

func TestAtomicWriteDoesNotClobberLateDestination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out")
	err := AtomicWrite(path, []byte("new"), false, func([]byte) error { return os.WriteFile(path, []byte("other writer"), 0600) })
	if err == nil {
		t.Fatal("overwrote concurrent destination")
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "other writer" {
		t.Fatalf("lost destination: %q %v", b, err)
	}
}

func TestQuantThresholdMatchesDiff(t *testing.T) {
	src := writePNG(t, t.TempDir(), "gradient.png", 64, 64)
	uncompressPNG(t, src)
	o := DefaultQuant()
	o.MinGain = 0
	o.MinPSNR = 0
	o.Colors = 4
	got := Quant(src, o)
	if got.Status != "done" {
		t.Fatalf("quant: %+v", got)
	}
	diff := Diff(src, got.Output, DefaultDiff())
	for _, key := range []string{"psnr", "ssim"} {
		a, _ := got.Num(key)
		b, _ := diff.Num(key)
		if a != b {
			t.Fatalf("%s quant=%v diff=%v", key, a, b)
		}
	}
	o.DryRun = true
	o.MinSSIM = 1
	if got := Quant(src, o); got.Code != "fidelity_below_minimum" {
		t.Fatalf("ignored SSIM: %+v", got)
	}
}

func TestAlphaOnlyDamageFailsFidelity(t *testing.T) {
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "a.png"), filepath.Join(dir, "b.png")}
	for i, p := range paths {
		img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				img.SetNRGBA(x, y, color.NRGBA{R: 20, G: 30, B: 40, A: uint8(255 - i*200)})
			}
		}
		raw, err := imgio.EncodePNG(img)
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(p, raw, 0600)
	}
	o := DefaultDiff()
	o.MinPSNR = 40
	if got := Diff(paths[0], paths[1], o); got.Status != "failed" {
		t.Fatalf("alpha damage passed: %+v", got)
	}
}
func TestPreflightRejectsHardlinkInputAlias(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source")
	alias := filepath.Join(dir, "alias")
	if err := os.WriteFile(src, []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(src, alias); err != nil {
		t.Skip(err)
	}
	err := PreflightDestinations([]Destination{{Source: src, Output: alias}}, true, false)
	if err == nil {
		t.Fatal("alias can overwrite input")
	}
}
