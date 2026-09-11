package imgio

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	webp "github.com/mayahiro/go-webp"
)

// A lossy WebP must come back with the range it went in with.
//
// VP8 stores luma between 16 and 235, and reading those planes as if they were
// full-range JPEG values turns white into grey. The mistake is invisible in a
// thumbnail and obvious in a measurement: it costs about fifteen decibels and
// does not move when quality changes, because it is an offset, not a loss.
func TestLossyWebPKeepsItsRange(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			v := uint8(255)
			if x >= 32 {
				v = 0
			}
			src.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
		}
	}

	var buf bytes.Buffer
	err := webp.Encode(&buf, src, &webp.Options{
		Quality: 100, Compression: webp.CompressionLossy, Mode: webp.ModeDefault,
	})
	if err != nil {
		t.Fatal(err)
	}

	got, format, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if format != "webp" {
		t.Errorf("format = %q, want webp", format)
	}

	out := ToNRGBA(got)
	white := out.NRGBAAt(16, 32)
	black := out.NRGBAAt(48, 32)
	if white.R < 250 {
		t.Errorf("white came back as %d, want 255 or close — the planes were "+
			"read as full-range instead of studio-range", white.R)
	}
	if black.R > 5 {
		t.Errorf("black came back as %d, want 0 or close", black.R)
	}
}

func TestReadHeader(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 7, 11))
	raw, err := EncodePNG(src)
	if err != nil {
		t.Fatal(err)
	}
	head, err := ReadHeader(raw)
	if err != nil {
		t.Fatal(err)
	}
	if head.Format != "png" || head.Width != 7 || head.Height != 11 {
		t.Errorf("got %+v, want png 7x11", head)
	}
}
