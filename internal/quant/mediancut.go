package quant

import (
	"cmp"
	"math"
	"slices"
)

// box is a contiguous range of the histogram and the palette entry it proposes.
type box struct {
	start, end int
	mean       colorf
	weight     float32
	err        float32 // total weighted squared distance from mean
}

// medianCut builds the starting palette: repeatedly take the box that is
// costing the most error and cut it in two.
//
// It reorders items in place.
func medianCut(items []histItem, maxColors int) []colorf {
	boxes := []box{newBox(items, 0, len(items))}

	for len(boxes) < maxColors {
		worst := -1
		for i := range boxes {
			if boxes[i].end-boxes[i].start < 2 || boxes[i].err <= 0 {
				continue
			}
			if worst < 0 || boxes[i].err > boxes[worst].err {
				worst = i
			}
		}
		if worst < 0 {
			break // every box holds a single colour; nothing left to gain
		}
		left, right := splitBox(items, boxes[worst])
		boxes[worst] = left
		boxes = append(boxes, right)
	}

	pal := make([]colorf, len(boxes))
	for i, b := range boxes {
		pal[i] = b.mean
	}
	return pal
}

// splitBox cuts a box along its widest axis.
//
// The cut is not made at the median. Sorting the box along that axis and
// scanning for the position that minimises the combined variance of the two
// halves costs one extra linear pass, and puts the boundary where the colours
// actually separate rather than where the pixel count happens to be even.
func splitBox(items []histItem, b box) (box, box) {
	seg := items[b.start:b.end]
	axis := widestAxis(seg)
	slices.SortFunc(seg, func(x, y histItem) int {
		return cmp.Compare(x.color.axis(axis), y.color.axis(axis))
	})

	var total accum
	for _, it := range seg {
		total.add(it)
	}

	var left accum
	best, bestErr := 0, math.Inf(1)
	for i := 0; i < len(seg)-1; i++ {
		left.add(seg[i])
		right := total.minus(left)
		if e := left.err() + right.err(); e < bestErr {
			bestErr, best = e, i+1
		}
	}

	return newBox(items, b.start, b.start+best), newBox(items, b.start+best, b.end)
}

func widestAxis(seg []histItem) uint8 {
	var a accum
	for _, it := range seg {
		a.add(it)
	}
	axis, widest := uint8(axisR), float64(-1)
	for ch := uint8(0); ch < 4; ch++ {
		if v := a.variance(ch); v > widest {
			axis, widest = ch, v
		}
	}
	return axis
}

func newBox(items []histItem, start, end int) box {
	var a accum
	for _, it := range items[start:end] {
		a.add(it)
	}
	return box{start: start, end: end, mean: a.mean(), weight: float32(a.w), err: float32(a.err())}
}

// accum collects the weighted moments a box needs. Sums run in float64: a
// histogram bucket can hold millions of pixels, and float32 stops counting
// somewhere short of that.
type accum struct {
	w     float64
	sum   [4]float64
	sumSq [4]float64
}

func (a *accum) add(it histItem) {
	w := float64(it.weight)
	a.w += w
	for ch := uint8(0); ch < 4; ch++ {
		v := float64(it.color.axis(ch))
		a.sum[ch] += w * v
		a.sumSq[ch] += w * v * v
	}
}

func (a accum) minus(b accum) accum {
	a.w -= b.w
	for ch := 0; ch < 4; ch++ {
		a.sum[ch] -= b.sum[ch]
		a.sumSq[ch] -= b.sumSq[ch]
	}
	return a
}

// err is the total weighted squared distance from the mean.
func (a accum) err() float64 {
	if a.w <= 0 {
		return 0
	}
	var e float64
	for ch := 0; ch < 4; ch++ {
		e += a.sumSq[ch] - a.sum[ch]*a.sum[ch]/a.w
	}
	if e < 0 {
		return 0 // rounding, not a negative variance
	}
	return e
}

func (a accum) variance(ch uint8) float64 {
	if a.w <= 0 {
		return 0
	}
	v := a.sumSq[ch]/a.w - (a.sum[ch]/a.w)*(a.sum[ch]/a.w)
	return math.Max(v, 0)
}

func (a accum) mean() colorf {
	if a.w <= 0 {
		return colorf{}
	}
	return colorf{
		r: float32(a.sum[axisR] / a.w),
		g: float32(a.sum[axisG] / a.w),
		b: float32(a.sum[axisB] / a.w),
		a: float32(a.sum[axisA] / a.w),
	}
}
