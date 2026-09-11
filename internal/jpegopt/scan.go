package jpegopt

import "fmt"

// --- reading ---------------------------------------------------------------

type bitReader struct {
	data   []byte
	pos    int
	buf    uint32
	n      uint
	atMark bool // a marker was reached; the rest of this segment reads as zero
}

// next returns the next byte of coded data. A 0xFF in the data is written as
// 0xFF 0x00 so that it cannot be mistaken for a marker; anything else after a
// 0xFF ends the segment.
func (r *bitReader) next() byte {
	if r.pos >= len(r.data) {
		r.atMark = true
		return 0
	}
	b := r.data[r.pos]
	if b != 0xFF {
		r.pos++
		return b
	}
	if r.pos+1 < len(r.data) && r.data[r.pos+1] == 0x00 {
		r.pos += 2
		return 0xFF
	}
	r.atMark = true
	return 0
}

func (r *bitReader) bits(count uint) uint32 {
	for r.n < count {
		r.buf = r.buf<<8 | uint32(r.next())
		r.n += 8
	}
	r.n -= count
	v := (r.buf >> r.n) & (1<<count - 1)
	return v
}

// restart drops the partial byte and steps over the restart marker.
func (r *bitReader) restart() error {
	r.buf, r.n, r.atMark = 0, 0, false
	for r.pos+1 < len(r.data) {
		if r.data[r.pos] == 0xFF && r.data[r.pos+1] >= markerRST0 && r.data[r.pos+1] <= markerRST7 {
			r.pos += 2
			return nil
		}
		r.pos++
	}
	return fmt.Errorf("%w: a restart marker is missing", ErrUnsupported)
}

func (r *bitReader) decode(t *huffTable) (byte, error) {
	code := int32(r.bits(1))
	for l := 1; l <= 16; l++ {
		if t.maxCode[l] >= 0 && code <= t.maxCode[l] {
			return t.values[t.valPtr[l]+int(code-t.minCode[l])], nil
		}
		code = code<<1 | int32(r.bits(1))
	}
	return 0, fmt.Errorf("%w: a code is not in the table", ErrUnsupported)
}

// extend turns the magnitude-and-sign form the standard uses into a number.
func extend(v uint32, size byte) int32 {
	if size == 0 {
		return 0
	}
	if v < 1<<(size-1) {
		return int32(v) - 1<<size + 1
	}
	return int32(v)
}

// decode reads every block of the scan into the components.
func (j *jpg) decode() error {
	r := &bitReader{data: j.entropy}
	pred := make([]int32, len(j.comps))
	mcu := 0

	for y := 0; y < j.mcusY; y++ {
		for x := 0; x < j.mcusX; x++ {
			if j.restart > 0 && mcu > 0 && mcu%j.restart == 0 {
				if err := r.restart(); err != nil {
					return err
				}
				for i := range pred {
					pred[i] = 0
				}
			}
			for ci := range j.comps {
				c := &j.comps[ci]
				for v := 0; v < c.v; v++ {
					for h := 0; h < c.h; h++ {
						blk := c.block(x*c.h+h, y*c.v+v)
						if err := j.decodeBlock(r, c, blk, &pred[ci]); err != nil {
							return err
						}
					}
				}
			}
			mcu++
		}
	}
	return nil
}

func (j *jpg) decodeBlock(r *bitReader, c *component, blk []int32, pred *int32) error {
	dc, ac := j.dcTbl[c.dc], j.acTbl[c.ac]
	if dc == nil || ac == nil {
		return fmt.Errorf("%w: the scan names a table that was never defined", ErrUnsupported)
	}

	size, err := r.decode(dc)
	if err != nil {
		return err
	}
	if size > 16 {
		return fmt.Errorf("%w: dc magnitude %d", ErrUnsupported, size)
	}
	*pred += extend(r.bits(uint(size)), size)
	blk[0] = *pred

	for k := 1; k < 64; {
		rs, err := r.decode(ac)
		if err != nil {
			return err
		}
		run, size := int(rs>>4), rs&15
		if size == 0 {
			if run != 15 {
				break // end of block
			}
			k += 16
			continue
		}
		k += run
		if k > 63 {
			return fmt.Errorf("%w: a run leaves the block", ErrUnsupported)
		}
		blk[k] = extend(r.bits(uint(size)), size)
		k++
	}
	return nil
}

// --- writing ---------------------------------------------------------------

type bitWriter struct {
	out []byte
	buf uint32
	n   uint
}

func (w *bitWriter) write(code uint32, size uint8) {
	w.buf = w.buf<<size | code
	w.n += uint(size)
	for w.n >= 8 {
		w.n -= 8
		b := byte(w.buf >> w.n)
		w.out = append(w.out, b)
		if b == 0xFF {
			w.out = append(w.out, 0x00) // stuffing, so it is not read as a marker
		}
	}
}

// flush pads the last byte with ones. Ones are used because no code is allowed
// to be all ones, so a decoder cannot mistake the padding for a symbol.
func (w *bitWriter) flush() {
	if w.n > 0 {
		w.write(1<<(8-w.n)-1, uint8(8-w.n))
	}
	w.buf, w.n = 0, 0
}

// magnitude is the size-and-value pair the standard codes a number as.
func magnitude(v int32) (size uint8, bits uint32) {
	a := v
	if a < 0 {
		a = -a
		v--
	}
	for a > 0 {
		size++
		a >>= 1
	}
	return size, uint32(v) & (1<<size - 1)
}

// walk visits every block of the scan in coded order, so that counting symbols
// and writing them cannot drift apart.
func (j *jpg) walk(visit func(c *component, blk []int32, pred *int32), boundary func()) {
	pred := make([]int32, len(j.comps))
	mcu := 0
	for y := 0; y < j.mcusY; y++ {
		for x := 0; x < j.mcusX; x++ {
			if j.restart > 0 && mcu > 0 && mcu%j.restart == 0 {
				boundary()
				for i := range pred {
					pred[i] = 0
				}
			}
			for ci := range j.comps {
				c := &j.comps[ci]
				for v := 0; v < c.v; v++ {
					for h := 0; h < c.h; h++ {
						visit(c, c.block(x*c.h+h, y*c.v+v), &pred[ci])
					}
				}
			}
			mcu++
		}
	}
}

// count gathers how often every symbol occurs, which is all a fitted table is.
func (j *jpg) count() (dcFreq, acFreq [4][]int) {
	for i := 0; i < 4; i++ {
		dcFreq[i] = make([]int, 256)
		acFreq[i] = make([]int, 256)
	}
	j.walk(func(c *component, blk []int32, pred *int32) {
		size, _ := magnitude(blk[0] - *pred)
		*pred = blk[0]
		dcFreq[c.dc][size]++

		run := 0
		for k := 1; k < 64; k++ {
			if blk[k] == 0 {
				run++
				continue
			}
			for run > 15 {
				acFreq[c.ac][0xF0]++
				run -= 16
			}
			size, _ := magnitude(blk[k])
			acFreq[c.ac][byte(run<<4)|size]++
			run = 0
		}
		if run > 0 {
			acFreq[c.ac][0x00]++
		}
	}, func() {})
	return dcFreq, acFreq
}

func (j *jpg) encodeScan(dc, ac [4]*encTable) []byte {
	w := &bitWriter{out: make([]byte, 0, len(j.entropy))}
	rst := byte(0)

	j.walk(func(c *component, blk []int32, pred *int32) {
		t := dc[c.dc]
		size, bits := magnitude(blk[0] - *pred)
		*pred = blk[0]
		w.write(t.code[size], t.size[size])
		w.write(bits, size)

		a := ac[c.ac]
		run := 0
		for k := 1; k < 64; k++ {
			if blk[k] == 0 {
				run++
				continue
			}
			for run > 15 {
				w.write(a.code[0xF0], a.size[0xF0])
				run -= 16
			}
			size, bits := magnitude(blk[k])
			sym := byte(run<<4) | size
			w.write(a.code[sym], a.size[sym])
			w.write(bits, size)
			run = 0
		}
		if run > 0 {
			w.write(a.code[0x00], a.size[0x00])
		}
	}, func() {
		w.flush()
		w.out = append(w.out, 0xFF, markerRST0+rst)
		rst = (rst + 1) & 7
	})

	w.flush()
	return w.out
}
