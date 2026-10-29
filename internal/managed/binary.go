package managed

import (
	"errors"
	"math"
	"unicode/utf16"
)

// The binary plist format (bplist00): an 8-byte header, the objects, an offset table, and a 32-byte trailer. Every
// count, offset and length comes from the file, so each one is checked against the bounds before it is used. The
// parser must never panic on bad input; FuzzParse checks that.

const (
	bplistMagic      = "bplist00"
	bplistTrailerLen = 32
)

// Object states, used to memoize decoded objects and to detect reference cycles.
const (
	stateUnseen uint8 = iota
	stateActive
	stateDone
)

type binaryReader struct {
	data    []byte
	limit   uint64 // start of the offset table. Object bodies must end at or before it.
	refSize uint64
	offsets []uint64
	state   []uint8
	values  []any
}

func parseBinary(data []byte) (any, error) {
	if len(data) < len(bplistMagic)+bplistTrailerLen || string(data[:len(bplistMagic)]) != bplistMagic {
		return nil, errors.New("plist: not a binary plist (bplist00)")
	}
	trailer := data[len(data)-bplistTrailerLen:]
	offSize := uint64(trailer[6])
	refSize := uint64(trailer[7])
	numObjects := readUint(trailer[8:16])
	top := readUint(trailer[16:24])
	tableStart := readUint(trailer[24:32])
	body := uint64(len(data) - bplistTrailerLen)

	if offSize < 1 || offSize > 8 || refSize < 1 || refSize > 8 {
		return nil, errors.New("plist: bad offset or reference size")
	}
	if numObjects == 0 || numObjects > body {
		return nil, errors.New("plist: bad object count")
	}
	if top >= numObjects {
		return nil, errors.New("plist: top object out of range")
	}
	if tableStart < uint64(len(bplistMagic)) || tableStart > body {
		return nil, errors.New("plist: offset table out of range")
	}
	if numObjects > (body-tableStart)/offSize {
		return nil, errors.New("plist: offset table out of range")
	}

	r := &binaryReader{
		data:    data,
		limit:   tableStart,
		refSize: refSize,
		offsets: make([]uint64, numObjects),
		state:   make([]uint8, numObjects),
		values:  make([]any, numObjects),
	}
	for i := range r.offsets {
		entry := tableStart + uint64(i)*offSize
		off := readUint(data[entry : entry+offSize])
		if off < uint64(len(bplistMagic)) || off >= tableStart {
			return nil, errors.New("plist: object offset out of range")
		}
		r.offsets[i] = off
	}
	return r.value(top, 0)
}

// value returns object idx, decoding it on first use. Each object is decoded once and shared after that, so a file
// that points many references at one object costs no more than one that does not.
func (r *binaryReader) value(idx uint64, depth int) (any, error) {
	if idx >= uint64(len(r.offsets)) {
		return nil, errors.New("plist: object reference out of range")
	}
	if depth > maxNestingDepth {
		return nil, errTooDeep
	}
	switch r.state[idx] {
	case stateDone:
		return r.values[idx], nil
	case stateActive:
		return nil, errors.New("plist: reference cycle")
	}
	r.state[idx] = stateActive
	v, err := r.decode(idx, depth)
	if err != nil {
		return nil, err
	}
	r.state[idx] = stateDone
	r.values[idx] = v
	return v, nil
}

func (r *binaryReader) decode(idx uint64, depth int) (any, error) {
	pos := r.offsets[idx]
	marker := r.data[pos] // in range: offsets are checked to lie before the offset table
	kind, info := marker>>4, uint64(marker&0x0F)
	pos++

	switch kind {
	case 0x0:
		switch info {
		case 0x0:
			return opaque{"null"}, nil
		case 0x8:
			return false, nil
		case 0x9:
			return true, nil
		}
	case 0x1: // integer, 2^info bytes. Sizes 1, 2 and 4 are unsigned, and 8 is two's complement.
		if info > 3 {
			break
		}
		b, err := r.read(pos, 1<<info)
		if err != nil {
			return nil, err
		}
		return int64(readUint(b)), nil
	case 0x2: // real: float32 for info 2, float64 for info 3
		if info != 2 && info != 3 {
			break
		}
		b, err := r.read(pos, 1<<info)
		if err != nil {
			return nil, err
		}
		if info == 2 {
			return float64(math.Float32frombits(uint32(readUint(b)))), nil
		}
		return math.Float64frombits(readUint(b)), nil
	case 0x3: // date: float64 seconds since 2001. Stored but not read.
		if info != 3 {
			break
		}
		if _, err := r.read(pos, 8); err != nil {
			return nil, err
		}
		return opaque{"date"}, nil
	case 0x4: // data: stored but not read
		n, pos, err := r.length(info, pos)
		if err != nil {
			return nil, err
		}
		if _, err := r.read(pos, n); err != nil {
			return nil, err
		}
		return opaque{"data"}, nil
	case 0x5: // ASCII string
		n, pos, err := r.length(info, pos)
		if err != nil {
			return nil, err
		}
		b, err := r.read(pos, n)
		if err != nil {
			return nil, err
		}
		return string(b), nil
	case 0x6: // UTF-16BE string: n code units
		n, pos, err := r.length(info, pos)
		if err != nil {
			return nil, err
		}
		if n > (r.limit-pos)/2 { // checked before 2*n, which could overflow
			return nil, errTruncated
		}
		b, err := r.read(pos, 2*n)
		if err != nil {
			return nil, err
		}
		units := make([]uint16, n)
		for i := range units {
			units[i] = uint16(readUint(b[2*i : 2*i+2]))
		}
		return string(utf16.Decode(units)), nil
	case 0x8: // UID: stored but not read
		if _, err := r.read(pos, info+1); err != nil {
			return nil, err
		}
		return opaque{"uid"}, nil
	case 0xA: // array of references
		n, pos, err := r.length(info, pos)
		if err != nil {
			return nil, err
		}
		if n > (r.limit-pos)/r.refSize {
			return nil, errTruncated
		}
		b, err := r.read(pos, n*r.refSize)
		if err != nil {
			return nil, err
		}
		arr := make([]any, 0, n)
		for i := uint64(0); i < n; i++ {
			v, err := r.value(r.ref(b, i), depth+1)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		return arr, nil
	case 0xD: // dictionary: n key references, then n value references
		n, pos, err := r.length(info, pos)
		if err != nil {
			return nil, err
		}
		if n > (r.limit-pos)/(2*r.refSize) {
			return nil, errTruncated
		}
		b, err := r.read(pos, 2*n*r.refSize)
		if err != nil {
			return nil, err
		}
		dict := make(map[string]any, n)
		for i := uint64(0); i < n; i++ {
			kv, err := r.value(r.ref(b, i), depth+1)
			if err != nil {
				return nil, err
			}
			key, isString := kv.(string)
			if !isString {
				return nil, errors.New("plist: dictionary key is not a string")
			}
			v, err := r.value(r.ref(b, n+i), depth+1)
			if err != nil {
				return nil, err
			}
			dict[key] = v
		}
		return dict, nil
	}
	return nil, errors.New("plist: unsupported object type")
}

// ref reads the i-th reference of size refSize from b.
func (r *binaryReader) ref(b []byte, i uint64) uint64 {
	return readUint(b[i*r.refSize : (i+1)*r.refSize])
}

// length returns the element count or byte length for an object whose marker info is info. A value of 0xF means the
// real length follows as an integer object.
func (r *binaryReader) length(info uint64, pos uint64) (n uint64, next uint64, err error) {
	if info != 0xF {
		return info, pos, nil
	}
	b, err := r.read(pos, 1)
	if err != nil {
		return 0, 0, err
	}
	if b[0]>>4 != 0x1 || b[0]&0x0F > 3 {
		return 0, 0, errors.New("plist: bad extended length")
	}
	size := uint64(1) << (b[0] & 0x0F)
	raw, err := r.read(pos+1, size)
	if err != nil {
		return 0, 0, err
	}
	return readUint(raw), pos + 1 + size, nil
}

// read returns the n bytes at pos, which must lie before the offset table.
func (r *binaryReader) read(pos, n uint64) ([]byte, error) {
	if pos > r.limit || n > r.limit-pos {
		return nil, errTruncated
	}
	return r.data[pos : pos+n], nil
}

var errTruncated = errors.New("plist: object runs past the end of the file")

// readUint reads up to 8 big-endian bytes.
func readUint(b []byte) uint64 {
	var v uint64
	for _, c := range b {
		v = v<<8 | uint64(c)
	}
	return v
}
