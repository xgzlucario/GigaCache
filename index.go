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
// +-----------------------+------------------------+
// |       start(32)       |       offset(32)       |
// +-----------------------+------------------------+

type Idx uint64

const (
	maxStart   = math.MaxUint32
	offsetMask = math.MaxUint32
)

func (i Idx) start() int {
	return int(i >> 32)
}

func (i Idx) offset() int {
	return int(i & offsetMask)
}

func newIdx(start, offset int) Idx {
	if start > maxStart {
		panic("start overflows the limit of uint32")
	}
	if offset > offsetMask {
		panic("offset overflows the limit of uint32")
	}

	return Idx(start<<32 | offset)
}
