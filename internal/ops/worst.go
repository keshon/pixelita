package ops

import (
	"fmt"
	"image"
	"math"
	"sort"

	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/metric"
	"github.com/keshon/pixelita/internal/report"
)

// WorstRegions finds where two images disagree most, and says where in the
// coordinates -crop accepts.
//
// This exists because of how the job was actually being done. A whole-image
// figure says something is wrong; finding out where meant writing a difference
// map, opening it at a size a person or a model can look at, picking out the
// bright patches by eye, and multiplying those coordinates by the scale factor
// to get back to the full-size image. An agent asked to audit a file did
// exactly that, five times, by hand — and mislabelled one region because the
// arithmetic landed it somewhere it had not meant to look.
//
// None of that is judgement. It is a search over tiles for the lowest score,
// which is work a machine should be doing.
//
// Ranking is by SSIM rather than PSNR because the damage that matters here is
// usually structural: on the photograph that prompted this, the worst region by
// PSNR was ordinary while its SSIM had collapsed from 0.96 to 0.86 — the
// texture in the shadows had gone, and only SSIM said so. PSNR is reported
// alongside, so a region that is merely shifted in level still shows up.
func WorstRegions(a, b string, n, tile int) ([]report.Item, error) {
	imgA, _, _, err := imgio.Load(a)
	if err != nil {
		return nil, err
	}
	imgB, _, _, err := imgio.Load(b)
	if err != nil {
		return nil, err
	}
	x, y := imgio.ToNRGBA(imgA), imgio.ToNRGBA(imgB)
	if x.Rect.Dx() != y.Rect.Dx() || x.Rect.Dy() != y.Rect.Dy() {
		return nil, fmt.Errorf("%w: %dx%d and %dx%d", metric.ErrSize,
			x.Rect.Dx(), x.Rect.Dy(), y.Rect.Dx(), y.Rect.Dy())
	}

	w, h := x.Rect.Dx(), x.Rect.Dy()
	if tile <= 0 {
		tile = 256
	}
	// A tile larger than the image would return the whole thing and call it a
	// region; a handful of tiles cannot rank anything. Shrink until there is
	// something to compare.
	for tile > 32 && (w/tile < 3 || h/tile < 3) {
		tile /= 2
	}

	type scored struct {
		r        image.Rectangle
		psnr     float64
		ssim     float64
		worst    int
		p95, p99 int
	}
	var all []scored

	for ty := 0; ty+tile <= h; ty += tile {
		for tx := 0; tx+tile <= w; tx += tile {
			r := image.Rect(tx, ty, tx+tile, ty+tile)
			ca, err := cropTo(x, r)
			if err != nil {
				continue
			}
			cb, err := cropTo(y, r)
			if err != nil {
				continue
			}
			res, err := metric.Compare(ca, cb)
			if err != nil {
				continue
			}
			all = append(all, scored{r, res.PSNR, res.SSIM, res.MaxDelta, res.P95, res.P99})
		}
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("the image is too small to divide into regions")
	}

	sort.Slice(all, func(i, j int) bool { return all[i].ssim < all[j].ssim })
	if n <= 0 || n > len(all) {
		n = min(len(all), 5)
	}

	items := make([]report.Item, 0, n)
	for _, s := range all[:n] {
		item := report.Item{Path: a, Output: b, Status: report.StatusDone,
			Metrics: map[string]any{}}
		// Spelled the way -crop wants it, so the next command is a paste rather
		// than a transcription.
		item.Metrics["crop"] = Rect(s.r)
		item.Metrics["ssim"] = s.ssim
		item.Metrics["maxDelta"] = float64(s.worst)
		item.Metrics["p95"] = s.p95
		item.Metrics["p99"] = s.p99
		if !math.IsInf(s.psnr, 1) {
			item.Metrics["psnr"] = s.psnr
		}
		items = append(items, item)
	}
	return items, nil
}

// Rect prints a rectangle the way ParseRect reads one.
func Rect(r image.Rectangle) string {
	return fmt.Sprintf("%d,%d,%d,%d", r.Min.X, r.Min.Y, r.Dx(), r.Dy())
}
