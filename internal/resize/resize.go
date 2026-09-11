// Package resize scales images in linear light.
//
// Resampling is an average of neighbouring pixels, and sRGB values are not
// proportional to light — 128 is not half as bright as 255, it is about a
// fifth. Averaging them directly therefore darkens every image it touches, most
// visibly wherever fine detail alternates between light and dark: thin text,
// hairlines, foliage. Converting to linear light first, averaging there and
// converting back costs two table lookups per channel and removes the problem.
// Most tools do not do it, and it is the difference you can see.
//
// Alpha is premultiplied for the same class of reason: without it the colour of
// fully transparent pixels leaks into the visible edge next to them.
package resize

import (
	"image"
	"math"
)

// Filter is a reconstruction kernel: how much a source pixel at a given
// distance contributes to a destination pixel.
type Filter struct {
	Name   string
	Radius float64
	At     func(x float64) float64
}

var (
	// Nearest picks one source pixel. Only useful for pixel art.
	Nearest = Filter{"nearest", 0.5, func(x float64) float64 {
		if x > -0.5 && x <= 0.5 {
			return 1
		}
		return 0
	}}

	// Box averages the source pixels that fall inside the destination pixel.
	Box = Filter{"box", 0.5, func(x float64) float64 {
		if x >= -0.5 && x < 0.5 {
			return 1
		}
		return 0
	}}

	// Triangle is linear interpolation. Soft, but never rings.
	Triangle = Filter{"triangle", 1, func(x float64) float64 {
		x = math.Abs(x)
		if x < 1 {
			return 1 - x
		}
		return 0
	}}

	// CatmullRom is sharp without much ringing; a good default for photos.
	CatmullRom = Filter{"catmull-rom", 2, func(x float64) float64 {
		x = math.Abs(x)
		switch {
		case x < 1:
			return 1.5*x*x*x - 2.5*x*x + 1
		case x < 2:
			return -0.5*x*x*x + 2.5*x*x - 4*x + 2
		}
		return 0
	}}

	// Lanczos3 keeps the most detail. It can ring on hard edges, which on
	// screenshots and flat-shaded graphics shows as a faint halo.
	Lanczos3 = Filter{"lanczos", 3, func(x float64) float64 {
		x = math.Abs(x)
		if x < 1e-8 {
			return 1
		}
		if x >= 3 {
			return 0
		}
		px := math.Pi * x
		return 3 * math.Sin(px) * math.Sin(px/3) / (px * px)
	}}
)

var filters = map[string]Filter{
	"nearest": Nearest, "box": Box, "triangle": Triangle,
	"catmull-rom": CatmullRom, "lanczos": Lanczos3,
}

// FilterByName looks up a filter for a command-line flag.
func FilterByName(name string) (Filter, bool) {
	f, ok := filters[name]
	return f, ok
}

// FilterNames lists what FilterByName accepts.
func FilterNames() []string {
	return []string{"nearest", "box", "triangle", "catmull-rom", "lanczos"}
}

// Resize scales src to exactly w by h.
func Resize(src *image.NRGBA, w, h int, f Filter) *image.NRGBA {
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	if w <= 0 || h <= 0 || sw == 0 || sh == 0 {
		return image.NewNRGBA(image.Rect(0, 0, max(w, 0), max(h, 0)))
	}
	if w == sw && h == sh {
		out := image.NewNRGBA(image.Rect(0, 0, w, h))
		copy(out.Pix, src.Pix)
		return out
	}

	lin := toLinear(src)
	rows := resample(lin, sw, sh, w, f)  // horizontal: sw -> w
	cols := resampleT(rows, w, sh, h, f) // vertical: sh -> h
	return fromLinear(cols, w, h)
}

// contribution is the set of source samples one destination sample averages.
type contribution struct {
	start   int
	weights []float32
}

// weightsFor precomputes the contributions for one axis.
//
// When shrinking, the kernel is widened to the destination pixel spacing rather
// than left at source spacing. That is what makes downscaling an average of
// everything being discarded instead of a sparse sample of it, and it is why
// this does not alias where a naive implementation would.
func weightsFor(srcSize, dstSize int, f Filter) []contribution {
	scale := float64(dstSize) / float64(srcSize)
	spread := 1.0
	if scale < 1 {
		spread = 1 / scale
	}
	support := f.Radius * spread

	out := make([]contribution, dstSize)
	for i := 0; i < dstSize; i++ {
		center := (float64(i)+0.5)/scale - 0.5
		start := int(math.Ceil(center - support))
		end := int(math.Floor(center + support))
		if start < 0 {
			start = 0
		}
		if end > srcSize-1 {
			end = srcSize - 1
		}
		if end < start {
			// The kernel fell between two samples; take the nearest one.
			start = min(max(int(math.Round(center)), 0), srcSize-1)
			end = start
		}

		w := make([]float32, end-start+1)
		var sum float64
		for j := start; j <= end; j++ {
			v := f.At((float64(j) - center) / spread)
			w[j-start] = float32(v)
			sum += v
		}
		if sum != 0 {
			for k := range w {
				w[k] = float32(float64(w[k]) / sum)
			}
		}
		out[i] = contribution{start: start, weights: w}
	}
	return out
}

// resample scales along x, writing rows of dstW pixels.
func resample(src []float32, sw, sh, dw int, f Filter) []float32 {
	cs := weightsFor(sw, dw, f)
	dst := make([]float32, dw*sh*4)
	for y := 0; y < sh; y++ {
		row := src[y*sw*4:]
		out := dst[y*dw*4:]
		for x, c := range cs {
			var r, g, b, a float32
			base := c.start * 4
			for k, wt := range c.weights {
				i := base + k*4
				r += row[i] * wt
				g += row[i+1] * wt
				b += row[i+2] * wt
				a += row[i+3] * wt
			}
			out[x*4], out[x*4+1], out[x*4+2], out[x*4+3] = r, g, b, a
		}
	}
	return dst
}

// resampleT scales along y for an image that is w wide and sh tall.
func resampleT(src []float32, w, sh, dh int, f Filter) []float32 {
	cs := weightsFor(sh, dh, f)
	dst := make([]float32, w*dh*4)
	for y, c := range cs {
		out := dst[y*w*4:]
		for x := 0; x < w; x++ {
			var r, g, b, a float32
			for k, wt := range c.weights {
				i := ((c.start+k)*w + x) * 4
				r += src[i] * wt
				g += src[i+1] * wt
				b += src[i+2] * wt
				a += src[i+3] * wt
			}
			out[x*4], out[x*4+1], out[x*4+2], out[x*4+3] = r, g, b, a
		}
	}
	return dst
}

var srgbToLinear = func() [256]float32 {
	var t [256]float32
	for i := range t {
		c := float64(i) / 255
		if c <= 0.04045 {
			t[i] = float32(c / 12.92)
		} else {
			t[i] = float32(math.Pow((c+0.055)/1.055, 2.4))
		}
	}
	return t
}()

func toLinear(src *image.NRGBA) []float32 {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	out := make([]float32, w*h*4)
	for y := 0; y < h; y++ {
		row := src.Pix[src.PixOffset(src.Rect.Min.X, src.Rect.Min.Y+y):][: w*4 : w*4]
		dst := out[y*w*4:]
		for x := 0; x < w*4; x += 4 {
			a := float32(row[x+3]) * (1.0 / 255.0)
			dst[x] = srgbToLinear[row[x]] * a
			dst[x+1] = srgbToLinear[row[x+1]] * a
			dst[x+2] = srgbToLinear[row[x+2]] * a
			dst[x+3] = a
		}
	}
	return out
}

func fromLinear(src []float32, w, h int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i, j := 0, 0; i < len(src); i, j = i+4, j+4 {
		a := clamp01(src[i+3])
		if a <= 0 {
			continue // already zeroed, and there is no colour to recover
		}
		out.Pix[j] = linearToSRGB(src[i] / a)
		out.Pix[j+1] = linearToSRGB(src[i+1] / a)
		out.Pix[j+2] = linearToSRGB(src[i+2] / a)
		out.Pix[j+3] = uint8(a*255 + 0.5)
	}
	return out
}

func linearToSRGB(v float32) uint8 {
	c := float64(clamp01(v))
	if c <= 0.0031308 {
		c *= 12.92
	} else {
		c = 1.055*math.Pow(c, 1/2.4) - 0.055
	}
	return uint8(c*255 + 0.5)
}

func clamp01(v float32) float32 { return min(max(v, 0), 1) }

// Fit computes the target size for a box, keeping the aspect ratio unless it is
// told not to. A zero width or height means "whatever the aspect ratio says".
//
//	inside  — largest size that fits inside the box
//	outside — smallest size that covers the box
//	exact   — the box, aspect ratio be damned
func Fit(sw, sh, w, h int, mode string) (int, int) {
	if sw <= 0 || sh <= 0 {
		return 0, 0
	}
	switch {
	case w <= 0 && h <= 0:
		return sw, sh
	case w <= 0:
		return round(float64(h) * float64(sw) / float64(sh)), h
	case h <= 0:
		return w, round(float64(w) * float64(sh) / float64(sw))
	}

	if mode == "exact" {
		return w, h
	}
	sx := float64(w) / float64(sw)
	sy := float64(h) / float64(sh)
	scale := math.Min(sx, sy)
	if mode == "outside" {
		scale = math.Max(sx, sy)
	}
	return max(round(float64(sw)*scale), 1), max(round(float64(sh)*scale), 1)
}

// Crop takes a centred rectangle of w by h.
func Crop(src *image.NRGBA, w, h int) *image.NRGBA {
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	w, h = min(w, sw), min(h, sh)
	x0, y0 := (sw-w)/2, (sh-h)/2

	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		copy(out.Pix[y*out.Stride:(y+1)*out.Stride],
			src.Pix[src.PixOffset(src.Rect.Min.X+x0, src.Rect.Min.Y+y0+y):])
	}
	return out
}

func round(v float64) int { return int(math.Round(v)) }
