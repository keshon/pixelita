package ops

import (
	"bytes"
	"fmt"
	"image"
	"math"

	webp "github.com/mayahiro/go-webp"

	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/metric"
)

type Candidate struct {
	BytesBefore  int64
	BytesAfter   int64
	PSNR         float64
	PSNRAlpha    float64
	HasPSNRAlpha bool
	SSIM         float64
	HasPSNR      bool
	HasSSIM      bool
}

type CandidatePolicy struct {
	MinGain float64
	MinPSNR float64
	MinSSIM float64
}

type Verdict struct {
	Accept bool
	Code   string
	Gain   float64
}

type EvaluatedCandidate struct {
	Encoded   []byte
	Candidate Candidate
	Verdict   Verdict
}

// EvaluateWebPCandidate is the shared encode, decode, measurement, and policy
// path for scanning and direct WebP conversion.
func EvaluateWebPCandidate(source image.Image, bytesBefore int64, options *webp.Options, policy CandidatePolicy) (EvaluatedCandidate, error) {
	var encoded bytes.Buffer
	if err := webp.Encode(&encoded, source, options); err != nil {
		return EvaluatedCandidate{}, err
	}
	decoded, _, err := imgio.Decode(encoded.Bytes())
	if err != nil {
		return EvaluatedCandidate{}, err
	}
	comparison, err := metric.Compare(source, decoded)
	if err != nil {
		return EvaluatedCandidate{}, err
	}
	candidate := Candidate{
		BytesBefore: bytesBefore,
		BytesAfter:  int64(encoded.Len()),
		PSNR:        comparison.PSNR, PSNRAlpha: comparison.PSNRAlpha, HasPSNRAlpha: true,
		SSIM:    comparison.SSIM,
		HasPSNR: true,
		HasSSIM: true,
	}
	return EvaluatedCandidate{
		Encoded:   append([]byte(nil), encoded.Bytes()...),
		Candidate: candidate,
		Verdict:   EvaluateCandidate(candidate, policy),
	}, nil
}

// EvaluateCandidate is the single size/fidelity verdict used by direct
// converters and scan recommendations.
func EvaluateCandidate(c Candidate, p CandidatePolicy) Verdict {
	v := Verdict{Accept: true, Gain: gain(c.BytesBefore, c.BytesAfter)}
	if err := p.Validate(); err != nil {
		return Verdict{Code: "invalid_threshold", Gain: v.Gain}
	}
	if p.MinPSNR > 0 && (!c.HasPSNR || math.IsNaN(c.PSNR) || math.IsInf(c.PSNR, -1)) || p.MinSSIM > 0 && (!c.HasSSIM || math.IsNaN(c.SSIM) || math.IsInf(c.SSIM, 0)) {
		return Verdict{Code: "fidelity_unavailable", Gain: v.Gain}
	}
	if p.MinPSNR > 0 && c.HasPSNR && !math.IsInf(c.PSNR, 1) && c.PSNR < p.MinPSNR {
		return Verdict{Code: "fidelity_below_minimum", Gain: v.Gain}
	}
	if p.MinPSNR > 0 && c.HasPSNRAlpha && (math.IsNaN(c.PSNRAlpha) || c.PSNRAlpha < p.MinPSNR) {
		return Verdict{Code: "fidelity_below_minimum", Gain: v.Gain}
	}
	if p.MinSSIM > 0 && c.HasSSIM && c.SSIM < p.MinSSIM {
		return Verdict{Code: "fidelity_below_minimum", Gain: v.Gain}
	}
	if v.Gain < p.MinGain {
		return Verdict{Code: "gain_below_minimum", Gain: v.Gain}
	}
	return v
}

// Validate rejects values whose comparisons would silently bypass policy.
func (p CandidatePolicy) Validate() error {
	for _, v := range []float64{p.MinGain, p.MinPSNR, p.MinSSIM} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return fmt.Errorf("thresholds must be finite and non-negative")
		}
	}
	if p.MinGain > 100 || p.MinSSIM > 1 {
		return fmt.Errorf("min-gain cannot exceed 100 and min-ssim cannot exceed 1")
	}
	return nil
}
