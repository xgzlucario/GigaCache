package cache

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIndex(t *testing.T) {
	t.Run("index", func(t *testing.T) {
		assert := assert.New(t)

		for i := 0; i < 1e6; i++ {
			start, ttl := int(rand.Uint32()), time.Now().UnixNano()
			idx := newIdx(start, ttl)
			assert.Equal(idx.start(), start)
			assert.Equal(idx.TTL()/timeCarry, ttl/timeCarry)
		}
	})

	t.Run("key", func(t *testing.T) {
		assert := assert.New(t)

		for i := 0; i < 1e6; i++ {
			hash := rand.Uint64()
			key := newKey(hash)

			assert.Equal(uint64(key), hash)
		}
	})

	t.Run("panic-start", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("should panic")
			}
		}()
		newIdx(math.MaxInt, 0)
	})

	t.Run("panic-ttl", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("should panic")
			}
		}()
		newIdx(100, -1)
	})
}
