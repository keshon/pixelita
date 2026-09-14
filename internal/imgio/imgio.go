// Package imgio is the file layer shared by every tool: decoding what we
// support, encoding what we produce, and reading what a file's header says
// without decoding it at all.
package imgio

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	_ "image/gif"
	_ "image/jpeg"

	xwebp "golang.org/x/image/webp"
)

// Extensions are the files the tools are willing to look at.
var Extensions = []string{".png", ".jpg", ".jpeg", ".gif", ".webp"}

// Decode reads an image from bytes and reports the format it was in.
func Decode(raw []byte) (image.Image, string, error) {
	if isWebP(raw) {
		img, err := decodeWebP(raw)
		return img, "webp", err
	}
	img, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil || format != "jpeg" {
		return img, format, err
	}
	// A photograph taken sideways carries its rotation as a tag rather than in
	// its pixels. Applying it here means everything downstream works on the
	// image as a person would see it — which matters because none of what we
	// write back carries the tag onward.
	if o := readOrientation(raw); o != OrientNormal {
		img = applyOrientation(ToNRGBA(img), o)
	}
	return img, format, nil
}

func isWebP(raw []byte) bool {
	return len(raw) >= 12 && string(raw[:4]) == "RIFF" && string(raw[8:12]) == "WEBP"
}

// decodeWebP works around a colour-space mismatch in the standard Go decoder.
//
// VP8 keeps luma and chroma in the studio range BT.601 specifies — 16 to 235
// for luma — and libwebp, and therefore every browser, converts back on that
// basis. golang.org/x/image/webp hands the planes over as an image.YCbCr, whose
// colour model is the full-range JPEG one, so white comes back as 237 instead
// of 255 and every lossy WebP reads as washed towards grey.
//
// This was found by pointing img-diff at our own WebP output and not believing
// the answer: 23 dB where 40 was expected, and flat across every quality
// setting, which is the signature of a constant offset rather than of loss.
// Chrome decodes the same files byte for byte identically to their source, so
// the files are fine and the reading was wrong. Converting the planes with
// VP8's own coefficients reproduces what a browser shows.
func decodeWebP(raw []byte) (image.Image, error) {
	img, err := xwebp.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	switch m := img.(type) {
	case *image.YCbCr:
		return vp8ToNRGBA(m, nil, 0), nil
	case *image.NYCbCrA:
		return vp8ToNRGBA(&m.YCbCr, m.A, m.AStride), nil
	}
	return img, nil // lossless WebP arrives as RGBA already
}

// vp8ToNRGBA applies libwebp's own fixed-point conversion, so the result is the
// same bytes a browser would paint.
func vp8ToNRGBA(m *image.YCbCr, alpha []uint8, aStride int) *image.NRGBA {
	b := m.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			yi := m.YOffset(b.Min.X+x, b.Min.Y+y)
			ci := m.COffset(b.Min.X+x, b.Min.Y+y)
			r, g, bl := vp8YUVToRGB(int(m.Y[yi]), int(m.Cb[ci]), int(m.Cr[ci]))

			o := out.PixOffset(x, y)
			out.Pix[o], out.Pix[o+1], out.Pix[o+2] = r, g, bl
			out.Pix[o+3] = 255
			if alpha != nil {
				out.Pix[o+3] = alpha[(b.Min.Y+y-b.Min.Y)*aStride+x]
			}
		}
	}
	return out
}

func vp8YUVToRGB(y, u, v int) (uint8, uint8, uint8) {
	return clip8(mulHi(y, 19077) + mulHi(v, 26149) - 14234),
		clip8(mulHi(y, 19077) - mulHi(u, 6419) - mulHi(v, 13320) + 8708),
		clip8(mulHi(y, 19077) + mulHi(u, 33050) - 17685)
}

func mulHi(v, coeff int) int { return (v * coeff) >> 8 }

func clip8(v int) uint8 {
	v >>= 6
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// Load reads a file and decodes it, returning the raw bytes too — the tools all
// need the original size, and re-reading the file to get it is silly.
func Load(path string) (image.Image, string, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", nil, err
	}
	img, format, err := Decode(raw)
	if err != nil {
		return nil, "", raw, err
	}
	return img, format, raw, nil
}

// ToNRGBA converts to the one layout every tool works in: eight bits per
// channel, alpha not premultiplied, origin at zero.
func ToNRGBA(src image.Image) *image.NRGBA {
	if img, ok := src.(*image.NRGBA); ok && img.Rect.Min == (image.Point{}) {
		return img
	}
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

// EncodePalettedPNG writes a paletted image with the encoder settings the tools
// agree on. Go's encoder picks the narrowest bit depth the palette allows and
// trims the transparency chunk after the last non-opaque entry, so a palette
// sorted with its transparent colours first comes out smaller for free.
func EncodePalettedPNG(img *image.Paletted) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// EncodePNG writes any image as PNG with the same settings.
func EncodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// HasExt reports whether the path is one we handle.
func HasExt(path string, exts ...string) bool {
	if len(exts) == 0 {
		exts = Extensions
	}
	ext := strings.ToLower(filepath.Ext(path))
	for _, e := range exts {
		if ext == e {
			return true
		}
	}
	return false
}

// PNG colour types, as stored in byte 25 of the IHDR chunk.
const (
	PNGGray      = 0
	PNGRGB       = 2
	PNGPalette   = 3
	PNGGrayAlpha = 4
	PNGRGBA      = 6
)

var pngColourNames = map[byte]string{
	PNGGray: "gray", PNGRGB: "rgb", PNGPalette: "palette",
	PNGGrayAlpha: "gray+a", PNGRGBA: "rgba",
}

// Header is what a file says about itself before anything is decoded.
type Header struct {
	Format   string
	Width    int
	Height   int
	BitDepth int
	// Palette is how many entries a palette PNG actually carries. Colour type
	// alone says a palette is in use; it does not say whether the encoder spent
	// 256 entries or 64, and that difference is the whole story when judging
	// what a quantiser did to a file.
	Palette int
	// Chunks names the ancillary blocks a PNG carries. A byte count alone
	// raises a question it cannot answer: "metadata 176 bytes against 29" reads
	// as though something was thrown away, and the reader has to go outside the
	// toolkit with a script to find out that what changed was a colour-space
	// declaration being restated in a shorter form. The names answer it.
	Chunks      []string
	ColourType  string
	HasAlpha    bool
	Interlaced  bool
	Progressive bool        // JPEG only
	Orientation Orientation // JPEG only; already applied by Decode
	Ancillary   int64       // bytes of metadata and other non-essential chunks
}

// ReadHeader parses what it can without decoding pixels. Walking the chunk
// table is orders of magnitude cheaper than decoding, and for most questions —
// is this already a palette image, how big is it, how much of the file is
// metadata — it is the whole answer.
func ReadHeader(raw []byte) (Header, error) {
	switch {
	case bytes.HasPrefix(raw, []byte("\x89PNG\r\n\x1a\n")):
		return pngHeader(raw)
	case bytes.HasPrefix(raw, []byte{0xFF, 0xD8}):
		return jpegHeader(raw)
	}
	img, format, err := Decode(raw)
	if err != nil {
		return Header{}, err
	}
	b := img.Bounds()
	return Header{Format: format, Width: b.Dx(), Height: b.Dy(), BitDepth: 8,
		ColourType: "rgba", HasAlpha: true}, nil
}

func pngHeader(raw []byte) (Header, error) {
	if len(raw) < 33 || string(raw[12:16]) != "IHDR" {
		return Header{}, fmt.Errorf("png: truncated header")
	}
	ct := raw[25]
	h := Header{
		Format:     "png",
		Width:      int(binary.BigEndian.Uint32(raw[16:20])),
		Height:     int(binary.BigEndian.Uint32(raw[20:24])),
		BitDepth:   int(raw[24]),
		ColourType: pngColourNames[ct],
		HasAlpha:   ct == PNGRGBA || ct == PNGGrayAlpha,
		Interlaced: raw[28] != 0,
	}

	// Walk the chunks. Everything outside the handful that carry the image is
	// weight the file does not need, and editors leave a lot of it behind.
	for off := 8; off+12 <= len(raw); {
		size := int(binary.BigEndian.Uint32(raw[off : off+4]))
		if size < 0 || off+12+size > len(raw) {
			break
		}
		name := string(raw[off+4 : off+8])
		switch name {
		case "IHDR", "PLTE", "IDAT", "IEND", "tRNS":
		default:
			h.Ancillary += int64(size + 12)
		}
		if name == "PLTE" {
			h.Palette = size / 3 // three bytes an entry, by the specification
		}
		switch name {
		case "IHDR", "IDAT", "IEND":
		default:
			h.Chunks = append(h.Chunks, name)
		}
		if name == "tRNS" {
			h.HasAlpha = true
		}
		off += 12 + size
		if name == "IEND" {
			break
		}
	}
	return h, nil
}

func jpegHeader(raw []byte) (Header, error) {
	h := Header{Format: "jpeg", BitDepth: 8, ColourType: "ycbcr"}
	for off := 2; off+4 <= len(raw); {
		if raw[off] != 0xFF {
			break
		}
		marker := raw[off+1]
		if marker == 0xD8 || marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			off += 2
			continue
		}
		if marker == 0xDA || marker == 0xD9 { // start of scan; pixels from here
			break
		}
		size := int(binary.BigEndian.Uint16(raw[off+2 : off+4]))
		if size < 2 || off+2+size > len(raw) {
			break
		}
		switch {
		case marker == 0xC2:
			h.Progressive = true
			fallthrough
		case marker >= 0xC0 && marker <= 0xCF && marker != 0xC4 && marker != 0xC8 && marker != 0xCC:
			if size >= 7 {
				h.Height = int(binary.BigEndian.Uint16(raw[off+5 : off+7]))
				h.Width = int(binary.BigEndian.Uint16(raw[off+7 : off+9]))
			}
		case marker >= 0xE0 && marker <= 0xEF, marker == 0xFE: // APPn and comments
			h.Ancillary += int64(size + 2)
		}
		off += 2 + size
	}
	if h.Width == 0 {
		return h, fmt.Errorf("jpeg: no frame header")
	}
	// The frame header gives the stored size. Decode turns the pixels, so the
	// two would disagree on a sideways photograph unless the size is reported
	// the way it will be seen.
	h.Orientation = readOrientation(raw)
	if h.Orientation.Turns() {
		h.Width, h.Height = h.Height, h.Width
	}
	return h, nil
}
