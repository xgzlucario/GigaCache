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

type Idx uint64

const (
	ttlMask   = 0x00000000ffffffff
	timeCarry = 1e9
)

func (i Idx) start() int {
	return int(i >> 32)
}

func (i Idx) expired() bool {
	return i.sec() > noTTL && i.sec() < GetSec()
}

func (i Idx) sec() uint32 {
	return uint32(i & ttlMask)
}

func (i Idx) TTL() int64 {
	return int64(i.sec()) * timeCarry
}

func convTTL(ttl int64) uint64 {
	if ttl < 0 {
		panic("ttl is negetive")
	}
	check(ttl / timeCarry)
	return uint64(ttl) / timeCarry
}

func check[T int | int64 | float64](x T) {
	if x > math.MaxUint32 {
		panic("x overflows the limit of uint32")
	}
}

func newIdx(start int, ttl int64) Idx {
	check(start)
	return Idx(uint64(start)<<32 | convTTL(ttl))
}

// newIdxx is more efficient than newIdx.
func newIdxx(start int, idx Idx) Idx {
	check(start)
	return Idx(uint64(start)<<32 | uint64(idx.sec()))
}
