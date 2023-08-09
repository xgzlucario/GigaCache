package cache

import (
	"math"
)

// Idx is the index of GigaCahce.
// hasTTL(1)|isAny(1)|start(31)|offset(31)

type Idx uint64

const (
	offsetMask = math.MaxUint32 >> 1

	ttlMask = 1 << 63
	anyMask = 1 << 62
)

func (i Idx) start() int {
	return int(i << 2 >> 2 >> 31)
}

func (i Idx) offset() int {
	return int(i & offsetMask)
}

func (i Idx) hasTTL() bool {
	return i&ttlMask == ttlMask
}

func (i Idx) IsAny() bool {
	return i&anyMask == anyMask
}

func newIdx(start, offset int, hasTTL bool, isAny bool) Idx {
	// bound check
	if start > offsetMask {
		panic("start overflow")
	}
	if offset > offsetMask {
		panic("offset overflow")
	}

	idx := Idx(start<<31 | offset)
	if hasTTL {
		idx |= ttlMask
	}
	if isAny {
		idx |= anyMask
	}

	return idx
}
