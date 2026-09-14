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

// This file is about sets of regions rather than one region at a time.
//
// That distinction came out of watching the tools used rather than from
// designing them. Three agents given a real image to judge all did the same
// thing: pick several places in the frame, measure each, and lay the numbers
// side by side. "How far apart are these two files" is never the question that
// gets asked; the question is where, and how much, and whether the damage is
// spread evenly.
//
// Built one region at a time, that table costs a call per cell. One audit of a
// single photograph spent sixteen calls assembling what is, in the end, one
// table: five regions found, then measured one at a time, then measured again
// with a second binary because the tonal figures lived apart from the fidelity
// figures. Not one of those calls was a mistake. The tools were shaped for a
// question nobody asks.
//
// So the unit here is a set: find one with WorstRegions or FlattestRegions, or
// hand one over through -crop, and get a row per region carrying every number
// that belongs on it.

// RegionMeasure is everything reported about one rectangle.
type RegionMeasure struct {
	Rect   image.Rectangle
	PSNR   float64
	SSIM   float64
	Worst  int
	P95    int
	P99    int
	Before [3]int // tonal levels in the first image
	Levels [3]int // and in the second
}

// MeasureRegions measures every rectangle in one pass over the two files.
func MeasureRegions(a, b string, rects []image.Rectangle, wantLevels bool) ([]report.Item, error) {
	x, y, err := loadPair(a, b)
	if err != nil {
		return nil, err
	}
	items := make([]report.Item, 0, len(rects))
	for _, r := range rects {
		m, err := measureRegion(x, y, r, wantLevels)
		if err != nil {
			item := report.Item{Path: a, Output: b,
				Metrics: map[string]any{"crop": Rect(r)}}
			items = append(items, fail(item, err, "region error"))
			continue
		}
		items = append(items, regionItem(a, b, m, wantLevels))
	}
	return items, nil
}

// WorstRegions finds where two images disagree most, in the coordinates -crop
// accepts.
//
// This exists because of how the job was being done by hand: write a difference
// map, open it scaled down to something a reader can take in, pick the bright
// patches out by eye, and multiply those coordinates back up by the scale
// factor. That is a search over tiles for the lowest score, which is not
// judgement and should not be anyone's job.
//
// Ranking is by SSIM rather than PSNR because the damage that matters is
// usually structural: on the photograph that prompted this, the worst region by
// PSNR was unremarkable while its SSIM had fallen from 0.96 to 0.86 — the
// texture in the shadows had gone, and only SSIM said so. PSNR is reported
// beside it, so a region merely shifted in level still stands out.
func WorstRegions(a, b string, n, tile int, by Ranking, wantLevels bool) ([]report.Item, error) {
	// Ranking by headroom needs the counts whether or not they are reported.
	return rank(a, b, n, tile, wantLevels || by == RankLevels, func(m RegionMeasure) float64 {
		switch by {
		case RankPSNR:
			return m.PSNR
		case RankLevels:
			// The share of the original's levels that survive, so a region that
			// kept 24 of 64 ranks below one that kept 200 of 256.
			before := m.Before[0] + m.Before[1] + m.Before[2]
			if before == 0 {
				return 1
			}
			return float64(m.Levels[0]+m.Levels[1]+m.Levels[2]) / float64(before)
		default:
			return m.SSIM
		}
	})
}

// Ranking names how "worst" is decided, because there is more than one way for
// a file to be worse and they do not point at the same places.
type Ranking string

const (
	// RankSSIM is structure lost: texture gone, detail flattened. The default,
	// because it is what a reader notices and what PSNR misses.
	RankSSIM Ranking = "ssim"
	// RankPSNR is error in level, which catches a region merely shifted.
	RankPSNR Ranking = "psnr"
	// RankLevels is tonal headroom lost — how much of the range a region used
	// before against how much it uses now.
	//
	// This replaced a -flattest search that was written, measured, and thrown
	// away. The idea was to find smooth regions, on the reasoning that too few
	// levels band where the tone ramps gently. Measured on a real photograph it
	// pointed at the wrong half of the frame: a night exposure has heavy grain
	// in the sky and crushed, quiet shadows, so by any roughness measure the
	// shadows are the "smooth" part. Worse, the premise was already known to be
	// false here — a dithered file does not band at all, it trades the steps for
	// noise, which -stretch had shown before the search was written.
	//
	// Ranking by the headroom itself needs no proxy and no assumption about how
	// the loss will show.
	RankLevels Ranking = "levels"
)

func rank(a, b string, n, tile int, wantLevels bool,
	score func(RegionMeasure) float64) ([]report.Item, error) {

	x, y, err := loadPair(a, b)
	if err != nil {
		return nil, err
	}
	w, h := x.Rect.Dx(), x.Rect.Dy()
	if tile <= 0 {
		tile = 256
	}
	// A tile as large as the image returns the whole thing and calls it a
	// region; a handful of tiles cannot rank anything.
	for tile > 32 && (w/tile < 3 || h/tile < 3) {
		tile /= 2
	}

	type entry struct {
		m RegionMeasure
		s float64
	}
	var all []entry
	for ty := 0; ty+tile <= h; ty += tile {
		for tx := 0; tx+tile <= w; tx += tile {
			r := image.Rect(tx, ty, tx+tile, ty+tile)
			m, err := measureRegion(x, y, r, wantLevels)
			if err != nil {
				continue
			}
			all = append(all, entry{m, score(m)})
		}
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("the image is too small to divide into regions")
	}

	sort.Slice(all, func(i, j int) bool { return all[i].s < all[j].s })
	if n <= 0 || n > len(all) {
		n = min(len(all), 5)
	}

	items := make([]report.Item, 0, n)
	for _, e := range all[:n] {
		items = append(items, regionItem(a, b, e.m, wantLevels))
	}
	return items, nil
}

func loadPair(a, b string) (*image.NRGBA, *image.NRGBA, error) {
	imgA, _, _, err := imgio.Load(a)
	if err != nil {
		return nil, nil, err
	}
	imgB, _, _, err := imgio.Load(b)
	if err != nil {
		return nil, nil, err
	}
	x, y := imgio.ToNRGBA(imgA), imgio.ToNRGBA(imgB)
	if x.Rect.Dx() != y.Rect.Dx() || x.Rect.Dy() != y.Rect.Dy() {
		return nil, nil, fmt.Errorf("%w: %dx%d and %dx%d", metric.ErrSize,
			x.Rect.Dx(), x.Rect.Dy(), y.Rect.Dx(), y.Rect.Dy())
	}
	return x, y, nil
}

func measureRegion(x, y *image.NRGBA, r image.Rectangle, wantLevels bool) (RegionMeasure, error) {
	ca, err := cropTo(x, r)
	if err != nil {
		return RegionMeasure{}, err
	}
	cb, err := cropTo(y, r)
	if err != nil {
		return RegionMeasure{}, err
	}
	res, err := metric.Compare(ca, cb)
	if err != nil {
		return RegionMeasure{}, err
	}
	m := RegionMeasure{Rect: r, PSNR: res.PSNR, SSIM: res.SSIM,
		Worst: res.MaxDelta, P95: res.P95, P99: res.P99}
	// Tonal levels travel with the fidelity figures because they always end up
	// in the same table. Fetching them from a second binary for the same
	// rectangle was doubling the number of calls needed to say one thing.
	if wantLevels {
		m.Before, m.Levels = channelLevels(ca), channelLevels(cb)
	}
	return m, nil
}

func regionItem(a, b string, m RegionMeasure, wantLevels bool) report.Item {
	item := report.Item{Path: a, Output: b, Status: report.StatusDone,
		Metrics: map[string]any{}}
	// Spelled the way -crop reads it, so the next command is a paste rather
	// than a transcription.
	item.Metrics["crop"] = Rect(m.Rect)
	item.Metrics["ssim"] = m.SSIM
	item.Metrics["maxDelta"] = float64(m.Worst)
	item.Metrics["p95"] = m.P95
	item.Metrics["p99"] = m.P99
	if !math.IsInf(m.PSNR, 1) {
		item.Metrics["psnr"] = m.PSNR
	}
	if wantLevels {
		before, after := m.Before, m.Levels
		item.Metrics["levelsBefore"] = before[:]
		item.Metrics["levels"] = after[:]
	}
	return item
}

// Rect prints a rectangle the way ParseRect reads one.
func Rect(r image.Rectangle) string {
	return fmt.Sprintf("%d,%d,%d,%d", r.Min.X, r.Min.Y, r.Dx(), r.Dy())
}
