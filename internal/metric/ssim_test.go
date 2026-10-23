package metric

import (
	"fmt"
	"image"
	"math/rand"
	"testing"
)

// Keep the former full-frame algorithm as an independent oracle for the
// rolling buffers, including short windows and overlapping row boundaries.
func TestRollingSSIMMatchesFullFrame(t *testing.T) {
	rng := rand.New(rand.NewSource(17))
	sizes := []image.Point{{1, 1}, {1, 17}, {2, 19}, {3, 21}, {4, 23}, {7, 31}, {8, 8}, {8, 13}, {9, 17}, {17, 9}, {63, 97}, {128, 65}}
	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size.X, size.Y), func(t *testing.T) {
			a := image.NewNRGBA(image.Rect(0, 0, size.X, size.Y))
			b := image.NewNRGBA(a.Rect)
			rng.Read(a.Pix)
			rng.Read(b.Pix)
			want := fullFrameSSIM(a, b)
			if got := ssim(a, b); got != want {
				t.Fatalf("SSIM changed: got %.17g want %.17g", got, want)
			}
			if got := ssim(a, a); got != 1 {
				t.Fatalf("identical image: %.17g", got)
			}
		})
	}
}

func BenchmarkSSIMRows(b *testing.B) {
	for _, h := range []int{256, 4096} {
		b.Run(fmt.Sprintf("1024x%d", h), func(b *testing.B) {
			a := image.NewNRGBA(image.Rect(0, 0, 1024, h))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ssim(a, a)
			}
		})
	}
}

func fullFrameSSIM(a, b *image.NRGBA) float64 {
	w, h := a.Rect.Dx(), a.Rect.Dy()
	la, lb := fullFrameLuma(a), fullFrameLuma(b)

	const (
		c1 = 0.01 * 0.01 // L = 1 after scaling to 0..1
		c2 = 0.03 * 0.03
	)
	win := min(8, w, h)

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
			n := float64(win * win)
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
func fullFrameLuma(img *image.NRGBA) []float64 {
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
