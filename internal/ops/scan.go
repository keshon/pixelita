package ops

import (
	"fmt"
	"math"
	"os"
	"sort"

	webp "github.com/mayahiro/go-webp"

	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/quant"
	"github.com/keshon/pixelita/internal/report"
)

type ScanOptions struct {
	Quick       bool
	Colors      int
	Effort      int
	WebPQuality int
	MinGain     float64
	MinPSNR     float64
}

func DefaultScan() ScanOptions {
	return ScanOptions{Colors: 256, Effort: 4, WebPQuality: 90, MinGain: 10, MinPSNR: 30}
}

// Scan describes one file and, unless asked to be quick, measures what each
// tool would actually achieve on it.
func Scan(path string, o ScanOptions) report.Item {
	item := report.Item{Path: path, Metrics: map[string]any{}}

	raw, err := os.ReadFile(path)
	if err != nil {
		return fail(item, err, "read error")
	}
	item.BytesBefore = int64(len(raw))

	head, err := imgio.ReadHeader(raw)
	if err != nil {
		return fail(item, err, "unreadable header")
	}
	item.Metrics["format"] = head.Format
	item.Metrics["width"] = head.Width
	item.Metrics["height"] = head.Height
	item.Metrics["colourType"] = head.ColourType
	item.Metrics["bitDepth"] = head.BitDepth
	item.Metrics["hasAlpha"] = head.HasAlpha
	item.Metrics["size"] = fmt.Sprintf("%dx%d", head.Width, head.Height)
	if head.Interlaced {
		item.Metrics["interlaced"] = true
	}
	if head.Progressive {
		item.Metrics["progressive"] = true
	}
	if head.Ancillary > 0 {
		item.Metrics["metadataBytes"] = head.Ancillary
	}

	if o.Quick {
		item.Status = report.StatusSkipped
		item.Reason = "not measured"
		return item
	}

	img, _, err := imgio.Decode(raw)
	if err != nil {
		return fail(item, err, "decode error")
	}

	// Quantisation only applies to PNG: turning a photograph into a palette and
	// calling it a JPEG replacement is not an improvement anyone asked for.
	if head.Format == "png" {
		q := quant.Quantize(img, quant.Options{MaxColors: o.Colors, Dither: 1, Effort: o.Effort})
		item.Metrics["colours"] = q.Distinct
		item.Metrics["coloursExact"] = q.DistinctExact
		if encoded, err := imgio.EncodePalettedPNG(q.Image); err == nil {
			item.Metrics["quantBytes"] = int64(len(encoded))
			item.Metrics["quantGain"] = gain(item.BytesBefore, int64(len(encoded)))
			if !math.IsInf(q.PSNR, 1) {
				item.Metrics["quantPSNR"] = q.PSNR
			}
		}
	}

	var counter sizeCounter
	err = webp.Encode(&counter, img, &webp.Options{
		Quality: o.WebPQuality, Compression: webp.CompressionLossy, Mode: webp.ModeDefault,
	})
	if err == nil {
		item.Metrics["webpBytes"] = int64(counter.n)
		item.Metrics["webpGain"] = gain(item.BytesBefore, int64(counter.n))
	}

	chooseBest(&item, head, o)
	return item
}

// chooseBest picks the conversion worth recommending: the smallest result that
// clears both the size and the fidelity floor.
func chooseBest(item *report.Item, head imgio.Header, o ScanOptions) {
	best, bestBytes := "", item.BytesBefore

	if b, ok := item.Num("quantBytes"); ok && int64(b) < bestBytes {
		psnr, lossy := item.Num("quantPSNR")
		if !lossy || psnr >= o.MinPSNR {
			best, bestBytes = "quant", int64(b)
		}
	}
	if b, ok := item.Num("webpBytes"); ok && int64(b) < bestBytes {
		best, bestBytes = "webp", int64(b)
	}

	if best == "" || gain(item.BytesBefore, bestBytes) < o.MinGain {
		item.Status = report.StatusSkipped
		item.Reason = "optimal"
		return
	}

	item.Status = report.StatusWould
	item.BytesAfter = bestBytes
	item.GainPercent = gain(item.BytesBefore, bestBytes)
	item.Metrics["best"] = best

	// img-webp leaves palette PNGs alone by default, because on them WebP
	// usually loses. Saying so here saves the reader from running the tool and
	// wondering why nothing happened.
	if best == "webp" && head.ColourType == "palette" {
		item.Reason = PaletteCaveat
	}
}

// PaletteCaveat marks a recommendation the WebP tool would decline by default.
const PaletteCaveat = "needs -skip-palette=false"

// ScanSummary adds the lines that make a scan worth reading: what each tool
// would save across the whole set, rather than file by file.
func ScanSummary(rep *report.Report, o ScanOptions) {
	if o.Quick {
		var total, meta int64
		var withMeta int
		formats := map[string]int{}
		for _, it := range rep.Items {
			total += it.BytesBefore
			formats[it.Str("format")]++
			if m, ok := it.Num("metadataBytes"); ok {
				meta += int64(m)
				withMeta++
			}
		}
		rep.Note("%d files, %s%s", len(rep.Items), report.Size(total), formatBreakdown(formats))
		if meta > 0 {
			rep.Note("metadata: %s in %d files", report.Size(meta), withMeta)
		}
		return
	}

	var quantFiles, webpFiles, metaFiles int
	var quantSaved, webpSaved, meta int64
	for _, it := range rep.Items {
		if b, ok := it.Num("quantBytes"); ok && gain(it.BytesBefore, int64(b)) >= o.MinGain {
			quantFiles++
			quantSaved += it.BytesBefore - int64(b)
		}
		if b, ok := it.Num("webpBytes"); ok && gain(it.BytesBefore, int64(b)) >= o.MinGain {
			webpFiles++
			webpSaved += it.BytesBefore - int64(b)
		}
		if m, ok := it.Num("metadataBytes"); ok {
			meta += int64(m)
			metaFiles++
		}
	}
	if quantFiles > 0 {
		rep.Note("img-quant: %d files, would save %s", quantFiles, report.Size(quantSaved))
	}
	if webpFiles > 0 {
		rep.Note("img-webp:  %d files, would save %s", webpFiles, report.Size(webpSaved))
	}
	if meta > 0 {
		rep.Note("metadata:  %s in %d files, dropped by any re-encode", report.Size(meta), metaFiles)
	}
	for _, it := range rep.Items {
		if it.Reason == PaletteCaveat {
			rep.Note("* img-webp leaves palette PNGs alone unless told otherwise: -skip-palette=false")
			break
		}
	}
}

func formatBreakdown(formats map[string]int) string {
	if len(formats) == 0 {
		return ""
	}
	keys := make([]string, 0, len(formats))
	for k := range formats {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := " ("
	for i, k := range keys {
		if i > 0 {
			out += ", "
		}
		out += fmt.Sprintf("%d %s", formats[k], k)
	}
	return out + ")"
}

// sizeCounter counts what an encoder writes without keeping it. An estimate
// needs the size, not the bytes, and a directory of photographs is a lot of
// bytes to hold for nothing.
type sizeCounter struct{ n int }

func (c *sizeCounter) Write(p []byte) (int, error) {
	c.n += len(p)
	return len(p), nil
}
