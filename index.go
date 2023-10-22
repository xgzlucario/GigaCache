package cache

import (
	"math"
)

// Idx is the index of GigaCahce.
// isAny(1)|start(31)|offset(32)

type Idx uint64

const (
	startMask  = math.MaxUint32 >> 1
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
	if start > startMask {
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
