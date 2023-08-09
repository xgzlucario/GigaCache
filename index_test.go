package cache

import (
	"math"
	"math/rand"
	"testing"
)

func TestIndex(t *testing.T) {
	t.Run("index", func(t *testing.T) {
		for i := 0; i < 1e8; i++ {
			a, b := int(rand.Uint32()>>1), int(rand.Uint32()>>1)
			idx := newIdx(a, b, i%2 == 0, false)

			if idx.start() != a {
				t.Fatalf("%v != %v", idx.start(), a)
			}
			if idx.offset() != b {
				t.Fatalf("%v != %v", idx.offset(), b)
			}

			if i%2 == 0 {
				if !idx.hasTTL() {
					t.Fatal("a")
				}
			} else {
				if idx.hasTTL() {
					t.Fatal("b")
				}
			}
		}
	})

	t.Run("panic-start", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("should panic")
			}
		}()
		newIdx(math.MaxInt, 0, false, false)
	})

	t.Run("panic-offset", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("should panic")
			}
		}()
		newIdx(0, math.MaxInt, false, false)
	})
}
