package ops

import (
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"

	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/metric"
	"github.com/keshon/pixelita/internal/report"
)

type DiffOptions struct {
	Out     string          // where to write the difference map, empty for none
	Crop    image.Rectangle // measure only this region of both; empty means all
	Amplify float64
	MinPSNR float64
	MinSSIM float64
}

func DefaultDiff() DiffOptions { return DiffOptions{Amplify: 8} }

// Diff measures how far b is from a.
func Diff(a, b string, o DiffOptions) report.Item {
	item := report.Item{Path: a, Output: b, Metrics: map[string]any{}}

	imgA, _, rawA, err := imgio.Load(a)
	if err != nil {
		return fail(item, err, "decode error")
	}
	imgB, _, rawB, err := imgio.Load(b)
	if err != nil {
		return fail(item, err, "decode error")
	}
	// Only when the whole file is the subject. Measuring one region and
	// printing "saved 69%" next to it describes a different thing entirely, and
	// in an audit of someone else's file it frames the work wrongly as well:
	// nothing was converted here, something was checked.
	if o.Crop.Empty() {
		item.BytesBefore = int64(len(rawA))
		item.BytesAfter = int64(len(rawB))
		item.GainPercent = gain(item.BytesBefore, item.BytesAfter)
	}

	// A whole-image average answers "is it broken" and hides where. Quantisers
	// and encoders do not spread their error evenly: on one photograph measured
	// here the smooth sky was a tie between two programs while the shadows
	// differed by 2.3 dB, and only the per-region figure said so. The rectangle
	// is the same one img-look takes, so a region can be looked at and measured
	// without restating it in different terms.
	//
	// The byte counts above stay whole-file on purpose: they describe the files,
	// which is still what was written, and cropping cannot change that.
	if !o.Crop.Empty() {
		ca, err := cropTo(imgio.ToNRGBA(imgA), o.Crop)
		if err != nil {
			return fail(item, err, "crop outside image")
		}
		cb, err := cropTo(imgio.ToNRGBA(imgB), o.Crop)
		if err != nil {
			return fail(item, err, "crop outside image")
		}
		imgA, imgB = ca, cb
		item.Metrics["crop"] = Rect(o.Crop)
	}

	res, err := metric.Compare(imgA, imgB)
	if err != nil {
		ab, bb := imgA.Bounds(), imgB.Bounds()
		return fail(item, fmt.Errorf("%w: %dx%d and %dx%d", err,
			ab.Dx(), ab.Dy(), bb.Dx(), bb.Dy()), "size mismatch")
	}

	if !math.IsInf(res.PSNR, 1) {
		item.Metrics["psnr"] = res.PSNR
		item.Metrics["psnrAlpha"] = res.PSNRAlpha
	}
	item.Metrics["ssim"] = res.SSIM
	item.Metrics["maxDelta"] = res.MaxDelta
	item.Metrics["p95"] = res.P95
	item.Metrics["p99"] = res.P99
	item.Metrics["differentPercent"] = res.Different
	item.Metrics["pixels"] = res.Pixels

	if o.Out != "" {
		if err := writeDiffMap(imgA, imgB, o); err != nil {
			return fail(item, err, "diff map error")
		}
	}

	switch {
	case o.MinPSNR > 0 && res.PSNR < o.MinPSNR:
		item.Status = report.StatusFailed
		item.Reason = fmt.Sprintf("below %.0f dB", o.MinPSNR)
	case o.MinSSIM > 0 && res.SSIM < o.MinSSIM:
		item.Status = report.StatusFailed
		item.Reason = fmt.Sprintf("below %.3f ssim", o.MinSSIM)
	default:
		item.Status = report.StatusDone
	}
	return item
}

// DiffMap renders the difference, brightened so that errors too small to see
// side by side become obvious. Black means the two images agree.
func DiffMap(a, b image.Image, amplify float64) *image.NRGBA {
	x, y := imgio.ToNRGBA(a), imgio.ToNRGBA(b)
	out := image.NewNRGBA(x.Rect)
	for i := 0; i < len(x.Pix) && i < len(y.Pix); i += 4 {
		for c := 0; c < 3; c++ {
			d := int(x.Pix[i+c]) - int(y.Pix[i+c])
			if d < 0 {
				d = -d
			}
			out.Pix[i+c] = uint8(math.Min(float64(d)*amplify, 255))
		}
		out.Pix[i+3] = 255
	}
	return out
}

func writeDiffMap(a, b image.Image, o DiffOptions) error {
	data, err := imgio.EncodePNG(DiffMap(a, b, o.Amplify))
	if err != nil {
		return err
	}
	if dir := filepath.Dir(o.Out); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(o.Out, data, 0o644)
}
