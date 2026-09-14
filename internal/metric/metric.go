// Package metric measures how far one image is from another.
//
// Two numbers, because they disagree in useful ways. PSNR is an average of
// squared error: it is objective, comparable between tools, and blind to
// structure — it cannot tell a little noise everywhere from a smudge in one
// place. SSIM compares local means, variances and covariance, which is closer
// to what an eye notices, and it is the one that catches a conversion that
// scored well and still looks wrong.
package metric

import (
	"errors"
	"image"
	"math"

	"github.com/keshon/pixelita/internal/imgio"
)

// Result is the full comparison between two images of the same size.
type Result struct {
	PSNR      float64 // dB, +Inf when identical
	PSNRAlpha float64 // dB, counting alpha via black and white backgrounds
	SSIM      float64 // 0..1
	MaxDelta  int     // largest single-channel difference, 0..255
	// P95 and P99 are the per-pixel error that 95% and 99% of pixels stay
	// under. MaxDelta alone cannot tell a single stray pixel from a systemic
	// shift across a tenth of the frame, and those two call for very different
	// answers — the first is noise, the second is damage.
	P95       int
	P99       int
	Different float64 // percent of pixels that differ at all
	Pixels    int
}

var ErrSize = errors.New("images have different dimensions")

// Compare measures b against a.
func Compare(a, b image.Image) (Result, error) {
	x, y := imgio.ToNRGBA(a), imgio.ToNRGBA(b)
	if x.Rect != y.Rect {
		return Result{}, ErrSize
	}
	w, h := x.Rect.Dx(), x.Rect.Dy()
	n := w * h
	if n == 0 {
		return Result{}, ErrSize
	}

	var sum, sumAlpha float64
	var differing int
	maxDelta := 0
	// One bucket per possible per-pixel error. Cheaper and more honest than
	// keeping every value: the answer wanted is a percentile, not a list.
	var hist [256]int
	for i := 0; i < len(x.Pix); i += 4 {
		pixelDelta := 0
		for c := 0; c < 3; c++ {
			d := int(x.Pix[i+c]) - int(y.Pix[i+c])
			if d < 0 {
				d = -d
			}
			if d > maxDelta {
				maxDelta = d
			}
			if d > pixelDelta {
				pixelDelta = d
			}
			f := float64(d) / 255
			sum += f * f
		}
		if d := int(x.Pix[i+3]) - int(y.Pix[i+3]); d != 0 {
			if d < 0 {
				d = -d
			}
			if d > maxDelta {
				maxDelta = d
			}
			if d > pixelDelta {
				pixelDelta = d
			}
		}
		hist[pixelDelta]++
		if x.Pix[i] != y.Pix[i] || x.Pix[i+1] != y.Pix[i+1] ||
			x.Pix[i+2] != y.Pix[i+2] || x.Pix[i+3] != y.Pix[i+3] {
			differing++
		}
		sumAlpha += alphaAware(x.Pix[i:i+4], y.Pix[i:i+4])
	}

	return Result{
		PSNR:      psnr(sum, n),
		PSNRAlpha: psnr(sumAlpha, n),
		SSIM:      ssim(x, y),
		MaxDelta:  maxDelta,
		P95:       percentile(&hist, n, 0.95),
		P99:       percentile(&hist, n, 0.99),
		Different: float64(differing) / float64(n) * 100,
		Pixels:    n,
	}, nil
}

// percentile walks the histogram to the first error value at or below which
// the given share of pixels falls.
func percentile(hist *[256]int, n int, share float64) int {
	want := int(float64(n) * share)
	seen := 0
	for v := 0; v < 256; v++ {
		seen += hist[v]
		if seen >= want {
			return v
		}
	}
	return 255
}

func psnr(sum float64, pixels int) float64 {
	if sum <= 0 {
		return math.Inf(1)
	}
	return 10 * math.Log10(3*float64(pixels)/sum)
}

// alphaAware measures the difference as it would be seen over a black
// background and over a white one, and takes the worse of the two. Comparing
// only the stored colour would call two fully transparent pixels of different
// hue different, and two barely-opaque ones of different hue the same.
func alphaAware(a, b []uint8) float64 {
	aa, ba := float64(a[3])/255, float64(b[3])/255
	da := ba - aa
	var sum float64
	for c := 0; c < 3; c++ {
		black := float64(a[c])/255*aa - float64(b[c])/255*ba
		white := black + da
		sum += math.Max(black*black, white*white)
	}
	return sum
}

// ssim computes mean structural similarity over the luma channel with the
// standard 8x8 sliding window.
func ssim(a, b *image.NRGBA) float64 {
	w, h := a.Rect.Dx(), a.Rect.Dy()
	la, lb := luma(a), luma(b)

	const (
		win = 8
		c1  = 0.01 * 0.01 // L = 1 after scaling to 0..1
		c2  = 0.03 * 0.03
	)
	if w < win || h < win {
		return 1
	}

	var total float64
	var count int
	for y := 0; y+win <= h; y += 4 {
		for x := 0; x+win <= w; x += 4 {
			var sa, sb, saa, sbb, sab float64
			for j := 0; j < win; j++ {
				row := (y+j)*w + x
				for i := 0; i < win; i++ {
					va, vb := la[row+i], lb[row+i]
					sa += va
					sb += vb
					saa += va * va
					sbb += vb * vb
					sab += va * vb
				}
			}
			const n = win * win
			ma, mb := sa/n, sb/n
			va := saa/n - ma*ma
			vb := sbb/n - mb*mb
			cov := sab/n - ma*mb
			total += ((2*ma*mb + c1) * (2*cov + c2)) /
				((ma*ma + mb*mb + c1) * (va + vb + c2))
			count++
		}
	}
	if count == 0 {
		return 1
	}
	return total / float64(count)
}

// luma flattens to perceptual brightness over white, so that a difference in
// transparency registers as a difference in the picture.
func luma(img *image.NRGBA) []float64 {
	out := make([]float64, img.Rect.Dx()*img.Rect.Dy())
	for i, j := 0, 0; i < len(img.Pix); i, j = i+4, j+1 {
		a := float64(img.Pix[i+3]) / 255
		r := float64(img.Pix[i])/255*a + (1 - a)
		g := float64(img.Pix[i+1])/255*a + (1 - a)
		b := float64(img.Pix[i+2])/255*a + (1 - a)
		out[j] = 0.2126*r + 0.7152*g + 0.0722*b
	}
	return out
}
