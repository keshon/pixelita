package ops

import (
	"fmt"
	"math"
	"os"

	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/quant"
	"github.com/keshon/pixelita/internal/report"
)

type QuantOptions struct {
	Colors  int
	Dither  float64
	Effort  int
	MinGain float64
	MinPSNR float64
	DryRun  bool
	Replace bool
	Suffix  string
}

func DefaultQuant() QuantOptions {
	return QuantOptions{Colors: 256, Dither: 1, Effort: 6, MinGain: 10, MinPSNR: 30, Suffix: "-min"}
}

// QuantResult is what quantising produced, before anything has been decided
// about whether it is worth keeping.
type QuantResult struct {
	Encoded     []byte
	BytesBefore int64
	Colors      int
	Distinct    int
	PSNR        float64 // +Inf when nothing was lost
}

// QuantBytes quantises a PNG and hands back the result without writing it. The
// web interface uses this to show a candidate before committing to it.
func QuantBytes(path string, o QuantOptions) (QuantResult, string, error) {
	img, _, raw, err := imgio.Load(path)
	if err != nil {
		return QuantResult{}, "decode error", err
	}

	q := quant.Quantize(img, quant.Options{
		MaxColors: o.Colors, Dither: o.Dither, Effort: o.Effort,
	})
	encoded, err := imgio.EncodePalettedPNG(q.Image)
	if err != nil {
		return QuantResult{}, "encode error", err
	}

	return QuantResult{
		Encoded:     encoded,
		BytesBefore: int64(len(raw)),
		Colors:      q.Colors,
		Distinct:    q.Distinct,
		PSNR:        q.PSNR,
	}, "", nil
}

// Quant reduces a PNG to a palette and writes it only when the result is both
// smaller and still faithful.
func Quant(path string, o QuantOptions) report.Item {
	item := report.Item{Path: path, Metrics: map[string]any{}}

	res, reason, err := QuantBytes(path, o)
	if err != nil {
		return fail(item, err, reason)
	}
	item.BytesBefore = res.BytesBefore
	item.BytesAfter = int64(len(res.Encoded))
	item.GainPercent = gain(item.BytesBefore, item.BytesAfter)
	item.Metrics["colors"] = res.Colors
	item.Metrics["distinct"] = res.Distinct
	if !math.IsInf(res.PSNR, 1) {
		item.Metrics["psnr"] = res.PSNR
	}

	if o.MinPSNR > 0 && res.PSNR < o.MinPSNR {
		item.Status = report.StatusSkipped
		item.Reason = fmt.Sprintf("%.0f dB", res.PSNR)
		return item
	}
	if item.GainPercent < o.MinGain {
		item.Status = report.StatusSkipped
		item.Reason = "gain " + percent(item.GainPercent)
		return item
	}

	item.Output = path
	if !o.Replace {
		item.Output = sibling(path, o.Suffix, ".png")
	}
	if o.DryRun {
		item.Status = report.StatusWould
		return item
	}
	if err := os.WriteFile(item.Output, res.Encoded, 0o644); err != nil {
		return fail(item, err, "write error")
	}
	item.Status = report.StatusDone
	return item
}
