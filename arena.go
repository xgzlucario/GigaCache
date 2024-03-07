package cache

import (
	"slices"
)

const (
	MAX_LEVEL        = 16
	LEVEL_SIZE       = 8
	LEVEL_SCALE_BITS = 1
)

// arena is a memory allocator.
type arena struct {
	mt [MAX_LEVEL][LEVEL_SIZE]node
}

type node struct {
	start, offset uint32
}

func newArena() *arena {
	return &arena{}
}

// Alloc allocates a block of memory and return it start and offset.
func (a *arena) Alloc(want int) (node, bool) {
	level := toLevel(want)
	if level >= MAX_LEVEL {
		return node{}, false
	}

	for i, n := range a.mt[level] {
		if n.offset >= uint32(want) {
			// split the node
			a.Free(n.start+uint32(want), n.offset-uint32(want))
			a.mt[level][i] = node{}
			n.offset = uint32(want)
			return n, true
		}
	}
	return node{}, false
}

// Free
func (a *arena) Free(start, offset uint32) {
	if offset == 0 {
		return
	}
	level := toLevel(int(offset))
	if level >= MAX_LEVEL {
		return
	}

	n := a.mt[level][0]
	if offset > n.offset {
		a.mt[level][0] = node{start, offset}
		// TODO: optimize move empty node to it place directly.
		// sort
		slices.SortFunc(a.mt[level][:], func(a, b node) int {
			return int(a.offset) - int(b.offset)
		})
	}
}

// Clear
func (a *arena) Clear() {
	a.mt = [MAX_LEVEL][LEVEL_SIZE]node{}
}

func toLevel(size int) (level int) {
	size /= 4
	for ; size > 0; size >>= LEVEL_SCALE_BITS {
		level++
	}
	return
}
