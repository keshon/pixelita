package ops

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/report"
	"github.com/keshon/pixelita/internal/resize"
)

// Look prepares images to be looked at rather than processed.
//
// It exists because of what kept being written by hand around this toolkit: a
// throwaway program to stack a before and an after, another to crop the same
// region out of both, another to drop a transparent image onto a checkerboard,
// another to print a few pixel values. Each was ten minutes and each was thrown
// away again. They are all the same operation — make this visible — and this is
// it.
//
// The output is a PNG, because that is what can be opened anywhere and handed
// to a model that reads images. The size cap is there for the same reason: an
// eight-megapixel photograph tells a reader nothing it could not learn from a
// tenth of it, and costs a great deal more to look at.
type LookOptions struct {
	Max        int             // longest side of each panel; 0 keeps the original
	Crop       image.Rectangle // empty means the whole image
	Background string          // checker, white, black, none
	Across     bool            // lay panels side by side rather than stacked
	Label      bool            // write the file name on each panel
	Out        string          // empty means LookPath()
}

func DefaultLook() LookOptions {
	return LookOptions{Max: 1400, Background: "checker", Label: true}
}

// LookPath is where the composite goes when nobody says otherwise.
//
// Not the working directory. This tool produces something to glance at and
// forget, and a glance should not leave a file in someone's repository for
// `git status` to find later. The path is fixed rather than unique so that it
// can be predicted without reading the output first — looking twice overwrites,
// which is what looking twice means. `-out` is there for when two composites
// have to exist at once.
func LookPath() string {
	return filepath.Join(os.TempDir(), "pixelita", "look.png")
}

var (
	sepColour   = color.NRGBA{220, 40, 40, 255} // a colour no photograph owns
	labelInk    = color.NRGBA{255, 255, 255, 255}
	labelGround = color.NRGBA{0, 0, 0, 190}
	padColour   = color.NRGBA{24, 24, 24, 255}
)

const sepWidth = 3

// Look builds the composite and returns it along with one report item per input.
func Look(paths []string, o LookOptions) (*image.NRGBA, []report.Item, error) {
	if len(paths) == 0 {
		return nil, nil, fmt.Errorf("nothing to look at")
	}

	panels := make([]*image.NRGBA, 0, len(paths))
	items := make([]report.Item, 0, len(paths))

	for _, p := range paths {
		item := report.Item{Path: p, Metrics: map[string]any{}}
		img, _, raw, err := imgio.Load(p)
		if err != nil {
			items = append(items, fail(item, err, "decode error"))
			continue
		}
		// The size goes in the metrics rather than in BytesBefore: those fields
		// mean "this operation changed the size", and looking at a picture
		// changes nothing. Left there, the summary would claim it saved the
		// whole file.
		item.Metrics["bytes"] = int64(len(raw))

		src := imgio.ToNRGBA(img)
		item.Metrics["from"] = fmt.Sprintf("%dx%d", src.Rect.Dx(), src.Rect.Dy())

		if !o.Crop.Empty() {
			src = cropTo(src, o.Crop)
			item.Metrics["crop"] = fmt.Sprintf("%dx%d at %d,%d",
				o.Crop.Dx(), o.Crop.Dy(), o.Crop.Min.X, o.Crop.Min.Y)
		}
		if alpha := hasAlpha(src); alpha && o.Background != "none" {
			src = onBackground(src, o.Background)
			item.Metrics["background"] = o.Background
		}
		if o.Max > 0 {
			w, h := resize.Fit(src.Rect.Dx(), src.Rect.Dy(), o.Max, o.Max, "inside")
			if w < src.Rect.Dx() {
				src = resize.Resize(src, w, h, resize.CatmullRom)
				item.Metrics["scaled"] = fmt.Sprintf("%dx%d", w, h)
			}
		}
		if o.Label {
			drawLabel(src, filepath.Base(p))
		}

		item.Metrics["to"] = fmt.Sprintf("%dx%d", src.Rect.Dx(), src.Rect.Dy())
		item.Status = report.StatusDone
		items = append(items, item)
		panels = append(panels, src)
	}

	if len(panels) == 0 {
		return nil, items, fmt.Errorf("nothing could be read")
	}
	return compose(panels, o.Across), items, nil
}

// Probe answers the other half of "let me see it": the exact numbers.
//
// Reading a picture tells you something is wrong; reading the pixels tells you
// what. The investigation that found a colour-range bug in this very repo came
// down to four values printed side by side.
func Probe(path string, points []image.Point) ([]string, error) {
	img, _, _, err := imgio.Load(path)
	if err != nil {
		return nil, err
	}
	src := imgio.ToNRGBA(img)
	b := src.Rect

	out := make([]string, 0, len(points))
	for _, pt := range points {
		if !pt.Add(b.Min).In(b) {
			out = append(out, fmt.Sprintf("%d,%d  outside the image (%dx%d)",
				pt.X, pt.Y, b.Dx(), b.Dy()))
			continue
		}
		c := src.NRGBAAt(b.Min.X+pt.X, b.Min.Y+pt.Y)
		out = append(out, fmt.Sprintf("%d,%d  rgba %3d %3d %3d %3d  #%02x%02x%02x",
			pt.X, pt.Y, c.R, c.G, c.B, c.A, c.R, c.G, c.B))
	}
	return out, nil
}

func cropTo(src *image.NRGBA, r image.Rectangle) *image.NRGBA {
	r = r.Add(src.Rect.Min).Intersect(src.Rect)
	if r.Empty() {
		return src
	}
	out := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Rect, src, r.Min, draw.Src)
	return out
}

// onBackground makes transparency visible. A checkerboard is the convention
// because it cannot be mistaken for content: no photograph is a grey grid.
func onBackground(src *image.NRGBA, kind string) *image.NRGBA {
	out := image.NewNRGBA(src.Rect)
	switch kind {
	case "white":
		draw.Draw(out, out.Rect, &image.Uniform{color.White}, image.Point{}, draw.Src)
	case "black":
		draw.Draw(out, out.Rect, &image.Uniform{color.Black}, image.Point{}, draw.Src)
	default:
		for y := out.Rect.Min.Y; y < out.Rect.Max.Y; y++ {
			for x := out.Rect.Min.X; x < out.Rect.Max.X; x++ {
				v := uint8(210)
				if (x/8+y/8)%2 == 0 {
					v = 150
				}
				out.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
			}
		}
	}
	draw.Draw(out, out.Rect, src, src.Rect.Min, draw.Over)
	return out
}

// drawLabel writes the name over a dark strip, so it reads on a white image and
// on a black one alike.
func drawLabel(dst *image.NRGBA, text string) {
	face := basicfont.Face7x13
	w := font.MeasureString(face, text).Ceil()
	h := face.Metrics().Height.Ceil()
	if w+8 > dst.Rect.Dx() {
		return // no room; a clipped label is worse than none
	}

	box := image.Rect(dst.Rect.Min.X, dst.Rect.Min.Y,
		dst.Rect.Min.X+w+8, dst.Rect.Min.Y+h+4)
	draw.Draw(dst, box, &image.Uniform{labelGround}, image.Point{}, draw.Over)

	d := &font.Drawer{
		Dst:  dst,
		Src:  &image.Uniform{labelInk},
		Face: face,
		Dot: fixed.P(dst.Rect.Min.X+4,
			dst.Rect.Min.Y+face.Metrics().Ascent.Ceil()+2),
	}
	d.DrawString(text)
}

// compose lays the panels out with a separator between them, so the eye knows
// where one picture ends even when two are nearly identical — which, when
// comparing a before with an after, they usually are.
func compose(panels []*image.NRGBA, across bool) *image.NRGBA {
	if len(panels) == 1 {
		return panels[0]
	}

	var w, h int
	for i, p := range panels {
		if across {
			w += p.Rect.Dx()
			if i > 0 {
				w += sepWidth
			}
			h = max(h, p.Rect.Dy())
		} else {
			h += p.Rect.Dy()
			if i > 0 {
				h += sepWidth
			}
			w = max(w, p.Rect.Dx())
		}
	}

	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(out, out.Rect, &image.Uniform{padColour}, image.Point{}, draw.Src)

	at := 0
	for i, p := range panels {
		if i > 0 {
			var sep image.Rectangle
			if across {
				sep = image.Rect(at, 0, at+sepWidth, h)
			} else {
				sep = image.Rect(0, at, w, at+sepWidth)
			}
			draw.Draw(out, sep, &image.Uniform{sepColour}, image.Point{}, draw.Src)
			at += sepWidth
		}
		var r image.Rectangle
		if across {
			r = image.Rect(at, 0, at+p.Rect.Dx(), p.Rect.Dy())
			at += p.Rect.Dx()
		} else {
			r = image.Rect(0, at, p.Rect.Dx(), at+p.Rect.Dy())
			at += p.Rect.Dy()
		}
		draw.Draw(out, r, p, p.Rect.Min, draw.Src)
	}
	return out
}

// WriteLook saves the composite and reports where it landed.
//
// The path comes back absolute because the next thing to happen to it is that
// something else opens it, quite possibly from another directory.
func WriteLook(img *image.NRGBA, path string) (string, int64, error) {
	if strings.TrimSpace(path) == "" {
		path = LookPath()
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	data, err := imgio.EncodePNG(img)
	if err != nil {
		return "", 0, err
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", 0, err
		}
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", 0, err
	}
	return path, int64(len(data)), nil
}

// ParseRect reads "x,y,w,h".
func ParseRect(s string) (image.Rectangle, error) {
	if strings.TrimSpace(s) == "" {
		return image.Rectangle{}, nil
	}
	var x, y, w, h int
	if _, err := fmt.Sscanf(strings.ReplaceAll(s, " ", ""), "%d,%d,%d,%d", &x, &y, &w, &h); err != nil {
		return image.Rectangle{}, fmt.Errorf("expected x,y,w,h, got %q", s)
	}
	if w <= 0 || h <= 0 {
		return image.Rectangle{}, fmt.Errorf("width and height must be positive in %q", s)
	}
	return image.Rect(x, y, x+w, y+h), nil
}

// ParsePoints reads "x,y" repeated: "10,20 300,15" or "10,20;300,15".
func ParsePoints(s string) ([]image.Point, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ';' })
	out := make([]image.Point, 0, len(fields))
	for _, f := range fields {
		var x, y int
		if _, err := fmt.Sscanf(f, "%d,%d", &x, &y); err != nil {
			return nil, fmt.Errorf("expected x,y, got %q", f)
		}
		out = append(out, image.Point{x, y})
	}
	return out, nil
}
