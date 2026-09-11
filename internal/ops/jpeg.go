package ops

import (
	"errors"
	"os"

	"github.com/keshon/pixelita/internal/jpegopt"
	"github.com/keshon/pixelita/internal/report"
)

type JPEGOptions struct {
	MinGain float64
	DryRun  bool
	Replace bool
	Suffix  string
}

// DefaultJPEG asks for almost nothing, because there is nothing to weigh: the
// picture cannot change, so any saving at all is worth taking.
func DefaultJPEG() JPEGOptions {
	return JPEGOptions{MinGain: 1, Suffix: "-min"}
}

// JPEG rewrites a JPEG with Huffman tables fitted to its own contents.
func JPEG(path string, o JPEGOptions) report.Item {
	item := report.Item{Path: path, Metrics: map[string]any{}}

	raw, err := os.ReadFile(path)
	if err != nil {
		return fail(item, err, "read error")
	}
	item.BytesBefore = int64(len(raw))

	out, err := jpegopt.Optimise(raw)
	if err != nil {
		if errors.Is(err, jpegopt.ErrProgressive) {
			item.Status = report.StatusSkipped
			item.Reason = "progressive"
			return item
		}
		return fail(item, err, "not readable as baseline jpeg")
	}

	item.BytesAfter = int64(len(out))
	item.GainPercent = gain(item.BytesBefore, item.BytesAfter)
	item.Metrics["lossless"] = true

	if item.GainPercent < o.MinGain {
		item.Status = report.StatusSkipped
		item.Reason = "gain " + percent(item.GainPercent)
		return item
	}

	item.Output = path
	if !o.Replace {
		item.Output = sibling(path, o.Suffix, ".jpg")
	}
	if o.DryRun {
		item.Status = report.StatusWould
		return item
	}
	if err := os.WriteFile(item.Output, out, 0o644); err != nil {
		return fail(item, err, "write error")
	}
	item.Status = report.StatusDone
	return item
}
