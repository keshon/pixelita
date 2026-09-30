package ops

import (
	"fmt"
	"math"
	"os"
	"path/filepath"

	webp "github.com/mayahiro/go-webp"

	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/report"
)

type WebPOptions struct {
	Quality      int
	Mode         string // lossy, lossless or near-lossless
	SkipPalette  bool
	MinGain      float64
	MinPSNR      float64
	MinSSIM      float64
	DryRun       bool
	KeepOriginal bool
	DeleteSource bool
	Overwrite    bool
	Output       string
	// OutDir is the same flag img-resize has. Its absence here cost a reader
	// thirty-four megabytes of copying to get a result into a scratch
	// directory: a set of tools whose flags differ between them makes people
	// guess, and guessing costs more than the flag does.
	OutDir string
}

func DefaultWebP() WebPOptions {
	return WebPOptions{Quality: 90, Mode: "lossy", SkipPalette: true, MinGain: 10, MinPSNR: 30, KeepOriginal: true}
}

func (o WebPOptions) OutputPath(path string) string {
	out := sibling(path, "", ".webp")
	if o.OutDir != "" {
		out = filepath.Join(o.OutDir, filepath.Base(out))
	}
	return out
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
	evaluated, err := EvaluateWebPCandidate(img, int64(len(raw)), enc, CandidatePolicy{})
	if err != nil {
		return nil, int64(len(raw)), "encode error", err
	}
	return evaluated.Encoded, int64(len(raw)), "", nil
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
			return skip(item, "palette_png_skipped", "palette")
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
	evaluated, err := EvaluateWebPCandidate(img, item.BytesBefore, enc,
		CandidatePolicy{MinGain: o.MinGain, MinPSNR: o.MinPSNR, MinSSIM: o.MinSSIM})
	if err != nil {
		return fail(item, err, "encode error")
	}

	item.BytesAfter = evaluated.Candidate.BytesAfter
	item.GainPercent = gain(item.BytesBefore, item.BytesAfter)
	if !math.IsInf(evaluated.Candidate.PSNR, 1) {
		item.Metrics["psnr"] = evaluated.Candidate.PSNR
	}
	item.Metrics["ssim"] = evaluated.Candidate.SSIM
	if evaluated.Verdict.Code == "fidelity_below_minimum" {
		return skip(item, evaluated.Verdict.Code, "candidate below fidelity threshold")
	}
	if evaluated.Verdict.Code == "gain_below_minimum" {
		return skip(item, "gain_below_minimum", "gain "+percent(item.GainPercent))
	}

	item.Output = o.OutputPath(path)
	if o.Output != "" {
		item.Output = o.Output
	}
	if o.DryRun {
		item.Status = report.StatusWould
		return item
	}
	if err := AtomicWrite(item.Output, evaluated.Encoded, o.Overwrite, func(data []byte) error {
		_, _, err := imgio.Decode(data)
		return err
	}); err != nil {
		return failCode(item, err, "write_failed", "write error")
	}
	item.Status = report.StatusDone
	if o.DeleteSource {
		if err := os.Remove(path); err != nil {
			return failCode(item, err, "source_delete_failed", "destination written; source kept")
		}
	}
	return item
}
