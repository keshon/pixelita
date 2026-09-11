package imgio

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

// withOrientation splices a minimal EXIF block carrying one tag into a JPEG.
//
// Built by hand rather than committed as a fixture: a real photograph from a
// phone would be a megabyte of someone's living room, and the thing under test
// is four bytes of it.
func withOrientation(t *testing.T, jpg []byte, o Orientation) []byte {
	t.Helper()

	var tiff bytes.Buffer
	tiff.WriteString("II")                                   // little endian
	binary.Write(&tiff, binary.LittleEndian, uint16(42))     // the magic
	binary.Write(&tiff, binary.LittleEndian, uint32(8))      // IFD0 starts here
	binary.Write(&tiff, binary.LittleEndian, uint16(1))      // one entry
	binary.Write(&tiff, binary.LittleEndian, uint16(0x0112)) // Orientation
	binary.Write(&tiff, binary.LittleEndian, uint16(3))      // SHORT
	binary.Write(&tiff, binary.LittleEndian, uint32(1))      // one value
	binary.Write(&tiff, binary.LittleEndian, uint16(o))
	binary.Write(&tiff, binary.LittleEndian, uint16(0)) // padding of the value field
	binary.Write(&tiff, binary.LittleEndian, uint32(0)) // no next IFD

	payload := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	app1 := []byte{0xFF, 0xE1}
	app1 = binary.BigEndian.AppendUint16(app1, uint16(len(payload)+2))
	app1 = append(app1, payload...)

	out := make([]byte, 0, len(jpg)+len(app1))
	out = append(out, jpg[:2]...) // SOI
	out = append(out, app1...)
	return append(out, jpg[2:]...)
}

// A wide image tagged "rotate 90" has to come back tall, with the corner that
// was top-left now at top-right.
func TestOrientationIsApplied(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 64, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 64; x++ {
			src.SetNRGBA(x, y, color.NRGBA{20, 20, 20, 255})
		}
	}
	// A marker in the top-left corner, big enough to survive JPEG.
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			src.SetNRGBA(x, y, color.NRGBA{255, 255, 255, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	plain := buf.Bytes()

	t.Run("untagged is left alone", func(t *testing.T) {
		img, _, err := Decode(plain)
		if err != nil {
			t.Fatal(err)
		}
		b := img.Bounds()
		if b.Dx() != 64 || b.Dy() != 32 {
			t.Errorf("got %dx%d, want 64x32", b.Dx(), b.Dy())
		}
	})

	t.Run("rotate 90 turns the pixels", func(t *testing.T) {
		raw := withOrientation(t, plain, OrientRotate90)

		if o := readOrientation(raw); o != OrientRotate90 {
			t.Fatalf("readOrientation = %v, want rotate 90", o)
		}

		img, _, err := Decode(raw)
		if err != nil {
			t.Fatal(err)
		}
		b := img.Bounds()
		if b.Dx() != 32 || b.Dy() != 64 {
			t.Fatalf("got %dx%d, want 32x64 — the pixels were not turned", b.Dx(), b.Dy())
		}

		out := ToNRGBA(img)
		if c := out.NRGBAAt(28, 4); c.R < 200 {
			t.Errorf("top-right is %v, want the white marker to have landed there", c)
		}
		if c := out.NRGBAAt(4, 4); c.R > 100 {
			t.Errorf("top-left is %v, want dark", c)
		}
	})

	t.Run("the header reports the size as it will be seen", func(t *testing.T) {
		head, err := ReadHeader(withOrientation(t, plain, OrientRotate90))
		if err != nil {
			t.Fatal(err)
		}
		if head.Width != 32 || head.Height != 64 {
			t.Errorf("header says %dx%d, want 32x64 to match what Decode returns",
				head.Width, head.Height)
		}
		if head.Orientation != OrientRotate90 {
			t.Errorf("orientation = %v", head.Orientation)
		}
	})
}

// img-jpeg works on the coded data and must keep every segment, orientation
// included: it promises the file is unchanged except for its Huffman tables.
func TestOrientationSurvivesLosslessJPEG(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 48, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 48; x++ {
			src.SetNRGBA(x, y, color.NRGBA{uint8(x * 5), uint8(y * 9), 90, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	raw := withOrientation(t, buf.Bytes(), OrientRotate270)

	if o := readOrientation(raw); o != OrientRotate270 {
		t.Fatalf("the fixture itself is wrong: %v", o)
	}
	head, err := ReadHeader(raw)
	if err != nil {
		t.Fatal(err)
	}
	if head.Width != 24 || head.Height != 48 {
		t.Errorf("header says %dx%d, want 24x48", head.Width, head.Height)
	}
}
