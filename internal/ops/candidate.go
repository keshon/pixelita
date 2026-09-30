package ops

import (
	"bytes"
	"image"
	"math"

	webp "github.com/mayahiro/go-webp"

	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/metric"
)

type Candidate struct {
	BytesBefore int64
	BytesAfter  int64
	PSNR        float64
	SSIM        float64
	HasPSNR     bool
	HasSSIM     bool
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
		PSNR:        comparison.PSNR,
		SSIM:        comparison.SSIM,
		HasPSNR:     true,
		HasSSIM:     true,
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
	if p.MinPSNR > 0 && c.HasPSNR && !math.IsInf(c.PSNR, 1) && c.PSNR < p.MinPSNR {
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
