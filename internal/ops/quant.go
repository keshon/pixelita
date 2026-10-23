package ops

import (
	"math"
	"path/filepath"
	"slices"

	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/metric"
	"github.com/keshon/pixelita/internal/quant"
	"github.com/keshon/pixelita/internal/result"
)

type QuantOptions struct {
	Colors    int
	Dither    float64
	Effort    int
	MinGain   float64
	MinPSNR   float64
	MinSSIM   float64
	DryRun    bool
	Replace   bool
	Suffix    string
	OutDir    string
	Output    string
	Overwrite bool
}

func DefaultQuant() QuantOptions {
	return QuantOptions{Colors: 256, Dither: 1, Effort: 6, MinGain: 10, MinPSNR: 30, Suffix: "-min"}
}

func (o QuantOptions) OutputPath(path string) string {
	out := path
	if !o.Replace {
		out = sibling(path, o.Suffix, ".png")
	}
	if o.OutDir != "" {
		out = filepath.Join(o.OutDir, filepath.Base(out))
	}
	return out
}

// QuantResult is what quantising produced, before anything has been decided
// about whether it is worth keeping.
type QuantResult struct {
	RemovedChunks []string
	Encoded       []byte
	BytesBefore   int64
	Colors        int
	Distinct      int
	PSNRAlpha     float64
	PSNR          float64 // +Inf when nothing was lost
	SSIM          float64
	DistinctExact bool
}

// QuantBytes quantises a PNG and returns the candidate without writing it.
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

	encoded, err = imgio.PreservePNGMetadata(raw, encoded)
	if err != nil {
		return QuantResult{}, "metadata preservation unsupported", err
	}
	decoded, _, err := imgio.Decode(encoded)
	if err != nil {
		return QuantResult{}, "verification error", err
	}
	measured, err := metric.Compare(img, decoded)
	if err != nil {
		return QuantResult{}, "measurement error", err
	}
	var removed []string
	header, err := imgio.ReadHeader(raw)
	if err != nil {
		return QuantResult{}, "header error", err
	}
	if slices.Contains(header.Chunks, "iDOT") {
		removed = []string{"iDOT"}
	}
	return QuantResult{
		RemovedChunks: removed,
		SSIM:          measured.SSIM, PSNRAlpha: measured.PSNRAlpha, DistinctExact: q.DistinctExact,
		Encoded:     encoded,
		BytesBefore: int64(len(raw)),
		Colors:      q.Colors,
		Distinct:    q.Distinct,
		PSNR:        measured.PSNR,
	}, "", nil
}

// Quant reduces a PNG to a palette and writes it only when the result is both
// smaller and still faithful.
func Quant(path string, o QuantOptions) result.Item {
	item := result.Item{Path: path, Metrics: map[string]any{}}

	res, reason, err := QuantBytes(path, o)
	if err != nil {
		return fail(item, err, reason)
	}
	item.BytesBefore = res.BytesBefore
	item.BytesAfter = int64(len(res.Encoded))
	item.GainPercent = gain(item.BytesBefore, item.BytesAfter)
	item.Metrics["colors"] = res.Colors
	if len(res.RemovedChunks) > 0 {
		item.Metrics["candidateRemovedChunks"] = res.RemovedChunks
	}
	if res.DistinctExact {
		item.Metrics["distinct"] = res.Distinct
	} else {
		item.Metrics["coloursAtLeast"] = res.Distinct
	}
	item.Verification = &result.Verification{Status: "measured", Method: "decode+psnr+alpha+ssim"}
	item.Metrics["ssim"] = res.SSIM
	if !math.IsInf(res.PSNRAlpha, 1) {
		item.Metrics["psnrAlpha"] = res.PSNRAlpha
	}
	if !math.IsInf(res.PSNR, 1) {
		item.Metrics["psnr"] = res.PSNR
	}

	verdict := EvaluateCandidate(Candidate{BytesBefore: item.BytesBefore, BytesAfter: item.BytesAfter,
		PSNR: res.PSNR, HasPSNR: true, PSNRAlpha: res.PSNRAlpha, HasPSNRAlpha: true, SSIM: res.SSIM, HasSSIM: true}, CandidatePolicy{MinGain: o.MinGain, MinPSNR: o.MinPSNR, MinSSIM: o.MinSSIM})
	if verdict.Code == "fidelity_below_minimum" {
		return skip(item, "fidelity_below_minimum", "candidate below requested RGB, alpha-aware PSNR or SSIM")
	}
	if verdict.Code == "gain_below_minimum" {
		return skip(item, "gain_below_minimum", "gain "+percent(item.GainPercent))
	}

	if !verdict.Accept {
		return skip(item, verdict.Code, "candidate rejected by policy")
	}
	if len(res.RemovedChunks) > 0 {
		item.Reason = "omits stale iDOT decoding offsets"
	}
	item.Output = o.OutputPath(path)
	if o.Output != "" {
		item.Output = o.Output
	}
	if o.DryRun {
		item.Status = result.StatusWould
		return item
	}
	if err := AtomicWrite(item.Output, res.Encoded, o.Overwrite || o.Replace, func(data []byte) error {
		_, _, err := imgio.Decode(data)
		return err
	}); err != nil {
		return failCode(item, err, "write_failed", "write error")
	}
	item.Written = true
	item.Status = result.StatusDone
	return item
}
