package quant

import (
	"runtime"
	"sync"
)

// refine moves the palette to where the pixels actually are.
//
// Median cut puts a colour at the centre of each box, but a box is a region the
// algorithm carved out, not the shape the colours in it actually have. Lloyd's
// algorithm fixes that: assign every histogram bucket to its nearest palette
// entry, move each entry to the weighted mean of what it captured, repeat.
//
// This is the step the off-the-shelf Go quantisers leave out, and it is the
// single largest difference in output quality between them and pngquant.
func refine(items []histItem, pal []colorf, rounds int) []colorf {
	if rounds <= 0 || len(items) <= len(pal) {
		return pal
	}

	workers := min(runtime.NumCPU(), 1+len(items)/8192)
	pal = append([]colorf(nil), pal...)
	sums := make([][]cluster, workers)
	for i := range sums {
		sums[i] = make([]cluster, len(pal))
	}

	// Palette entries barely move between rounds, so last round's answer is
	// almost always this round's answer too. Remembering it turns each lookup
	// into a confirmation rather than a search.
	assign := make([]int32, len(items))
	for i := range assign {
		assign[i] = -1
	}

	for r := 0; r < rounds; r++ {
		tree := newKDTree(pal)

		var wg sync.WaitGroup
		chunk := (len(items) + workers - 1) / workers
		for w := 0; w < workers; w++ {
			lo, hi := w*chunk, min((w+1)*chunk, len(items))
			if lo >= hi {
				continue
			}
			wg.Add(1)
			go func(acc []cluster, part []histItem, hints []int32) {
				defer wg.Done()
				clear(acc)
				for i, it := range part {
					idx, _ := tree.nearestHint(it.color, hints[i])
					hints[i] = idx
					acc[idx].add(it)
				}
			}(sums[w], items[lo:hi], assign[lo:hi])
		}
		wg.Wait()

		var moved float32
		for i := range pal {
			var c cluster
			for w := range sums {
				c.merge(sums[w][i])
			}
			if c.w == 0 {
				continue // nothing chose this colour; leave it where it is
			}
			next := c.mean()
			moved = max(moved, pal[i].euclid(next))
			pal[i] = next
		}
		if moved < 1e-9 {
			break // settled; further rounds would only burn time
		}
	}
	return pal
}

type cluster struct {
	r, g, b, a float64
	w          float64
}

func (c *cluster) add(it histItem) {
	w := float64(it.weight)
	c.r += w * float64(it.color.r)
	c.g += w * float64(it.color.g)
	c.b += w * float64(it.color.b)
	c.a += w * float64(it.color.a)
	c.w += w
}

func (c *cluster) merge(o cluster) {
	c.r += o.r
	c.g += o.g
	c.b += o.b
	c.a += o.a
	c.w += o.w
}

func (c cluster) mean() colorf {
	return colorf{
		r: float32(c.r / c.w),
		g: float32(c.g / c.w),
		b: float32(c.b / c.w),
		a: float32(c.a / c.w),
	}
}
