package cache

import (
	"math"
)

// Key is the key of GigaCache.
// +--------------------------------+----------------+
// |            hash(52)            |    klen(12)    |
// +--------------------------------+----------------+

type Key uint64

const (
	klenMask = 0xfff
	klenbits = 12
)

func newKey(hash uint64, keylen int) Key {
	if keylen > klenMask {
		panic("key length overflow of 4KB")
	}
	return Key((hash >> klenbits << klenbits) | uint64(keylen))
}

func (k Key) hash() uint64 {
	return uint64(k >> klenbits)
}

func (k Key) klen() int {
	return int(k & klenMask)
}

// Idx is the index of GigaCache.
// +----------+-----------------+--------------------+
// | isAny(1) |    start(31)    |     offset(32)     |
// +----------+-----------------+--------------------+

type Idx uint64

const (
	maxStart   = math.MaxUint32 >> 1
	offsetMask = math.MaxUint32

	anyMask = 1 << 63
)

func (i Idx) start() int {
	return int(i << 1 >> 1 >> 32)
}

func (i Idx) offset() int {
	return int(i & offsetMask)
}

func (i Idx) IsAny() bool {
	return i&anyMask == anyMask
}

func newIdx(start, offset int, isAny bool) Idx {
	if start > maxStart {
		panic("start overflow")
	}
	if offset > offsetMask {
		panic("offset overflow")
	}

	idx := Idx(start<<32 | offset)
	if isAny {
		idx |= anyMask
	}

	return idx
}
