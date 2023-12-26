package cache

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIndex(t *testing.T) {
	assert := assert.New(t)

	// index
	for i := 0; i < 1e6; i++ {
		start, ttl := int(rand.Uint32()), time.Now().UnixNano()
		idx := newIdx(start, ttl)
		assert.Equal(idx.start(), start)
		assert.Equal(idx.TTL()/timeCarry, ttl/timeCarry)
	}

	// key
	for i := 0; i < 1e6; i++ {
		hash := rand.Uint64()
		key := newKey(hash)
		assert.Equal(uint64(key), hash)
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
