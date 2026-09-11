package quant

import "image"

// histItem is one bucket of the colour histogram: the mean colour of every
// pixel that fell into it, and how much the palette search should care.
type histItem struct {
	color  colorf
	weight float32
}

// The histogram is an open-addressed table. Keeping it in flat slices rather
// than a map matters: a photograph can have millions of distinct colours, and
// the difference between a map and this is tens of seconds and hundreds of
// megabytes.
const (
	histSlots = 1 << 19
	histMax   = histSlots / 4 // load factor 0.25, probes stay short
)

type histSum struct {
	r, g, b, a uint64
	n          uint64
}

type histTable struct {
	keys  []uint32 // packed RGBA; only meaningful where sums[i].n > 0
	sums  []histSum
	mask  uint32 // packed per-channel mask, narrows as precision drops
	count int
	full  bool // true once precision has been dropped: counts are approximate
}

// A slot is free when nothing has been counted into it. Using the count rather
// than a sentinel key matters: every 32-bit value is a legal colour, opaque
// white included, so there is no key left over to mean "empty".
func (t *histTable) free(i uint32) bool { return t.sums[i].n == 0 }

func newHistTable() *histTable {
	return &histTable{
		keys: make([]uint32, histSlots),
		sums: make([]histSum, histSlots),
		mask: 0xFFFFFFFF,
	}
}

func hash(key uint32) uint32 {
	key *= 0x9E3779B1
	return key ^ (key >> 15)
}

func (t *histTable) add(r, g, b, a uint8) {
	// A fully transparent pixel has no colour. PNG files are full of them, and
	// they carry whatever RGB the editor left behind — often thousands of
	// distinct values that all render identically. Collapsing them here keeps
	// the histogram honest about how many colours the image really has.
	if a == 0 {
		r, g, b = 0, 0, 0
	}
	key := (uint32(r)<<24 | uint32(g)<<16 | uint32(b)<<8 | uint32(a)) & t.mask
	for i := hash(key) & (histSlots - 1); ; i = (i + 1) & (histSlots - 1) {
		if t.free(i) {
			t.keys[i] = key
			t.sums[i] = histSum{uint64(r), uint64(g), uint64(b), uint64(a), 1}
			t.count++
			if t.count > histMax {
				t.collapse()
			}
			return
		}
		if t.keys[i] == key {
			s := &t.sums[i]
			s.r += uint64(r)
			s.g += uint64(g)
			s.b += uint64(b)
			s.a += uint64(a)
			s.n++
			return
		}
	}
}

// collapse sheds precision until the table has comfortable headroom again.
// It reduces past the cap rather than to it, so a table sitting exactly on the
// limit does not rehash itself on every new colour.
func (t *histTable) collapse() {
	for t.count > histMax*3/4 {
		t.full = true
		t.reduce()
	}
}

// reduce drops one bit of precision from every channel and merges the buckets
// that become equal.
//
// Losing a bit sounds worse than it is. A bucket keeps the mean of the colours
// that landed in it, so two shades four levels apart out of 255 are replaced by
// the shade exactly between them — while the palette they are competing for has
// 256 entries for the whole image, each covering a span roughly ten times
// wider. The cap on buckets buys a bounded, predictable run; the precision it
// costs is below what the palette can represent anyway.
func (t *histTable) reduce() {
	t.mask &= t.mask << 1 & 0xFEFEFEFE

	keys, sums := t.keys, t.sums
	t.keys = make([]uint32, histSlots)
	t.sums = make([]histSum, histSlots)
	t.count = 0

	for i, old := range sums {
		if old.n == 0 {
			continue
		}
		key := keys[i] & t.mask
		for j := hash(key) & (histSlots - 1); ; j = (j + 1) & (histSlots - 1) {
			if t.free(j) {
				t.keys[j] = key
				t.sums[j] = old
				t.count++
				break
			}
			if t.keys[j] == key {
				s := &t.sums[j]
				s.r += old.r
				s.g += old.g
				s.b += old.b
				s.a += old.a
				s.n += old.n
				break
			}
		}
	}
}

func (t *histTable) items() []histItem {
	items := make([]histItem, 0, t.count)
	for _, s := range t.sums {
		if s.n == 0 {
			continue
		}
		// The bucket's colour is the rounded mean of what fell into it. Rounding
		// to eight bits costs nothing — the palette is written as eight-bit
		// colours anyway — and it keeps a bucket that saw a single colour
		// bit-identical to that colour, so an image that fits in the palette
		// comes back untouched.
		items = append(items, histItem{
			color:  fromNRGBA(mean8(s.r, s.n), mean8(s.g, s.n), mean8(s.b, s.n), mean8(s.a, s.n)),
			weight: float32(s.n),
		})
	}
	return items
}

func mean8(sum, n uint64) uint8 {
	return uint8((sum + n/2) / n)
}

// buildHistogram counts the colours of img, collapsing near-identical ones
// until the table fits. The second return value is false once that has
// happened, because from then on the bucket count is a floor, not a census.
func buildHistogram(img *image.NRGBA) ([]histItem, bool) {
	t := newHistTable()
	w := img.Rect.Dx()
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
		row := img.Pix[img.PixOffset(img.Rect.Min.X, y):][:w*4]
		for x := 0; x < len(row); x += 4 {
			t.add(row[x], row[x+1], row[x+2], row[x+3])
		}
	}
	return t.items(), !t.full
}
