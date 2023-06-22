package cache

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/stretchr/testify/assert"
)

const (
	num = 2000 * 100000
)

var (
	str = []byte("0123456789")
)

func TestCache(t *testing.T) {
	cache := NewGigaCache[string]()

	valid := map[string][]byte{}
	ttl := map[string]int64{}

	for i := 0; i < num/10; i++ {
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

func BenchmarkSet(b *testing.B) {
	m1 := map[string][]byte{}
	b.Run("stdmap", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m1[strconv.Itoa(i)] = str
		}
	})

	m2 := NewGigaCache[string]()
	b.Run("gigacache", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m2.Set(strconv.Itoa(i), str)
		}
	})

	m3, _ := bigcache.New(context.Background(), bigcache.DefaultConfig(time.Minute))
	b.Run("bigcache", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m3.Set(strconv.Itoa(i), str)
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
		m2.Set(strconv.Itoa(i), str)
	}
	b.ResetTimer()
	b.Run("gigacache", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m2.Get(strconv.Itoa(i))
		}
	})

	m3, _ := bigcache.New(context.Background(), bigcache.Config{
		Shards:             1024,
		Verbose:            false,
		MaxEntriesInWindow: num,
	})
	for i := 0; i < num; i++ {
		m3.Set(strconv.Itoa(i), str)
	}
	b.ResetTimer()
	b.Run("bigcache", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m3.Get(strconv.Itoa(i))
		}
	})
}
