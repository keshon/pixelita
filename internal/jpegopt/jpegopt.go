package jpegopt

import "encoding/binary"

// Optimise rewrites a JPEG with Huffman tables fitted to its own contents.
//
// Nothing else changes: the quantised coefficients are copied across, and every
// other segment — quantisation tables, colour transform, EXIF, ICC profile — is
// re-emitted exactly as it arrived. The decoded image is identical to the last
// bit, which is why there is no quality knob here and no way to get one wrong.
//
// Metadata is deliberately kept. Stripping an EXIF block can turn a photograph
// on its side and dropping an ICC profile changes the colours a browser paints,
// so neither belongs in an operation that promises to change nothing.
func Optimise(raw []byte) ([]byte, error) {
	j, err := parse(raw)
	if err != nil {
		return nil, err
	}
	if err := j.decode(); err != nil {
		return nil, err
	}

	dcFreq, acFreq := j.count()
	var dc, ac [4]*encTable
	for i := range j.comps {
		c := &j.comps[i]
		if dc[c.dc] == nil {
			counts, values := buildTable(dcFreq[c.dc])
			dc[c.dc] = newEncTable(0, c.dc, counts, values)
		}
		if ac[c.ac] == nil {
			counts, values := buildTable(acFreq[c.ac])
			ac[c.ac] = newEncTable(1, c.ac, counts, values)
		}
	}

	entropy := j.encodeScan(dc, ac)

	out := make([]byte, 0, len(raw))
	out = append(out, 0xFF, markerSOI)
	for _, s := range j.head {
		out = appendSegment(out, s.marker, s.data)
	}
	for _, t := range dc {
		if t != nil {
			out = appendSegment(out, markerDHT, t.dht)
		}
	}
	for _, t := range ac {
		if t != nil {
			out = appendSegment(out, markerDHT, t.dht)
		}
	}
	out = appendSegment(out, markerSOS, j.scanHeader)
	out = append(out, entropy...)
	out = append(out, 0xFF, markerEOI)
	return out, nil
}

func appendSegment(out []byte, marker byte, payload []byte) []byte {
	out = append(out, 0xFF, marker)
	out = binary.BigEndian.AppendUint16(out, uint16(len(payload)+2))
	return append(out, payload...)
}
