package imgio

import (
	"encoding/binary"
	"fmt"
)

// PreservePNGMetadata retains ancillary chunks whose meaning survives a palette
// change. Palette-indexed and animated metadata require a different operation.
func PreservePNGMetadata(source, encoded []byte) ([]byte, error) {
	if len(source) < 8 || len(encoded) < 33 {
		return nil, fmt.Errorf("invalid PNG stream")
	}
	var chunks []byte
	for off := 8; off+12 <= len(source); {
		n := int(binary.BigEndian.Uint32(source[off : off+4]))
		if n > len(source)-off-12 {
			return nil, fmt.Errorf("truncated PNG chunk")
		}
		name := string(source[off+4 : off+8])
		chunk := source[off : off+n+12]
		switch name {
		case "IHDR", "IDAT", "IEND", "PLTE", "tRNS":
		case "acTL", "fcTL", "fdAT", "bKGD", "hIST":
			return nil, fmt.Errorf("cannot preserve %s during palette conversion", name)
		case "iCCP", "sRGB", "gAMA", "cHRM", "pHYs", "tEXt", "zTXt", "iTXt", "eXIf", "tIME":
			chunks = append(chunks, chunk...)
		default:
			if name[3] < 'a' || name[3] > 'z' {
				return nil, fmt.Errorf("cannot safely copy unknown PNG chunk %s", name)
			}
			if name[0] >= 'a' && name[0] <= 'z' {
				chunks = append(chunks, chunk...)
			} else {
				return nil, fmt.Errorf("unsupported critical PNG chunk %s", name)
			}
		}
		off += n + 12
		if name == "IEND" {
			break
		}
	}
	out := make([]byte, 0, len(encoded)+len(chunks))
	out = append(out, encoded[:33]...)
	out = append(out, chunks...)
	out = append(out, encoded[33:]...)
	return out, nil
}

// ReencodeRestriction identifies rendering properties that a plain decoded
// pixel buffer cannot carry into the current output encoders.
func ReencodeRestriction(raw []byte) error {
	h, err := ReadHeader(raw)
	if err != nil {
		return err
	}
	for _, c := range h.Chunks {
		switch c {
		case "acTL", "fcTL", "fdAT":
			return fmt.Errorf("animated images require an animation-preserving encoder")
		case "iCCP", "ICC_PROFILE":
			return fmt.Errorf("ICC-profile conversion is not supported; preserve the source format and profile")
		}
	}
	return nil
}
