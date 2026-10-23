package imgio

import "testing"

func FuzzReadHeader(f *testing.F) {
	f.Add([]byte("not an image"))
	f.Add([]byte{0xff, 0xd8, 0xff, 0xd9})
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 65536 {
			return
		}
		_, _ = ReadHeader(b)
	})
}
