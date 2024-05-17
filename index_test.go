package cache

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIndex(t *testing.T) {
	assert := assert.New(t)

	// index
	for i := 0; i < 10000; i++ {
		start, ttl := int(FastRand()), time.Now().UnixNano()
		idx := newIdx(start, ttl)
		idxx := newIdxx(start, idx)
		assert.Equal(idx, idxx)
		assert.Equal(idx.start(), start)
		assert.Equal(int64(idx.l), ttl/timeCarry)
	}

	// panic-start
	assert.Panics(func() {
		newIdx(math.MaxUint32+1, 0)
	})
	assert.Panics(func() {
		newIdxx(math.MaxUint32+1, Idx{})
	})

	// panic-ttl
	assert.Panics(func() {
		newIdx(100, -1)
	})
	assert.Panics(func() {
		newIdx(100, (math.MaxUint32+1)*timeCarry)
	})
}
