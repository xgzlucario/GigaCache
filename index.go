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
	ttlMask   = math.MaxUint32
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
	return int64(uint64(i&ttlMask) * timeCarry)
}

func convTTL(ttl int64) uint64 {
	if ttl < 0 {
		panic("ttl is negetive")
	}
	if ttl/timeCarry > math.MaxUint32 {
		panic("ttl overflows the limit of uint32")
	}
	return uint64(ttl) / timeCarry
}

func newIdx(start int, ttl int64) Idx {
	if start > math.MaxUint32 {
		panic("start overflows the limit of uint32")
	}
	return Idx(uint64(start)<<32 | convTTL(ttl))
}
