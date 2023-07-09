package cache

import (
	"strconv"
	"testing"
	"time"

	"golang.org/x/exp/rand"

	"github.com/jellydator/ttlcache/v3"
)

const (
	num = 1000 * 10000
)

var (
	str = []byte("0123456789")
)

func TestIdx(b *testing.T) {
	for i := 0; i < 1e8; i++ {
		a, b := int(rand.Uint32()), int(rand.Uint32()>>1)
		idx := newIdx(int(a), int(b), i%2 == 0)

		if idx.start() != a {
			panic("a")
		}
		if idx.offset() != b {
			panic("b")
		}

		if i%2 == 0 {
			if !idx.hasTTL() {
				panic("c")
			}
		} else {
			if idx.hasTTL() {
				panic("c")
			}
		}
	}
}

func BenchmarkSet(b *testing.B) {
	m1 := map[string][]byte{}
	b.Run("stdmap/Set", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m1[strconv.Itoa(i)] = str
		}
	})

	m2 := NewGigaCache[string]()
	b.Run("gigacache/Set", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m2.Set(strconv.Itoa(i), str)
		}
	})

	m3 := NewGigaCache[string]()
	b.Run("gigacache/SetTx", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m3.SetEx(strconv.Itoa(i), str, time.Minute)
		}
	})

	m4 := ttlcache.New[string, []byte]()
	b.Run("ttlcache/Set", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m4.Set(strconv.Itoa(i), str, time.Minute)
		}
	})
}

func BenchmarkGet(b *testing.B) {
	m1 := map[string][]byte{}
	for i := 0; i < num; i++ {
		m1[strconv.Itoa(i)] = str
	}
	b.ResetTimer()
	b.Run("stdmap", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = m1[strconv.Itoa(i)]
		}
	})

	m2 := NewGigaCache[string]()
	for i := 0; i < num; i++ {
		m2.SetEx(strconv.Itoa(i), str, time.Minute)
	}
	b.ResetTimer()
	b.Run("gigacache", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m2.Get(strconv.Itoa(i))
		}
	})

	m3 := ttlcache.New[string, []byte]()
	for i := 0; i < num; i++ {
		m3.Set(strconv.Itoa(i), str, time.Minute)
	}
	b.ResetTimer()
	b.Run("ttlcache", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m3.Get(strconv.Itoa(i))
		}
	})
}

func BenchmarkDelete(b *testing.B) {
	m1 := map[string][]byte{}
	for i := 0; i < num; i++ {
		m1[strconv.Itoa(i)] = str
	}
	b.ResetTimer()
	b.Run("stdmap", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			delete(m1, strconv.Itoa(i))
		}
	})

	m2 := NewGigaCache[string]()
	for i := 0; i < num; i++ {
		m2.Delete(strconv.Itoa(i))
	}
	b.ResetTimer()
	b.Run("gigacache", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m2.Get(strconv.Itoa(i))
		}
	})

	m3 := ttlcache.New[string, []byte]()
	for i := 0; i < num; i++ {
		m3.Set(strconv.Itoa(i), str, time.Minute)
	}
	b.ResetTimer()
	b.Run("ttlcache", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m3.Get(strconv.Itoa(i))
		}
	})
}
