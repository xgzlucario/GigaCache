package cache

import (
	"math/rand"
	"strconv"
	"testing"
	"time"

	rand2 "golang.org/x/exp/rand"
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

	b.Run("gigacache", func(b *testing.B) {
		m := New[string]()
		for i := 0; i < b.N; i++ {
			m.Set(strconv.Itoa(i), str)
		}
	})

	b.Run("gigacache/Ex", func(b *testing.B) {
		m := New[string]()
		for i := 0; i < b.N; i++ {
			m.SetEx(strconv.Itoa(i), str, time.Minute)
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

	m3 := New[string]()
	for i := 0; i < num; i++ {
		m3.Set(strconv.Itoa(i), str)
	}
	b.Run("gigacache", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m3.Get(strconv.Itoa(i))
		}
	})

	m4 := New[string]()
	for i := 0; i < num; i++ {
		m4.SetEx(strconv.Itoa(i), str, time.Minute)
	}
	b.Run("gigacache/Ex", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m4.Get(strconv.Itoa(i))
		}
	})
}

func BenchmarkDelete(b *testing.B) {
	m1 := getStdmap()
	b.Run("stdmap", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			delete(m1, strconv.Itoa(i))
		}
	})

	m3 := New[string]()
	for i := 0; i < num; i++ {
		m3.Set(strconv.Itoa(i), str)
	}
	b.Run("gigacache", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m3.Delete(strconv.Itoa(i))
		}
	})

	m4 := New[string]()
	for i := 0; i < num; i++ {
		m4.SetEx(strconv.Itoa(i), str, time.Minute)
	}
	b.Run("gigacache/Ex", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m4.Delete(strconv.Itoa(i))
		}
	})
}

func BenchmarkRand(b *testing.B) {
	b.Run("std", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			rand.Uint64()
		}
	})

	b.Run("exp/std", func(b *testing.B) {
		source := rand2.NewSource(uint64(time.Now().UnixNano()))
		for i := 0; i < b.N; i++ {
			source.Uint64()
		}
	})
}
