package cache

import (
	"maps"
	"testing"
)

const N = 100 * 10000

func getStdmap(num int) map[string][]byte {
	m := map[string][]byte{}
	for i := 0; i < num; i++ {
		k, v := genKV(i)
		m[k] = v
	}
	return m
}

func getCache(num int, options ...Options) *GigaCache {
	opt := DefaultOptions
	if len(options) > 0 {
		opt = options[0]
	}
	m := New(opt)
	for i := 0; i < num; i++ {
		k, v := genKV(i)
		m.Set(k, v)
	}
	return m
}

func BenchmarkSet(b *testing.B) {
	b.Run("stdmap", func(b *testing.B) {
		m := map[string][]byte{}
		for i := 0; i < b.N; i++ {
			k, v := genKV(i)
			m[k] = v
		}
	})
	b.Run("cache", func(b *testing.B) {
		m := New(DefaultOptions)
		for i := 0; i < b.N; i++ {
			k, v := genKV(i)
			m.Set(k, v)
		}
	})
	b.Run("cache/disableEvict", func(b *testing.B) {
		options := DefaultOptions
		options.EvictInterval = -1
		m := New(options)
		for i := 0; i < b.N; i++ {
			k, v := genKV(i)
			m.Set(k, v)
		}
	})
}

func BenchmarkGet(b *testing.B) {
	b.Run("stdmap", func(b *testing.B) {
		m := getStdmap(N)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			k, _ := genKV(i)
			_ = m[k]
		}
	})
	b.Run("cache", func(b *testing.B) {
		m := getCache(N)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			k, _ := genKV(i)
			m.Get(k)
		}
	})
}

func BenchmarkScan(b *testing.B) {
	b.Run("stdmap", func(b *testing.B) {
		m := getStdmap(N)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for k, v := range m {
				_, _ = k, v
			}
		}
	})
	b.Run("cache", func(b *testing.B) {
		m := getCache(N)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			m.Scan(func(s, b []byte, i int64) bool {
				return true
			})
		}
	})
}

func BenchmarkRemove(b *testing.B) {
	b.Run("stdmap", func(b *testing.B) {
		m := getStdmap(N)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			k, _ := genKV(i)
			delete(m, k)
		}
	})
	b.Run("cache", func(b *testing.B) {
		m := getCache(N)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			k, _ := genKV(i)
			m.Remove(k)
		}
	})
}

func BenchmarkMigrate(b *testing.B) {
	b.Run("stdmap", func(b *testing.B) {
		m := getStdmap(100000)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			maps.Clone(m)
		}
	})
	b.Run("cache", func(b *testing.B) {
		m := getCache(100000)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			m.Migrate()
		}
	})
}
