package quant

import "image/color"

// Colour axes, in the order colorf stores them.
const (
	axisR = iota
	axisG
	axisB
	axisA
)

// colorf is a colour the quantiser can do arithmetic on: channels scaled to
// 0..1 and premultiplied by alpha.
//
// Premultiplication is not a convenience here, it is the point. Two fully
// transparent pixels of different hue are the same pixel on screen, and in this
// form they are literally equal — so no palette entry is ever spent telling
// them apart.
type colorf struct{ r, g, b, a float32 }

func fromNRGBA(r, g, b, a uint8) colorf {
	af := float32(a) * (1.0 / 255.0)
	s := af * (1.0 / 255.0)
	return colorf{float32(r) * s, float32(g) * s, float32(b) * s, af}
}

// nrgba converts back, undoing the premultiplication.
func (c colorf) nrgba() color.NRGBA {
	if c.a <= 0 {
		return color.NRGBA{}
	}
	f := 255 / c.a
	return color.NRGBA{R: clamp8(c.r * f), G: clamp8(c.g * f), B: clamp8(c.b * f), A: clamp8(c.a * 255)}
}

func clamp8(v float32) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 254.5 {
		return 255
	}
	return uint8(v + 0.5)
}

func (c colorf) axis(i uint8) float32 {
	switch i {
	case axisR:
		return c.r
	case axisG:
		return c.g
	case axisB:
		return c.b
	}
	return c.a
}

// diff is the squared distance between two colours: the worse of how they
// differ against a black background and against a white one.
//
// Comparing premultiplied values alone would call a 10%-opaque red and a
// 10%-opaque blue near-identical — which they are over black, and are not over
// white. Taking the larger of the two errors is what keeps semi-transparent
// colours from collapsing into each other, and it is the reason this metric is
// worth the three extra comparisons over plain Euclidean distance.
func (c colorf) diff(o colorf) float32 {
	da := o.a - c.a

	black := c.r - o.r
	white := black + da
	d := max(black*black, white*white)

	black = c.g - o.g
	white = black + da
	d += max(black*black, white*white)

	black = c.b - o.b
	white = black + da
	d += max(black*black, white*white)

	return d
}

// euclid is the plain squared distance, used where a cheap and symmetric
// measure of spread is enough: variance inside a box, and the lower bound that
// lets the nearest-colour search prune. It is never larger than diff, which is
// exactly what makes it safe to prune with.
func (c colorf) euclid(o colorf) float32 {
	dr, dg, db, da := c.r-o.r, c.g-o.g, c.b-o.b, c.a-o.a
	return dr*dr + dg*dg + db*db + da*da
}
