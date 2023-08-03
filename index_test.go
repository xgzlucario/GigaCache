package cache

import (
	"math/rand"
	"testing"
)

func TestIndex(t *testing.T) {
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
}
