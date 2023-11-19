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
// h1:
// +-----------------------+------------------------+
// |       start(32)       |       offset(32)       |
// +-----------------------+------------------------+
// h2:
// +-----------------------+------------------------+
// |                   ttl(int64)                   |
// +-----------------------+------------------------+

type Idx struct {
	h1 uint64
	h2 int64
}

const (
	maxStart   = math.MaxUint32
	offsetMask = math.MaxUint32
)

func (i Idx) start() int {
	return int(i.h1 >> 32)
}

func (i Idx) offset() int {
	return int(i.h1 & offsetMask)
}

func (i Idx) expired() bool {
	return i.h2 > noTTL && i.h2 < GetClock()
}

func (i Idx) TTL() int64 {
	return i.h2
}

func newIdx(start, offset int, ttl int64) Idx {
	if start > maxStart {
		panic("start overflows the limit of uint32")
	}
	if offset > offsetMask {
		panic("offset overflows the limit of uint32")
	}
	if ttl < 0 {
		panic("ttl is negetive")
	}
	return Idx{
		h1: uint64(start<<32 | offset),
		h2: ttl,
	}
}
