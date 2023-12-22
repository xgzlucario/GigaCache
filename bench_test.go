package cache

import (
	"strconv"
	"testing"
)

var (
	num = 10 * 10000
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
		m := map[string][]byte{}
		for i := 0; i < b.N; i++ {
			m[strconv.Itoa(i)] = str
		}
	})

	b.Run("GigaCache", func(b *testing.B) {
		m := New(DefaultOption)
		for i := 0; i < b.N; i++ {
			m.Set(strconv.Itoa(i), str)
		}
	})
}

func BenchmarkGet(b *testing.B) {
	m1 := getStdmap()
	b.Run("stdmap", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = m1[strconv.Itoa(i)]
		}
	})

	m2 := New(DefaultOption)
	for i := 0; i < num; i++ {
		m2.Set(strconv.Itoa(i), str)
	}
	b.Run("GigaCache", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m2.Get(strconv.Itoa(i))
		}
	})
}

func BenchmarkIter(b *testing.B) {
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
		m := New(DefaultOption)
		for i := 0; i < num; i++ {
			m.Set(strconv.Itoa(i), str)
		}
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			m.Scan(func(s []byte, b []byte, i int64) bool {
				return false
			})
		}
	})
}

func BenchmarkDelete(b *testing.B) {
	b.Run("stdmap", func(b *testing.B) {
		m := getStdmap()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			delete(m, strconv.Itoa(i))
		}
	})

	b.Run("GigaCache", func(b *testing.B) {
		m := New(DefaultOption)
		for i := 0; i < num; i++ {
			m.Set(strconv.Itoa(i), str)
		}
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			m.Delete(strconv.Itoa(i))
		}
	})
}
