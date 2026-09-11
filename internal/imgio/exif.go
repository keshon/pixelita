package imgio

import (
	"encoding/binary"
	"image"
)

// Orientation is what EXIF says has to happen to the stored pixels before
// anyone looks at them.
//
// A camera that is held sideways does not rotate what it writes — it writes the
// sensor as it is and records a tag saying which way was up. Every viewer is
// expected to honour that tag. A tool that decodes the pixels, does something to
// them and encodes them again without honouring it produces a file that is
// sideways twice over: the pixels were never turned and the tag that would have
// turned them is gone.
//
// So the tag is applied at the point of decoding. From there on the image in
// memory is the image as a person would see it, and nothing downstream has to
// know that EXIF exists.
type Orientation uint16

const (
	OrientNormal     Orientation = 1
	OrientFlipH      Orientation = 2
	OrientRotate180  Orientation = 3
	OrientFlipV      Orientation = 4
	OrientTranspose  Orientation = 5
	OrientRotate90   Orientation = 6
	OrientTransverse Orientation = 7
	OrientRotate270  Orientation = 8
)

// Turns reports whether the orientation swaps width and height.
func (o Orientation) Turns() bool {
	return o == OrientTranspose || o == OrientRotate90 ||
		o == OrientTransverse || o == OrientRotate270
}

func (o Orientation) String() string {
	switch o {
	case OrientFlipH:
		return "flip horizontal"
	case OrientRotate180:
		return "rotate 180"
	case OrientFlipV:
		return "flip vertical"
	case OrientTranspose:
		return "transpose"
	case OrientRotate90:
		return "rotate 90"
	case OrientTransverse:
		return "transverse"
	case OrientRotate270:
		return "rotate 270"
	}
	return "normal"
}

// readOrientation digs the one tag we care about out of a JPEG's EXIF block.
//
// A full EXIF parser is not wanted here: the rest of the block is metadata we
// deliberately never touch, and walking only as far as tag 0x0112 in the first
// directory keeps this immune to the malformed tails real cameras produce.
func readOrientation(raw []byte) Orientation {
	app1 := findExif(raw)
	if app1 == nil {
		return OrientNormal
	}

	// TIFF header: the byte order, the number 42, then where the first
	// directory starts.
	if len(app1) < 8 {
		return OrientNormal
	}
	var order binary.ByteOrder
	switch string(app1[0:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return OrientNormal
	}
	if order.Uint16(app1[2:4]) != 42 {
		return OrientNormal
	}

	offset := int(order.Uint32(app1[4:8]))
	if offset < 8 || offset+2 > len(app1) {
		return OrientNormal
	}
	count := int(order.Uint16(app1[offset : offset+2]))
	entries := app1[offset+2:]

	for i := 0; i < count; i++ {
		e := i * 12
		if e+12 > len(entries) {
			break
		}
		if order.Uint16(entries[e:e+2]) != 0x0112 {
			continue
		}
		// A SHORT sits in the first two bytes of the value field, whichever
		// way round the file is.
		v := Orientation(order.Uint16(entries[e+8 : e+10]))
		if v >= OrientNormal && v <= OrientRotate270 {
			return v
		}
		return OrientNormal
	}
	return OrientNormal
}

// findExif returns the TIFF block inside the APP1 segment, or nil.
func findExif(raw []byte) []byte {
	if len(raw) < 4 || raw[0] != 0xFF || raw[1] != 0xD8 {
		return nil
	}
	for i := 2; i+4 <= len(raw); {
		if raw[i] != 0xFF {
			return nil
		}
		marker := raw[i+1]
		if marker == 0xD8 || marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			i += 2
			continue
		}
		if marker == 0xDA || marker == 0xD9 { // the scan begins; no more headers
			return nil
		}
		size := int(binary.BigEndian.Uint16(raw[i+2 : i+4]))
		if size < 2 || i+2+size > len(raw) {
			return nil
		}
		if marker == 0xE1 {
			p := raw[i+4 : i+2+size]
			if len(p) >= 6 && string(p[0:4]) == "Exif" && p[4] == 0 {
				return p[6:]
			}
		}
		i += 2 + size
	}
	return nil
}

// applyOrientation turns the pixels the way the tag asks.
//
// Done by moving pixels rather than by handing the tag downstream, because the
// tag does not survive re-encoding and half the formats we write cannot carry
// it at all.
func applyOrientation(src *image.NRGBA, o Orientation) *image.NRGBA {
	if o <= OrientNormal || o > OrientRotate270 {
		return src
	}
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dw, dh := w, h
	if o.Turns() {
		dw, dh = h, w
	}
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var nx, ny int
			switch o {
			case OrientFlipH:
				nx, ny = w-1-x, y
			case OrientRotate180:
				nx, ny = w-1-x, h-1-y
			case OrientFlipV:
				nx, ny = x, h-1-y
			case OrientTranspose:
				nx, ny = y, x
			case OrientRotate90:
				nx, ny = h-1-y, x
			case OrientTransverse:
				nx, ny = h-1-y, w-1-x
			case OrientRotate270:
				nx, ny = y, w-1-x
			}
			s := src.PixOffset(src.Rect.Min.X+x, src.Rect.Min.Y+y)
			d := dst.PixOffset(nx, ny)
			copy(dst.Pix[d:d+4], src.Pix[s:s+4])
		}
	}
	return dst
}
