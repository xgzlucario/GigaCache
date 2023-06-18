package cache

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCache(t *testing.T) {
	cache := NewGigaCache[string]()

	valid := map[string][]byte{}
	ttl := map[string]int64{}

	for i := 0; i < 100000; i++ {
		p := "xgz" + strconv.Itoa(i)

		// make it unexpired
		ts := time.Now().UnixNano() << 1

		valid[p] = []byte(p)
		ttl[p] = ts

		cache.SetTx(p, []byte(p), ts)
	}

	for k, v := range valid {
		value, ts, ok := cache.GetTx(k)
		assert.True(t, ok)
		assert.Equal(t, v, value)
		assert.Equal(t, ttl[k], ts)
	}
}

func BenchmarkCache(b *testing.B) {
	m := map[string][]byte{}
	b.Run("stdmap", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m[strconv.Itoa(i)] = []byte("value")
		}
	})

	m2 := NewMap[string, []byte]()
	b.Run("tidwall/hashmap", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m2.Set(strconv.Itoa(i), []byte("value"))
		}
	})

	c := NewGigaCache[string]()
	b.Run("gigacache", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c.Set(strconv.Itoa(i), []byte("value"))
		}
	})
}
