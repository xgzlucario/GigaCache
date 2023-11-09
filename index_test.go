package cache

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIndex(t *testing.T) {
	t.Run("index", func(t *testing.T) {
		assert := assert.New(t)

		for i := 0; i < 1e6; i++ {
			a, b := int(rand.Uint32()>>1), int(rand.Uint32()>>1)
			idx := newIdx(a, b, i%2 == 0)

			assert.Equal(idx.start(), a)
			assert.Equal(idx.offset(), b)

			if i%2 == 0 {
				assert.True(idx.IsAny())
			} else {
				assert.False(idx.IsAny())
			}
		}
	})

	t.Run("key", func(t *testing.T) {
		assert := assert.New(t)

		for i := 0; i < 1e6; i++ {
			hash := rand.Uint64()
			keylen := int(rand.Uint32() & klenMask)

			key := newKey(hash, keylen)

			assert.Equal(key.hash(), hash>>klenbits)
			assert.Equal(key.klen(), keylen)
		}
	})

	t.Run("panic-start", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("should panic")
			}
		}()
		newIdx(math.MaxInt, 0, false)
	})

	t.Run("panic-offset", func(t *testing.T) {
		defer func() {

			if r := recover(); r == nil {
				t.Fatal("should panic")
			}
		}()
		newIdx(0, math.MaxInt, false)
	})

	t.Run("panic-keylen", func(t *testing.T) {
		newKey(0, klenMask)

		defer func() {
			if r := recover(); r == nil {
				t.Fatal("should panic")
			}
		}()
		newKey(0, klenMask+1+1)
	})
}
