package cache

import (
	"math"
)

// Key is the key of GigaCache.
// +------------------------------------------------+
// |                    hash(64)                    |
// +------------------------------------------------+

type Key uint64

// Idx is the index of GigaCache.
// +-----------------------+-------------------------+
// |       start(32)       |       ttl(uint32)       |
// +-----------------------+-------------------------+

type Idx struct {
	h, l uint32
}

const (
	ttlMask   = 0x00000000ffffffff
	timeCarry = 1e9
)

func (i Idx) start() int {
	return int(i.h)
}

func (i Idx) expired() bool {
	return i.l > noTTL && i.l < GetSec()
}

func (i Idx) setTTL(ts int64) Idx {
	i.l = convTTL(ts)
	return i
}

func (i Idx) TTL() int64 {
	return int64(i.l) * timeCarry
}

func convTTL(ttl int64) uint32 {
	if ttl < 0 {
		panic("ttl is negetive")
	}
	check(ttl / timeCarry)
	return uint32(ttl / timeCarry)
}

func check[T int | int64](x T) {
	if x > math.MaxUint32 {
		panic("x overflows the limit of uint32")
	}
}

func newIdx(start int, ttl int64) Idx {
	check(start)
	return Idx{h: uint32(start), l: convTTL(ttl)}
}

// newIdxx is more efficient than newIdx.
func newIdxx(start int, idx Idx) Idx {
	check(start)
	return Idx{h: uint32(start), l: idx.l}
}
