package cache

import (
	"math"
	"time"

	"github.com/zeebo/xxh3"
)

type Key = xxh3.Uint128

type Idx struct {
	hi uint32 // hi is position of data.
	lo int64  // lo is timestamp of key.
}

func (i Idx) start() int {
	return int(i.hi)
}

func (i Idx) expired() bool {
	return i.lo > noTTL && i.lo < time.Now().UnixNano()
}

func (i Idx) expiredWith(nanosec int64) bool {
	return i.lo > noTTL && i.lo < nanosec
}

func (i Idx) setTTL(ts int64) Idx {
	i.lo = ts
	return i
}

func check(x int) {
	if x > math.MaxUint32 {
		panic("x overflows the limit of uint32")
	}
}

func newIdx(start int, ttl int64) Idx {
	check(start)
	return Idx{hi: uint32(start), lo: ttl}
}

// newIdxx is more efficient than newIdx.
func newIdxx(start int, idx Idx) Idx {
	check(start)
	return Idx{hi: uint32(start), lo: idx.lo}
}
