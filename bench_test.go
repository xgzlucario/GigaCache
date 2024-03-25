package cache

import (
	"strconv"
	"testing"
	"time"

	"github.com/aclements/go-perfevent/perfbench"
)

var (
	num = 100 * 10000
	str = []byte("Hello World")
)

func getStdmap() map[string][]byte {
	m := map[string][]byte{}
	for i := 0; i < num; i++ {
		m[strconv.Itoa(i)] = str
	}
	return m
}

func BenchmarkSet(b *testing.B) {
	b.Run("stdmap", func(b *testing.B) {
		perfbench.Open(b)
		m := map[string][]byte{}
		for i := 0; i < b.N; i++ {
			m[strconv.Itoa(i)] = str
		}
	})

	b.Run("GigaCache", func(b *testing.B) {
		perfbench.Open(b)
		m := New(DefaultOptions)
		for i := 0; i < b.N; i++ {
			m.Set(strconv.Itoa(i), str)
		}
	})

	b.Run("GigaCache/disableEvict", func(b *testing.B) {
		perfbench.Open(b)
		options := DefaultOptions
		options.DisableEvict = true
		m := New(options)
		for i := 0; i < b.N; i++ {
			m.Set(strconv.Itoa(i), str)
		}
	})
}

func BenchmarkGet(b *testing.B) {
	m1 := getStdmap()
	b.Run("stdmap", func(b *testing.B) {
		perfbench.Open(b)
		for i := 0; i < b.N; i++ {
			_ = m1[strconv.Itoa(i)]
		}
	})

	m2 := New(DefaultOptions)
	for i := 0; i < num; i++ {
		m2.Set(strconv.Itoa(i), str)
	}
	b.Run("GigaCache", func(b *testing.B) {
		perfbench.Open(b)
		for i := 0; i < b.N; i++ {
			m2.Get(strconv.Itoa(i))
		}
	})
}

func BenchmarkScan(b *testing.B) {
	b.Run("stdmap", func(b *testing.B) {
		m := getStdmap()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for k, v := range m {
				_, _ = k, v
			}
		}
	})

	b.Run("GigaCache", func(b *testing.B) {
		m := New(DefaultOptions)
		for i := 0; i < num; i++ {
			m.Set(strconv.Itoa(i), str)
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			m.Scan(func(s []byte, b []byte, i int64) bool {
				return true
			})
		}
	})
}

func BenchmarkRemove(b *testing.B) {
	b.Run("stdmap", func(b *testing.B) {
		m := getStdmap()
		b.ResetTimer()
		for i := 0; i < num; i++ {
			delete(m, strconv.Itoa(i))
		}
	})

	b.Run("GigaCache", func(b *testing.B) {
		m := New(DefaultOptions)
		for i := 0; i < num; i++ {
			m.Set(strconv.Itoa(i), str)
		}
		b.ResetTimer()
		for i := 0; i < num; i++ {
			m.Remove(strconv.Itoa(i))
		}
	})
}

func BenchmarkConvTTL(b *testing.B) {
	b.Run("newIdx", func(b *testing.B) {
		idx := newIdx(1024, time.Now().Unix())
		for i := 0; i < b.N; i++ {
			newIdx(idx.start(), idx.TTL())
		}
	})
	b.Run("newIdxx", func(b *testing.B) {
		idx := newIdx(1024, time.Now().Unix())
		for i := 0; i < b.N; i++ {
			newIdxx(idx.start(), idx)
		}
	})
}
