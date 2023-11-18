package cache

import (
	"strconv"
	"testing"

	"github.com/dolthub/swiss"
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
		m := New()
		for i := 0; i < b.N; i++ {
			m.Set(strconv.Itoa(i), str)
		}
	})

	b.Run("swissmap", func(b *testing.B) {
		m := swiss.NewMap[string, []byte](8)
		for i := 0; i < b.N; i++ {
			m.Put(strconv.Itoa(i), str)
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

	m2 := New()
	for i := 0; i < num; i++ {
		m2.Set(strconv.Itoa(i), str)
	}
	b.Run("GigaCache", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m2.Get(strconv.Itoa(i))
		}
	})

	m3 := swiss.NewMap[string, []byte](8)
	for i := 0; i < num; i++ {
		m3.Put(strconv.Itoa(i), str)
	}
	b.Run("swissmap", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m3.Get(strconv.Itoa(i))
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
		m := New()
		for i := 0; i < num; i++ {
			m.Set(strconv.Itoa(i), str)
		}
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			m.Delete(strconv.Itoa(i))
		}
	})

	b.Run("swissmap", func(b *testing.B) {
		m := swiss.NewMap[string, []byte](8)
		for i := 0; i < num; i++ {
			m.Put(strconv.Itoa(i), str)
		}
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			m.Delete(strconv.Itoa(i))
		}
	})
}
