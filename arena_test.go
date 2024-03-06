package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestArenaAlloc(t *testing.T) {
	assert := assert.New(t)

	arena := NewArena()
	n, ok := arena.Alloc(1)
	assert.False(ok)
	assert.Equal(node{}, n)

	// free 16
	arena.Free(0x86, 16)

	// alloc > 16
	for i := 32; i > 16; i-- {
		n, ok := arena.Alloc(i)
		assert.False(ok)
		assert.Equal(node{}, n)
	}

	// alloc 16
	n, ok = arena.Alloc(16)
	assert.True(ok)
	assert.Equal(node{0x86, 16}, n)

	// free 15
	arena.Free(10, 15)

	// alloc 11
	n, ok = arena.Alloc(11)
	assert.True(ok)
	assert.Equal(node{10, 11}, n)
}
