package ops

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"

	webp "github.com/mayahiro/go-webp"

	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/quant"
	"github.com/keshon/pixelita/internal/report"
	"github.com/keshon/pixelita/internal/resize"
)

type ScanOptions struct {
	Quick       bool
	Colors      int
	Effort      int
	WebPQuality int
	MinGain     float64
	MinPSNR     float64
	// DisplayWidths are the widths these images might actually be shown at.
	//
	// A curve rather than a threshold, and that was a correction. The first
	// version took one number, defaulted it to 1920, and reported what resizing
	// to it would save. On the folder that prompted the feature it found 17 KB,
	// because the files were 2016px wide and 1920 is barely narrower — while
	// the real win, serving telephones something built for telephones, was a
	// factor of eighteen. A single threshold cannot help but be a guess about
	// someone else's layout, and a guess that small reads as "not worth it".
	//
	// Three widths cost three small encodes on the files that are oversized,
	// and hand back the decision instead of making it.
	//
	// This is here because a scan of a real folder reported, correctly, that
	// WebP would save 83% — and said nothing about every file being two
	// thousand pixels wide and served at that size to telephones. Resizing
	// first turned an 83% saving into a factor of eighteen. The larger of the
	// two wins was the one the tool did not mention.
	//
	// It is a caller's figure, not a fact about the file: two thousand pixels
	// are excessive for a landing page and unremarkable for a print source or
	// an archive master. So the number used is printed with the result, where
	// it can be seen and argued with, rather than applied quietly.
	DisplayWidths []int
}

func DefaultScan() ScanOptions {
	return ScanOptions{Colors: 256, Effort: 4, WebPQuality: 90, MinGain: 10,
		MinPSNR: 30, DisplayWidths: []int{640, 1280, 1920}}
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
	// "palette" says a palette is in use; only this says how big it is, which
	// is the difference between a file quantised to 256 colours and one
	// quantised to 64. A reader judging what an encoder did needs the number,
	// and inferring it from the bit depth gives the ceiling, not the answer.
	if head.Palette > 0 {
		item.Metrics["paletteSize"] = head.Palette
	}
	if len(head.Chunks) > 0 {
		// Named, not just weighed. Whether a file still declares its colour
		// space is a question about gAMA, cHRM, sRGB and iCCP being present,
		// and it decides whether "the colours were ruined" is a fair charge.
		item.Metrics["chunks"] = head.Chunks
	}
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

	// Width comes from the header, so this costs nothing and is reported even
	// under -quick, where no encoding happens at all.
	if w := widest(o.DisplayWidths); w > 0 && head.Width > w {
		item.Metrics["widthOver"] = head.Width - w
	}

	if o.Quick {
		// Worded so it does not read as a fault. -quick is asked for, and
		// "skipped: not measured" looks like something went wrong with a file
		// that is in fact perfectly fine — it just was not encoded.
		item.Status = report.StatusSkipped
		item.Reason = "headers only"
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
		if !q.DistinctExact {
			// Named so it cannot be read as the answer. A field called
			// "colours" sitting next to a separate "coloursExact: false" was
			// quoted to five significant digits by a reader who missed the
			// flag; a name that carries the caveat cannot be misread that way.
			item.Metrics["coloursAtLeast"] = q.Distinct
			delete(item.Metrics, "colours")
		}
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

	// Measured, not estimated: the file is actually resized and actually
	// re-encoded, because a projected saving dressed as a measurement is worse
	// than no number. It is cheap — the encode is of a smaller image than the
	// one already encoded above.
	if _, over := item.Num("widthOver"); over {
		src := imgio.ToNRGBA(img)
		at := map[string]int64{}
		for _, target := range o.DisplayWidths {
			if target <= 0 || target >= head.Width {
				continue // no upscaling, and no pretending a width is a saving
			}
			w, h := resize.Fit(head.Width, head.Height, target, 0, "inside")
			var c sizeCounter
			if err := webp.Encode(&c, resize.Resize(src, w, h, resize.Lanczos3),
				&webp.Options{Quality: o.WebPQuality,
					Compression: webp.CompressionLossy, Mode: webp.ModeDefault}); err == nil {
				at[strconv.Itoa(target)] = int64(c.n)
			}
		}
		if len(at) > 0 {
			item.Metrics["atWidth"] = at
		}
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
		if wide := countWide(rep); wide > 0 {
			rep.Note("oversize: %d files wider than %dpx — run without -quick to measure "+
				"what they would cost at smaller widths", wide, widest(o.DisplayWidths))
		}
		return
	}

	var quantFiles, webpFiles, metaFiles, wideFiles int
	var quantSaved, webpSaved, meta int64
	atWidth := map[string]int64{}
	var total int64
	for _, it := range rep.Items {
		total += it.BytesBefore
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
		if at, ok := it.Metrics["atWidth"].(map[string]int64); ok {
			wideFiles++
			for w, b := range at {
				atWidth[w] += b
			}
			// A file narrower than a given width is served whole at it, so it
			// has to count towards that width's total or the totals compare
			// different sets of files.
			for _, w := range o.DisplayWidths {
				if _, measured := at[strconv.Itoa(w)]; !measured {
					atWidth[strconv.Itoa(w)] += smallest(it)
				}
			}
		} else if _, wide := it.Num("widthOver"); !wide {
			for _, w := range o.DisplayWidths {
				atWidth[strconv.Itoa(w)] += smallest(it)
			}
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
	// Last, and deliberately after the codec lines, because it is usually the
	// larger number and reads as the correction to them: converting a file
	// nobody needs at that size is the second question, not the first.
	// Last, and deliberately after the codec lines, because it is usually the
	// larger number and reads as the correction to them: converting a file that
	// nobody needs at that size is the second question, not the first.
	if wideFiles > 0 {
		widths := append([]int(nil), o.DisplayWidths...)
		sort.Ints(widths)
		for _, w := range widths {
			b, ok := atWidth[strconv.Itoa(w)]
			if !ok || b == 0 {
				continue
			}
			rep.Note("at %4dpx:  %s for the whole set (%s), %d files are wider than that",
				w, report.Size(b), report.Percent(gain(total, b), total > 0), wideFiles)
		}
		rep.Note("           widths come from -widths; serving these needs srcset")
	}
	for _, it := range rep.Items {
		if it.Reason == PaletteCaveat {
			rep.Note("* img-webp leaves palette PNGs alone unless told otherwise: -skip-palette=false")
			break
		}
	}
}

// smallest is what a file would weigh after the best conversion available to
// it, or as it stands if none pays off.
func smallest(it report.Item) int64 {
	if it.BytesAfter > 0 {
		return it.BytesAfter
	}
	return it.BytesBefore
}

func widest(widths []int) int {
	w := 0
	for _, v := range widths {
		w = max(w, v)
	}
	return w
}

func countWide(rep *report.Report) int {
	n := 0
	for _, it := range rep.Items {
		if _, ok := it.Num("widthOver"); ok {
			n++
		}
	}
	return n
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
