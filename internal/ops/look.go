package ops

import (
	"fmt"
	"hash/fnv"
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
	Zoom       int             // magnify by this many times, 0 or 1 leaves it alone
	Stats      bool            // also report what the shown pixels average to
	Stretch    bool            // map the region's own range to full scale
	Label      bool            // write the file name on each panel
	Out        string          // empty means LookPath()
}

func DefaultLook() LookOptions {
	return LookOptions{Max: 1400, Background: "checker", Label: true}
}

// LookDir is where composites go when nobody says otherwise.
//
// Not the working directory. This tool produces something to glance at and
// forget, and a glance should not leave a file in someone's repository for
// `git status` to find later.
func LookDir() string { return filepath.Join(os.TempDir(), "pixelita") }

// LookPath derives the file name from what is being shown.
//
// It used to be one fixed name, on the reasoning that a predictable path is
// worth more than a unique one. That was wrong in practice: taking several
// views of the same pair — the whole thing, then a region, then that region
// magnified — silently destroyed each previous one, and the caller had to
// remember -out every time or lose the work. Looking twice at the *same* thing
// should still overwrite, so the name is derived from the inputs and the
// options rather than from a counter or a clock: repeat a command and it lands
// on the same file, change anything and it does not.
func LookPath(paths []string, o LookOptions) string {
	h := fnv.New32a()
	for _, p := range paths {
		fmt.Fprintln(h, p)
	}
	fmt.Fprintf(h, "%v|%d|%d|%v|%v|%s", o.Crop, o.Zoom, o.Max, o.Across, o.Label, o.Background)

	name := "look"
	for i, p := range paths {
		if i == 2 {
			break // two names is enough to recognise; the hash does the rest
		}
		base := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
		if len(base) > 14 {
			base = base[:14]
		}
		name += "-" + base
	}
	return filepath.Join(LookDir(), fmt.Sprintf("%s-%08x.png", name, h.Sum32()))
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
	stretchLo, stretchHi := -1, -1

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
			cropped, err := cropTo(src, o.Crop)
			if err != nil {
				items = append(items, fail(item, err, "crop outside image"))
				continue
			}
			src = cropped
			item.Metrics["crop"] = Rect(o.Crop)
		}
		// Stats are taken here: after the crop, so they describe the region
		// asked about, and before the background, because compositing onto a
		// checkerboard would average in the checkerboard.
		if o.Stats {
			addStats(&item, src)
		}
		// Every panel is stretched by the FIRST panel's range, not its own.
		// Stretched individually they would each be mapped differently and the
		// comparison would be meaningless — which is the only thing anyone
		// wants this for.
		if o.Stretch {
			if stretchLo < 0 {
				stretchLo, stretchHi = lumaRange(src)
			}
			src = stretchTo(src, stretchLo, stretchHi)
			item.Metrics["stretched"] = fmt.Sprintf("%d..%d to 0..255", stretchLo, stretchHi)
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
		if o.Zoom > 1 {
			src = magnify(src, o.Zoom)
			item.Metrics["zoom"] = o.Zoom
		}
		// The label goes on last, at its own size. Drawn before the zoom it
		// would be magnified into unreadable blocks along with everything else.
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

// magnify replicates each pixel into an n by n block.
//
// Deliberately not resize.Resize with a nearest filter. That path converts to
// linear light as float32 and back, and although a nearest weight of exactly 1
// should survive the round trip, "should" is not good enough for the one tool
// whose entire purpose is showing a pixel as it actually is. Copying bytes is
// exact by construction, and faster.
func magnify(src *image.NRGBA, n int) *image.NRGBA {
	if n < 2 {
		return src
	}
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, w*n, h*n))
	for y := 0; y < h; y++ {
		// Build one magnified row, then copy it n times: the rows of a block
		// are identical, so the inner work is done once per source row.
		row := dst.Pix[dst.PixOffset(0, y*n) : dst.PixOffset(0, y*n)+w*n*4]
		for x := 0; x < w; x++ {
			s := src.Pix[src.PixOffset(src.Rect.Min.X+x, src.Rect.Min.Y+y):][:4]
			for k := 0; k < n; k++ {
				copy(row[(x*n+k)*4:], s)
			}
		}
		for k := 1; k < n; k++ {
			copy(dst.Pix[dst.PixOffset(0, y*n+k):], row)
		}
	}
	return dst
}

// addStats records what the region averages to.
//
// This exists because the alternative kept being invented on the spot: resize
// the region to one pixel and read it, or write a script. Both work; neither is
// something anyone should have to think of. Twice in one afternoon these three
// numbers caught a wrong reading of a picture that looked convincing.
//
// The mean is taken in linear light, by handing the region to the toolkit's own
// resampler and asking for one pixel. An arithmetic mean of sRGB values is the
// familiar mistake this repository avoids everywhere else, and on dark material
// it is not a rounding difference: the same shadow patch reads 32 19 9 averaged
// as light and 25 13 5 averaged as code values. Routing through Resize also
// makes the number agree with `img-resize -filter box -fit exact -width 1
// -height 1` by construction rather than by coincidence — a property a test can
// hold us to — and it weights each pixel by its alpha, so what cannot be seen
// cannot move the answer.
//
// The luma range is deliberately not linearised. It answers "did the darkest
// pixels get crushed towards black", which is a question about stored values.
func addStats(item *report.Item, src *image.NRGBA) {
	one := resize.Resize(src, 1, 1, resize.Box)
	m := one.NRGBAAt(one.Rect.Min.X, one.Rect.Min.Y)
	if m.A == 0 {
		item.Metrics["mean"] = "fully transparent"
		return
	}
	item.Metrics["mean"] = fmt.Sprintf("#%02x%02x%02x", m.R, m.G, m.B)
	item.Metrics["meanRGB"] = []int{int(m.R), int(m.G), int(m.B)}

	// How many distinct values each channel still uses here — the tonal
	// headroom left in the region.
	//
	// This is the honest form of "the file can no longer be edited". That claim
	// is usually demonstrated by applying some tone curve and counting colours
	// afterwards, which proves it but invites the reply that the curve was
	// chosen to suit. The cause needs no curve: a region holding 58 levels
	// where the original held 254 will band under any lift at all, and the two
	// numbers side by side say so without anyone having to agree on an edit.
	var occupied [3][256]bool
	for y := src.Rect.Min.Y; y < src.Rect.Max.Y; y++ {
		for x := src.Rect.Min.X; x < src.Rect.Max.X; x++ {
			c := src.NRGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			occupied[0][c.R], occupied[1][c.G], occupied[2][c.B] = true, true, true
		}
	}
	levels := make([]int, 3)
	for ch := range occupied {
		for _, on := range occupied[ch] {
			if on {
				levels[ch]++
			}
		}
	}
	item.Metrics["levels"] = levels

	lo, hi, seen := 255, 0, false
	for y := src.Rect.Min.Y; y < src.Rect.Max.Y; y++ {
		for x := src.Rect.Min.X; x < src.Rect.Max.X; x++ {
			c := src.NRGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			// Rec. 709 luma on sRGB values: this is for judging whether the
			// shadows moved, not for colour science.
			l := (2126*int(c.R) + 7152*int(c.G) + 722*int(c.B)) / 10000
			lo, hi, seen = min(lo, l), max(hi, l), true
		}
	}
	if seen {
		item.Metrics["luma"] = []int{lo, hi}
	}
}

// lumaRange is the darkest and brightest the region gets.
func lumaRange(src *image.NRGBA) (int, int) {
	lo, hi := 255, 0
	for y := src.Rect.Min.Y; y < src.Rect.Max.Y; y++ {
		for x := src.Rect.Min.X; x < src.Rect.Max.X; x++ {
			c := src.NRGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			l := (2126*int(c.R) + 7152*int(c.G) + 722*int(c.B)) / 10000
			lo, hi = min(lo, l), max(hi, l)
		}
	}
	return lo, hi
}

// stretchTo maps a range of levels onto the whole scale, the way the first
// thing anyone does to a flat photograph maps it.
//
// This is here because "the file can no longer be edited" is a claim that
// sounds like an opinion until someone sees it. A region quantised down to a
// few dozen levels looks fine until it is stretched, and then the steps between
// those levels open into visible bands. Applying a real tone curve would prove
// the same thing while inviting the argument that the curve was chosen to
// flatter the conclusion; stretching a region's own range to full scale has no
// parameter to choose, and it is what auto-levels does in every editor there
// is.
//
// One scale factor for all three channels, taken from luma, so that colour
// relationships survive: stretching each channel by its own range would shift
// the hue and produce an artefact of the measurement rather than of the file.
func stretchTo(src *image.NRGBA, lo, hi int) *image.NRGBA {
	if hi <= lo {
		return src
	}
	var lut [256]uint8
	scale := 255.0 / float64(hi-lo)
	for v := 0; v < 256; v++ {
		n := float64(v-lo) * scale
		lut[v] = uint8(min(max(n, 0), 255))
	}

	out := image.NewNRGBA(image.Rect(0, 0, src.Rect.Dx(), src.Rect.Dy()))
	for y := 0; y < out.Rect.Dy(); y++ {
		for x := 0; x < out.Rect.Dx(); x++ {
			c := src.NRGBAAt(src.Rect.Min.X+x, src.Rect.Min.Y+y)
			out.SetNRGBA(x, y, color.NRGBA{lut[c.R], lut[c.G], lut[c.B], c.A})
		}
	}
	return out
}

func cropTo(src *image.NRGBA, r image.Rectangle) (*image.NRGBA, error) {
	clipped := r.Add(src.Rect.Min).Intersect(src.Rect)
	if clipped.Empty() {
		return nil, fmt.Errorf("crop %dx%d at %d,%d lies outside the image (%dx%d)",
			r.Dx(), r.Dy(), r.Min.X, r.Min.Y, src.Rect.Dx(), src.Rect.Dy())
	}
	out := image.NewNRGBA(image.Rect(0, 0, clipped.Dx(), clipped.Dy()))
	draw.Draw(out, out.Rect, src, clipped.Min, draw.Src)
	return out, nil
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
		path = filepath.Join(LookDir(), "look.png")
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
