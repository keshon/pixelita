package ops

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/jpegopt"
	"github.com/keshon/pixelita/internal/result"
)

type JPEGOptions struct {
	MinGain   float64
	DryRun    bool
	Replace   bool
	Suffix    string
	OutDir    string
	Output    string
	Overwrite bool
}

// DefaultJPEG asks for almost nothing, because there is nothing to weigh: the
// picture cannot change, so any saving at all is worth taking.
func DefaultJPEG() JPEGOptions {
	return JPEGOptions{MinGain: 1, Suffix: "-min"}
}

func (o JPEGOptions) OutputPath(path string) string {
	out := path
	if !o.Replace {
		out = sibling(path, o.Suffix, ".jpg")
	}
	if o.OutDir != "" {
		out = filepath.Join(o.OutDir, filepath.Base(out))
	}
	return out
}

// JPEG rewrites a JPEG with Huffman tables fitted to its own contents.
func JPEG(path string, o JPEGOptions) result.Item {
	item := result.Item{Path: path, Metrics: map[string]any{}}

	raw, err := os.ReadFile(path)
	if err != nil {
		return fail(item, err, "read error")
	}
	item.BytesBefore = int64(len(raw))

	out, err := jpegopt.Optimise(raw)
	if err != nil {
		if errors.Is(err, jpegopt.ErrProgressive) {
			return skip(item, "progressive_jpeg_unsupported", "progressive")
		}
		return fail(item, err, "not readable as baseline jpeg")
	}

	if err := verifyJPEGIdentity(raw, out); err != nil {
		return failCode(item, err, "verification_failed", "pixel identity verification failed")
	}
	item.BytesAfter = int64(len(out))
	item.GainPercent = gain(item.BytesBefore, item.BytesAfter)
	item.Metrics["lossless"] = true

	if item.GainPercent < o.MinGain {
		return skip(item, "gain_below_minimum", "gain "+percent(item.GainPercent))
	}

	item.Verification = &result.Verification{Status: "verified", Method: "decode+pixel-identity"}
	item.Output = o.OutputPath(path)
	if o.Output != "" {
		item.Output = o.Output
	}
	if o.DryRun {
		item.Status = result.StatusWould
		return item
	}
	if err := AtomicWrite(item.Output, out, o.Overwrite || o.Replace, func(data []byte) error { return verifyJPEGIdentity(raw, data) }); err != nil {
		return failCode(item, err, "write_failed", "write error")
	}

	item.Verification = &result.Verification{Status: "verified", Method: "decode+pixel-identity"}
	item.Written = true
	item.Status = result.StatusDone
	return item
}

func verifyJPEGIdentity(raw, data []byte) error {
	before, _, err := imgio.Decode(raw)
	if err != nil {
		return err
	}
	after, _, err := imgio.Decode(data)
	if err != nil {
		return err
	}
	a, b := imgio.ToNRGBA(before), imgio.ToNRGBA(after)
	if a.Rect != b.Rect || !bytes.Equal(a.Pix, b.Pix) {
		return fmt.Errorf("JPEG optimization changed decoded pixels")
	}
	return nil
}
