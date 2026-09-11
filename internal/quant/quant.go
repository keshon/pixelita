// Package quant reduces an image to a palette of at most 256 colours.
//
// The pipeline is the one libimagequant arrived at, for the reason it arrived
// at it: build a histogram, cut it into boxes along the axis of greatest
// variance, then stop trusting the boxes and refine the palette with weighted
// k-means, then remap with error diffusion. Median cut alone places colours
// where the histogram is wide; k-means moves them to where the pixels are.
package quant

import (
	"cmp"
	"image"
	"image/draw"
	"math"
	"slices"
)

type Options struct {
	MaxColors int     // 2..256
	Dither    float64 // 0 turns dithering off, 1 is full Floyd-Steinberg
	Effort    int     // 1..10, how long to spend refining the palette
}

type Result struct {
	Image  *image.Paletted
	Colors int
	PSNR   float64 // +Inf when nothing was lost

	// Distinct is how many different colours the source turned out to have,
	// and DistinctExact says whether that is a census or a floor: above roughly
	// a hundred thousand colours the histogram trades precision for a bounded
	// run, and from then on it can only undercount.
	Distinct      int
	DistinctExact bool
}

func Quantize(src image.Image, o Options) *Result {
	o.MaxColors = min(max(o.MaxColors, 2), 256)
	o.Effort = min(max(o.Effort, 1), 10)
	o.Dither = min(max(o.Dither, 0), 1)

	img := toNRGBA(src)
	items, exactCount := buildHistogram(img)

	// An image that already fits in the palette keeps every colour it has.
	// Screenshots, icons and anything previously quantised land here, and for
	// them the whole operation is lossless.
	exact := len(items) <= o.MaxColors

	var pal []colorf
	dither := float32(o.Dither)
	if exact {
		pal = make([]colorf, len(items))
		for i, it := range items {
			pal[i] = it.color
		}
		dither = 0
	} else {
		pal = medianCut(items, o.MaxColors)
		pal = refine(items, pal, o.Effort*2)
	}

	// PNG keeps palette alpha in a tRNS chunk running from index zero to the
	// last entry that is not fully opaque, so every transparent colour moved to
	// the front is a byte that chunk does not have to carry.
	slices.SortFunc(pal, func(x, y colorf) int { return cmp.Compare(x.a, y.a) })

	out, sum := remap(img, pal, dither)

	n := float64(img.Rect.Dx() * img.Rect.Dy())
	psnr := math.Inf(1)
	if sum > 0 {
		psnr = 10 * math.Log10(3*n/sum)
	}
	return &Result{Image: out, Colors: len(pal), PSNR: psnr, Distinct: len(items), DistinctExact: exactCount}
}

func toNRGBA(src image.Image) *image.NRGBA {
	if img, ok := src.(*image.NRGBA); ok && img.Rect.Min == (image.Point{}) {
		return img
	}
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}
