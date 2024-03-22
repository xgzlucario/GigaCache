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
		assert.Equal(idx.start(), start)
		assert.Equal(idx.TTL()/timeCarry, ttl/timeCarry)
	}

	// panic-start
	assert.Panics(func() {
		newIdx(math.MaxUint32+1, 0)
	})

	// panic-ttl
	assert.Panics(func() {
		newIdx(100, -1)
	})
	assert.Panics(func() {
		newIdx(100, (math.MaxUint32+1)*timeCarry)
	})
}
