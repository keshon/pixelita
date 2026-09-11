// Package jpegopt rewrites a JPEG without touching a single pixel.
//
// A JPEG stores blocks of quantised coefficients, entropy-coded with Huffman
// tables. The coefficients are the picture; the tables are only how they were
// written down. Most encoders ship a fixed pair of tables from the standard's
// example annex rather than tables fitted to the image in front of them, and
// swapping in fitted ones costs nothing at all: the coefficients are copied
// across untouched, so the decoded image is identical to the last bit.
//
// This is the rare optimisation with no trade-off to weigh. There is no quality
// setting here because nothing is lost.
package jpegopt

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	markerSOI  = 0xD8
	markerEOI  = 0xD9
	markerSOS  = 0xDA
	markerDQT  = 0xDB
	markerDNL  = 0xDC
	markerDRI  = 0xDD
	markerDHT  = 0xC4
	markerSOF0 = 0xC0
	markerSOF1 = 0xC1
	markerSOF2 = 0xC2
	markerCOM  = 0xFE
	markerRST0 = 0xD0
	markerRST7 = 0xD7
)

var (
	// ErrProgressive marks an input this package leaves alone. A progressive
	// file has already been through an encoder that cared, and rewriting one
	// needs a different scan machinery for a gain that is usually nil.
	ErrProgressive = errors.New("progressive jpeg")
	ErrUnsupported = errors.New("unsupported jpeg")
)

type segment struct {
	marker byte
	data   []byte // payload without the two length bytes
}

type component struct {
	id     byte
	h, v   int  // sampling factors
	tq     byte // quantisation table selector
	dc, ac byte // huffman table selectors, from the scan header
	bw, bh int  // blocks across and down, over the whole image
	coeff  []int32
}

func (c *component) block(bx, by int) []int32 {
	i := (by*c.bw + bx) * 64
	return c.coeff[i : i+64 : i+64]
}

type jpg struct {
	raw     []byte
	head    []segment // everything before the scan, in order, minus DHT
	comps   []component
	dcTbl   [4]*huffTable
	acTbl   [4]*huffTable
	restart int

	width, height int
	hmax, vmax    int
	mcusX, mcusY  int

	scanHeader []byte // the SOS payload, re-emitted as it came
	entropy    []byte // the coded data, from after SOS to EOI
}

// parse walks the marker structure and stops at the scan, keeping everything a
// rewrite has to reproduce.
func parse(raw []byte) (*jpg, error) {
	if len(raw) < 4 || raw[0] != 0xFF || raw[1] != markerSOI {
		return nil, ErrUnsupported
	}
	j := &jpg{raw: raw}

	for i := 2; i+4 <= len(raw); {
		if raw[i] != 0xFF {
			return nil, fmt.Errorf("%w: expected a marker at %d", ErrUnsupported, i)
		}
		marker := raw[i+1]
		if marker == 0xFF { // fill byte
			i++
			continue
		}
		size := int(binary.BigEndian.Uint16(raw[i+2 : i+4]))
		if size < 2 || i+2+size > len(raw) {
			return nil, fmt.Errorf("%w: bad segment length at %d", ErrUnsupported, i)
		}
		payload := raw[i+4 : i+2+size]

		switch marker {
		case markerSOF2:
			return nil, ErrProgressive
		case markerSOF0, markerSOF1:
			if err := j.readFrame(payload); err != nil {
				return nil, err
			}
			j.head = append(j.head, segment{marker, payload})
		case markerDHT:
			if err := j.readHuffman(payload); err != nil {
				return nil, err
			}
		case markerDRI:
			if len(payload) < 2 {
				return nil, ErrUnsupported
			}
			j.restart = int(binary.BigEndian.Uint16(payload))
			j.head = append(j.head, segment{marker, payload})
		case markerSOS:
			if err := j.readScanHeader(payload); err != nil {
				return nil, err
			}
			j.scanHeader = payload
			end := findEntropyEnd(raw, i+2+size)
			j.entropy = raw[i+2+size : end]
			return j, nil
		case markerDNL:
			return nil, ErrUnsupported
		default:
			j.head = append(j.head, segment{marker, payload})
		}
		i += 2 + size
	}
	return nil, fmt.Errorf("%w: no scan", ErrUnsupported)
}

// findEntropyEnd walks to the marker that ends the coded data. Restart markers
// belong to the stream; anything else terminates it.
func findEntropyEnd(raw []byte, from int) int {
	for i := from; i+1 < len(raw); i++ {
		if raw[i] != 0xFF {
			continue
		}
		next := raw[i+1]
		if next == 0x00 || next == 0xFF || (next >= markerRST0 && next <= markerRST7) {
			continue
		}
		return i
	}
	return len(raw)
}

func (j *jpg) readFrame(p []byte) error {
	if len(p) < 6 {
		return ErrUnsupported
	}
	if p[0] != 8 {
		return fmt.Errorf("%w: %d bits per sample", ErrUnsupported, p[0])
	}
	j.height = int(binary.BigEndian.Uint16(p[1:3]))
	j.width = int(binary.BigEndian.Uint16(p[3:5]))
	n := int(p[5])
	if n == 0 || len(p) < 6+n*3 {
		return ErrUnsupported
	}

	j.comps = make([]component, n)
	for i := 0; i < n; i++ {
		q := p[6+i*3:]
		c := &j.comps[i]
		c.id, c.h, c.v, c.tq = q[0], int(q[1]>>4), int(q[1]&15), q[2]
		if c.h < 1 || c.h > 4 || c.v < 1 || c.v > 4 {
			return ErrUnsupported
		}
		j.hmax = max(j.hmax, c.h)
		j.vmax = max(j.vmax, c.v)
	}

	j.mcusX = ceilDiv(j.width, 8*j.hmax)
	j.mcusY = ceilDiv(j.height, 8*j.vmax)
	for i := range j.comps {
		c := &j.comps[i]
		c.bw, c.bh = j.mcusX*c.h, j.mcusY*c.v
		c.coeff = make([]int32, c.bw*c.bh*64)
	}
	return nil
}

func (j *jpg) readScanHeader(p []byte) error {
	if len(p) < 1 {
		return ErrUnsupported
	}
	n := int(p[0])
	if n != len(j.comps) || len(p) < 1+n*2+3 {
		return fmt.Errorf("%w: scan over %d of %d components", ErrUnsupported, n, len(j.comps))
	}
	for i := 0; i < n; i++ {
		id, tables := p[1+i*2], p[2+i*2]
		c := j.byID(id)
		if c == nil {
			return ErrUnsupported
		}
		c.dc, c.ac = tables>>4, tables&15
		if c.dc > 3 || c.ac > 3 {
			return ErrUnsupported
		}
	}
	// Spectral selection and successive approximation must be the whole block
	// at full precision; anything else is a progressive scan in disguise.
	tail := p[1+n*2:]
	if tail[0] != 0 || tail[1] != 63 || tail[2] != 0 {
		return ErrProgressive
	}
	return nil
}

func (j *jpg) byID(id byte) *component {
	for i := range j.comps {
		if j.comps[i].id == id {
			return &j.comps[i]
		}
	}
	return nil
}

func ceilDiv(a, b int) int { return (a + b - 1) / b }
