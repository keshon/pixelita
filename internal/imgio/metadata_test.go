package imgio

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"testing"
)

func metadataChunk(name string, data []byte) []byte {
	b := make([]byte, len(data)+12)
	binary.BigEndian.PutUint32(b, uint32(len(data)))
	copy(b[4:8], name)
	copy(b[8:], data)
	binary.BigEndian.PutUint32(b[len(b)-4:], crc32.ChecksumIEEE(b[4:len(b)-4]))
	return b
}
func TestPNGMetadataSurvivesPaletteEncoding(t *testing.T) {
	raw, err := EncodePNG(image.NewNRGBA(image.Rect(0, 0, 8, 8)))
	if err != nil {
		t.Fatal(err)
	}
	chunk := metadataChunk("tEXt", []byte("Author\x00Pixelita"))
	idot := metadataChunk("iDOT", make([]byte, 28))
	source := append(append(append(append([]byte{}, raw[:33]...), chunk...), idot...), raw[33:]...)
	out, err := PreservePNGMetadata(source, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, chunk) {
		t.Fatal("metadata lost")
	}
	if bytes.Contains(out, idot) {
		t.Fatal("stale iDOT offsets copied into recompressed PNG")
	}
	if _, _, err := Decode(out); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"acTL", "bKGD", "hIST", "abCD"} {
		c := metadataChunk(name, []byte{0})
		src := append(append(append([]byte{}, raw[:33]...), c...), raw[33:]...)
		if _, err := PreservePNGMetadata(src, raw); err == nil {
			t.Fatalf("unsafe metadata accepted: %s", name)
		}
	}
}
