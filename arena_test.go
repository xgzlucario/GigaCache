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

	// free
	arena.Free(0x86, 16)

	for i := 32; i > 16; i-- {
		n, ok := arena.Alloc(i)
		assert.False(ok)
		assert.Equal(node{}, n)
	}

	n, ok = arena.Alloc(16)
	assert.True(ok)
	assert.Equal(node{0x86, 16}, n)
}
