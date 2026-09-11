package ops

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"

	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/report"
	"github.com/keshon/pixelita/internal/resize"
)

type ResizeOptions struct {
	Width, Height       int
	MaxWidth, MaxHeight int
	Scale               float64
	Widths              []int
	Fit                 string // inside, outside, cover or exact
	Filter              resize.Filter
	AllowUpscale        bool
	Format              string // keep, png or jpeg
	JPEGQuality         int
	OutDir              string
	Suffix              string
	Replace             bool
	DryRun              bool
}

func DefaultResize() ResizeOptions {
	return ResizeOptions{Fit: "inside", Filter: resize.CatmullRom, Format: "keep", JPEGQuality: 90}
}

// Targets works out every size a file should be written at, and returns nothing
// when the options say to leave it alone.
//
// The order matters: an explicit list of widths beats a scale, a scale beats a
// ceiling, and a ceiling beats a box. Each is a different intent, and mixing
// them silently would be worse than picking one.
func (o ResizeOptions) Targets(sw, sh int) ([][2]int, string) {
	switch {
	case len(o.Widths) > 0:
		var out [][2]int
		for _, w := range o.Widths {
			tw, th := resize.Fit(sw, sh, w, 0, "inside")
			out = append(out, [2]int{tw, th})
		}
		return out, ""

	case o.Scale > 0:
		w := max(int(float64(sw)*o.Scale+0.5), 1)
		h := max(int(float64(sh)*o.Scale+0.5), 1)
		return [][2]int{{w, h}}, ""

	case o.MaxWidth > 0 || o.MaxHeight > 0:
		if (o.MaxWidth == 0 || sw <= o.MaxWidth) && (o.MaxHeight == 0 || sh <= o.MaxHeight) {
			return nil, "already within bounds"
		}
		w, h := o.MaxWidth, o.MaxHeight
		if w == 0 {
			w = sw
		}
		if h == 0 {
			h = sh
		}
		tw, th := resize.Fit(sw, sh, w, h, "inside")
		return [][2]int{{tw, th}}, ""

	case o.Width > 0 || o.Height > 0:
		mode := o.Fit
		if mode == "cover" {
			mode = "outside"
		}
		tw, th := resize.Fit(sw, sh, o.Width, o.Height, mode)
		return [][2]int{{tw, th}}, ""
	}
	return nil, "no size asked for"
}

// Resize scales one file into every size the options ask for.
func Resize(path string, o ResizeOptions) []report.Item {
	base := report.Item{Path: path, Metrics: map[string]any{}}

	img, format, raw, err := imgio.Load(path)
	if err != nil {
		return []report.Item{fail(base, err, "decode error")}
	}
	base.BytesBefore = int64(len(raw))
	src := imgio.ToNRGBA(img)
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	base.Metrics["from"] = fmt.Sprintf("%dx%d", sw, sh)
	base.Metrics["filter"] = o.Filter.Name

	sizes, reason := o.Targets(sw, sh)
	if len(sizes) == 0 {
		base.Status = report.StatusSkipped
		base.Reason = reason
		return []report.Item{base}
	}

	out := make([]report.Item, 0, len(sizes))
	for _, size := range sizes {
		out = append(out, resizeOne(src, base, path, format, size[0], size[1], o))
	}
	return out
}

func resizeOne(src *image.NRGBA, base report.Item, path, format string, w, h int, o ResizeOptions) report.Item {
	item := base
	item.Metrics = make(map[string]any, len(base.Metrics)+4)
	for k, v := range base.Metrics {
		item.Metrics[k] = v
	}
	sw, sh := src.Rect.Dx(), src.Rect.Dy()

	if (w > sw || h > sh) && !o.AllowUpscale {
		item.Status = report.StatusSkipped
		item.Reason = "would upscale"
		return item
	}

	dst := resize.Resize(src, w, h, o.Filter)
	if o.Fit == "cover" && o.Width > 0 && o.Height > 0 && len(o.Widths) == 0 {
		dst = resize.Crop(dst, o.Width, o.Height)
	}
	item.Metrics["width"] = dst.Rect.Dx()
	item.Metrics["height"] = dst.Rect.Dy()
	item.Metrics["to"] = fmt.Sprintf("%dx%d", dst.Rect.Dx(), dst.Rect.Dy())

	encoded, ext, err := EncodeAs(dst, format, o, item.Metrics)
	if err != nil {
		return fail(item, err, "encode error")
	}
	item.BytesAfter = int64(len(encoded))
	item.GainPercent = gain(item.BytesBefore, item.BytesAfter)
	item.Output = o.OutputPath(path, ext, dst.Rect.Dx(), dst.Rect.Dy())

	if o.DryRun {
		item.Status = report.StatusWould
		return item
	}
	if dir := filepath.Dir(item.Output); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fail(item, err, "write error")
		}
	}
	if err := os.WriteFile(item.Output, encoded, 0o644); err != nil {
		return fail(item, err, "write error")
	}
	item.Status = report.StatusDone
	return item
}

// EncodeAs writes the result in the format the source had, unless told
// otherwise. JPEG cannot carry transparency, so an image with alpha is
// flattened onto white — and the report says so, rather than quietly changing
// what the file shows.
func EncodeAs(img *image.NRGBA, format string, o ResizeOptions, metrics map[string]any) ([]byte, string, error) {
	target := o.Format
	if target == "keep" || target == "" {
		target = "png"
		if format == "jpeg" {
			target = "jpeg"
		}
	}

	if target == "jpeg" {
		if hasAlpha(img) {
			if metrics != nil {
				metrics["flattened"] = "onto white"
			}
			flat := image.NewNRGBA(img.Rect)
			draw.Draw(flat, flat.Rect, &image.Uniform{color.White}, image.Point{}, draw.Src)
			draw.Draw(flat, flat.Rect, img, img.Rect.Min, draw.Over)
			img = flat
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: o.JPEGQuality}); err != nil {
			return nil, "", err
		}
		return buf.Bytes(), ".jpg", nil
	}

	data, err := imgio.EncodePNG(img)
	return data, ".png", err
}

func hasAlpha(img *image.NRGBA) bool {
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 255 {
			return true
		}
	}
	return false
}

// OutputPath names the result: the size, unless the caller had a better idea.
func (o ResizeOptions) OutputPath(path, ext string, w, h int) string {
	if o.Replace && len(o.Widths) == 0 {
		return path
	}
	dir, base := filepath.Split(path)
	base = strings.TrimSuffix(base, filepath.Ext(base))

	suffix := o.Suffix
	switch {
	case suffix != "":
	case len(o.Widths) > 0:
		suffix = fmt.Sprintf("-%dw", w)
	default:
		suffix = fmt.Sprintf("-%dx%d", w, h)
	}
	if o.OutDir != "" {
		dir = o.OutDir
	}
	return filepath.Join(dir, base+suffix+ext)
}
