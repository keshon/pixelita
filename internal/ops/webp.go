package ops

import (
	"bytes"
	"fmt"
	"os"

	webp "github.com/mayahiro/go-webp"

	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/report"
)

type WebPOptions struct {
	Quality      int
	Mode         string // lossy, lossless or near-lossless
	SkipPalette  bool
	MinGain      float64
	DryRun       bool
	KeepOriginal bool
}

func DefaultWebP() WebPOptions {
	return WebPOptions{Quality: 90, Mode: "lossy", SkipPalette: true, MinGain: 10, KeepOriginal: true}
}

// EncoderOptions translates our options into the encoder's.
func (o WebPOptions) EncoderOptions() (*webp.Options, error) {
	out := &webp.Options{Quality: o.Quality}
	switch o.Mode {
	case "lossy":
		out.Compression = webp.CompressionLossy
		out.Mode = webp.ModeDefault
	case "lossless":
		out.Compression = webp.CompressionLossless
		out.Mode = webp.ModeBestCompression
	case "near-lossless":
		out.Compression = webp.CompressionLossless
		out.Mode = webp.ModeNearLossless
	default:
		return nil, fmt.Errorf("unknown mode %q, expected lossy, lossless or near-lossless", o.Mode)
	}
	return out, nil
}

// WebPBytes encodes without writing anything.
func WebPBytes(path string, o WebPOptions) ([]byte, int64, string, error) {
	enc, err := o.EncoderOptions()
	if err != nil {
		return nil, 0, "bad options", err
	}
	img, _, raw, err := imgio.Load(path)
	if err != nil {
		return nil, 0, "decode error", err
	}
	var buf bytes.Buffer
	if err := webp.Encode(&buf, img, enc); err != nil {
		return nil, int64(len(raw)), "encode error", err
	}
	return buf.Bytes(), int64(len(raw)), "", nil
}

// WebP converts an image to WebP, and writes it only when that pays off.
func WebP(path string, o WebPOptions) report.Item {
	item := report.Item{Path: path, Metrics: map[string]any{}}

	source, err := os.ReadFile(path)
	if err != nil {
		return fail(item, err, "read error")
	}
	item.BytesBefore = int64(len(source))

	// The colour type comes from the header, so a palette PNG can be dismissed
	// without paying to decode it.
	if head, err := imgio.ReadHeader(source); err == nil {
		item.Metrics["colourType"] = head.ColourType
		item.Metrics["width"] = head.Width
		item.Metrics["height"] = head.Height
		if o.SkipPalette && head.ColourType == "palette" {
			item.Status = report.StatusSkipped
			item.Reason = "palette"
			return item
		}
	}

	enc, err := o.EncoderOptions()
	if err != nil {
		return fail(item, err, "bad options")
	}
	img, _, err := imgio.Decode(source)
	if err != nil {
		return fail(item, err, "decode error")
	}
	var encoded bytes.Buffer
	if err := webp.Encode(&encoded, img, enc); err != nil {
		return fail(item, err, "encode error")
	}

	item.BytesAfter = int64(encoded.Len())
	item.GainPercent = gain(item.BytesBefore, item.BytesAfter)
	if item.GainPercent < o.MinGain {
		item.Status = report.StatusSkipped
		item.Reason = "gain " + percent(item.GainPercent)
		return item
	}

	item.Output = sibling(path, "", ".webp")
	if o.DryRun {
		item.Status = report.StatusWould
		return item
	}
	if err := os.WriteFile(item.Output, encoded.Bytes(), 0o644); err != nil {
		return fail(item, err, "write error")
	}
	item.Status = report.StatusDone
	if !o.KeepOriginal {
		if err := os.Remove(path); err != nil {
			item.Error = err.Error()
			item.Reason = "original kept"
		}
	}
	return item
}
