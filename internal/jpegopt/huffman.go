package jpegopt

import "sort"

// huffTable is a decoding table in the form the standard describes: for each
// code length, the smallest and largest code of that length and where its
// values start.
type huffTable struct {
	counts  [17]int // codes of each length, 1..16
	values  []byte
	minCode [17]int32
	maxCode [17]int32
	valPtr  [17]int
}

func newHuffTable(counts [17]int, values []byte) *huffTable {
	t := &huffTable{counts: counts, values: values}
	code, k := int32(0), 0
	for l := 1; l <= 16; l++ {
		if t.counts[l] == 0 {
			t.maxCode[l] = -1
			code <<= 1
			continue
		}
		t.valPtr[l] = k
		t.minCode[l] = code
		code += int32(t.counts[l])
		k += t.counts[l]
		t.maxCode[l] = code - 1
		code <<= 1
	}
	return t
}

func (j *jpg) readHuffman(p []byte) error {
	for len(p) > 0 {
		if len(p) < 17 {
			return ErrUnsupported
		}
		class, id := p[0]>>4, p[0]&15
		if class > 1 || id > 3 {
			return ErrUnsupported
		}

		var counts [17]int
		total := 0
		for i := 1; i <= 16; i++ {
			counts[i] = int(p[i])
			total += counts[i]
		}
		if len(p) < 17+total {
			return ErrUnsupported
		}
		values := make([]byte, total)
		copy(values, p[17:17+total])

		if class == 0 {
			j.dcTbl[id] = newHuffTable(counts, values)
		} else {
			j.acTbl[id] = newHuffTable(counts, values)
		}
		p = p[17+total:]
	}
	return nil
}

// --- fitting new tables ----------------------------------------------------

// buildTable fits a Huffman table to the symbols that actually occurred.
//
// This is the standard's own procedure, and the subtle part is the length
// limit: an optimal code can run past sixteen bits, which JPEG cannot express,
// so over-long codes are folded back by borrowing a bit from a shorter one. The
// spare symbol reserved before the fit is what guarantees no code is all ones —
// a code of all ones would be indistinguishable from the padding at the end of
// a stream.
func buildTable(freq []int) ([17]int, []byte) {
	const reserved = 256

	f := make([]int64, 257)
	for i, n := range freq {
		f[i] = int64(n)
	}
	f[reserved] = 1

	others := make([]int, 257)
	sizes := make([]int, 257)
	for i := range others {
		others[i] = -1
	}

	for {
		v1 := smallest(f, -1)
		if v1 < 0 {
			break
		}
		v2 := smallest(f, v1)
		if v2 < 0 {
			break
		}
		f[v1] += f[v2]
		f[v2] = 0
		sizes[v1]++
		for others[v1] >= 0 {
			v1 = others[v1]
			sizes[v1]++
		}
		others[v1] = v2
		sizes[v2]++
		for others[v2] >= 0 {
			v2 = others[v2]
			sizes[v2]++
		}
	}

	var bits [33]int
	for i := 0; i <= 256; i++ {
		if sizes[i] > 0 {
			bits[sizes[i]]++
		}
	}

	// Fold anything longer than sixteen bits back into range.
	for i := 32; i > 16; i-- {
		for bits[i] > 0 {
			j := i - 2
			for bits[j] == 0 {
				j--
			}
			bits[i] -= 2
			bits[i-1]++
			bits[j+1] += 2
			bits[j]--
		}
	}
	// Drop the reserved symbol: it was only there to keep the all-ones code out
	// of use, and it occupies the longest code by construction.
	for i := 16; i > 0; i-- {
		if bits[i] > 0 {
			bits[i]--
			break
		}
	}

	var counts [17]int
	for i := 1; i <= 16; i++ {
		counts[i] = bits[i]
	}

	// Values, ordered by code length and then by symbol.
	type sym struct{ size, value int }
	var syms []sym
	for i := 0; i < 256; i++ {
		if sizes[i] > 0 {
			syms = append(syms, sym{sizes[i], i})
		}
	}
	sort.Slice(syms, func(a, b int) bool {
		if syms[a].size != syms[b].size {
			return syms[a].size < syms[b].size
		}
		return syms[a].value < syms[b].value
	})
	values := make([]byte, 0, len(syms))
	for _, s := range syms {
		values = append(values, byte(s.value))
	}
	return counts, values
}

// smallest finds the least frequent symbol still in play, preferring the
// highest index on a tie. The tie-break is not arbitrary: it is what the
// reference encoder does, and matching it keeps our tables comparable with
// everyone else's.
func smallest(f []int64, exclude int) int {
	best, bestVal := -1, int64(0)
	for i := 0; i < len(f); i++ {
		if f[i] == 0 || i == exclude {
			continue
		}
		if best < 0 || f[i] <= bestVal {
			best, bestVal = i, f[i]
		}
	}
	return best
}

// encTable is the same table turned inside out for writing.
type encTable struct {
	code [256]uint32
	size [256]uint8
	dht  []byte // the DHT payload for this table
}

func newEncTable(class, id byte, counts [17]int, values []byte) *encTable {
	t := &encTable{}
	code, k := uint32(0), 0
	for l := 1; l <= 16; l++ {
		for i := 0; i < counts[l]; i++ {
			t.code[values[k]] = code
			t.size[values[k]] = uint8(l)
			code++
			k++
		}
		code <<= 1
	}

	t.dht = make([]byte, 0, 17+len(values))
	t.dht = append(t.dht, class<<4|id)
	for l := 1; l <= 16; l++ {
		t.dht = append(t.dht, byte(counts[l]))
	}
	t.dht = append(t.dht, values...)
	return t
}
