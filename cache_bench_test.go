package cache

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

func getStdmap() map[string][]byte {
	m := map[string][]byte{}
	for i := 0; i < num; i++ {
		m[strconv.Itoa(i)] = str
	}
	return m
}

func getSyncmap() *sync.Map {
	m := &sync.Map{}
	for i := 0; i < num; i++ {
		m.Store(strconv.Itoa(i), str)
	}
	return m
}

func BenchmarkSet(b *testing.B) {
	m1 := map[string][]byte{}
	b.Run("stdmap", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m1[strconv.Itoa(i)] = str
		}
	})

	m4 := sync.Map{}
	b.Run("syncmap", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m4.Store(strconv.Itoa(i), str)
		}
	})

	m2 := New[string]()
	b.Run("gigacache", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m2.Set(strconv.Itoa(i), str)
		}
	})

	m3 := New[string]()
	b.Run("gigacache/Ex", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m3.SetEx(strconv.Itoa(i), str, time.Minute)
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

	m2 := getSyncmap()
	b.Run("syncmap", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m2.Load(strconv.Itoa(i))
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

	m2 := getSyncmap()
	b.Run("syncmap", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m2.Delete(strconv.Itoa(i))
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
