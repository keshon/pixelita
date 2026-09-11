package quant

import (
	"cmp"
	"math"
	"slices"
)

// kdTree answers "which palette entry is closest to this colour" exactly,
// without testing every entry.
//
// Exactness is worth insisting on. The usual shortcut — a coarse lookup cube
// filled in lazily — is wrong near cell boundaries, and dithering walks colours
// straight along those boundaries, so the error it introduces is not random
// noise but a bias that the error diffusion then spreads across the image.
type kdTree struct {
	nodes []kdNode
	pal   []colorf
	root  int32
}

type kdNode struct {
	index       int32
	left, right int32
	split       float32
	axis        uint8
}

func newKDTree(pal []colorf) *kdTree {
	t := &kdTree{pal: pal, nodes: make([]kdNode, 0, len(pal))}
	idx := make([]int32, len(pal))
	for i := range idx {
		idx[i] = int32(i)
	}
	t.root = t.build(idx)
	return t
}

func (t *kdTree) build(idx []int32) int32 {
	if len(idx) == 0 {
		return -1
	}
	axis := t.widestAxis(idx)
	slices.SortFunc(idx, func(x, y int32) int {
		return cmp.Compare(t.pal[x].axis(axis), t.pal[y].axis(axis))
	})

	mid := len(idx) / 2
	n := int32(len(t.nodes))
	t.nodes = append(t.nodes, kdNode{index: idx[mid], axis: axis, split: t.pal[idx[mid]].axis(axis)})

	left := t.build(idx[:mid])
	right := t.build(idx[mid+1:])
	t.nodes[n].left, t.nodes[n].right = left, right
	return n
}

func (t *kdTree) widestAxis(idx []int32) uint8 {
	var lo, hi [4]float32
	for ch := range lo {
		lo[ch], hi[ch] = math.MaxFloat32, -math.MaxFloat32
	}
	for _, i := range idx {
		for ch := uint8(0); ch < 4; ch++ {
			v := t.pal[i].axis(ch)
			lo[ch] = min(lo[ch], v)
			hi[ch] = max(hi[ch], v)
		}
	}
	axis, widest := uint8(axisR), float32(-1)
	for ch := uint8(0); ch < 4; ch++ {
		if w := hi[ch] - lo[ch]; w > widest {
			axis, widest = ch, w
		}
	}
	return axis
}

func (t *kdTree) nearest(c colorf) (int32, float32) {
	return t.nearestHint(c, -1)
}

// nearestHint starts from a colour that is probably already the answer.
//
// A pruning search is only as good as the best match it has found so far, and
// it starts with none. Neighbouring pixels almost always land on the same
// palette entry, so handing the previous one in as a starting guess makes the
// first bound tight and lets most of the tree be dismissed without ever being
// visited. The result is identical — a guess can only narrow the search, never
// change what it finds.
func (t *kdTree) nearestHint(c colorf, hint int32) (int32, float32) {
	best, bestDist := int32(0), float32(math.MaxFloat32)
	if hint >= 0 {
		best, bestDist = hint, c.diff(t.pal[hint])
		if bestDist == 0 {
			return best, 0 // an exact match; nothing can beat it
		}
	}
	q := [4]float32{c.r, c.g, c.b, c.a}
	t.search(t.root, c, q, [4]float32{}, 0, &best, &bestDist)
	return best, bestDist
}

// search carries the distance to the region it is standing in, rather than
// recomputing a bound from the current split alone.
//
// The difference is not small. A bound from one axis only rules out a subtree
// when that single axis already separates it far enough; an accumulated bound
// adds up the gaps along every axis crossed on the way down, so most of the
// tree is dismissed near the root. Alpha is kept out of that sum — the metric
// bounds it separately, not additively — and folded in as its own bound.
func (t *kdTree) search(n int32, c colorf, q, off [4]float32, rd float32, best *int32, bestDist *float32) {
	for n >= 0 {
		node := &t.nodes[n]
		if d := c.diff(t.pal[node.index]); d < *bestDist {
			*bestDist, *best = d, node.index
		}

		ax := node.axis
		delta := q[ax] - node.split
		near, far := node.left, node.right
		if delta > 0 {
			near, far = node.right, node.left
		}

		farRD := rd
		if ax != axisA {
			farRD += delta*delta - off[ax]*off[ax]
		}
		farOff := off
		farOff[ax] = delta

		bound := max(farRD, 0.75*farOff[axisA]*farOff[axisA])
		if bound < *bestDist {
			t.search(far, c, q, farOff, farRD, best, bestDist)
		}
		n = near
	}
}
