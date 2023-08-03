package cache

import (
	"bytes"
	"strconv"
	"sync"
	"testing"
	"time"
)

const (
	num = 1000 * 10000
)

var (
	str = []byte("0123456789")
)

func TestCacheSet(t *testing.T) {
	m := New[string](2)

	// set
	m.Set("foo", []byte("123"))
	m.Set("bar", []byte("456"))

	if m.bytesLen() != 6 {
		t.Fatalf("bytes len error: %d", m.bytesLen())
	}

	// update
	m.Set("foo", []byte("234"))

	if m.bytesLen() != 6 {
		t.Fatalf("bytes len error: %d", m.bytesLen())
	}

	// get
	val, ts, ok := m.Get("foo")
	if !ok {
		t.Fatal("1")
	}
	if !bytes.Equal(val, []byte("234")) {
		t.Fatal("2")
	}
	if ts != 0 {
		t.Fatal("3")
	}
}

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
	b.Run("gigacache/Tx", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m3.Set(strconv.Itoa(i), str, time.Minute)
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
		m4.Set(strconv.Itoa(i), str, time.Minute)
	}
	b.Run("gigacache/Tx", func(b *testing.B) {
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
		m4.Set(strconv.Itoa(i), str, time.Minute)
	}
	b.Run("gigacache/Tx", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m4.Delete(strconv.Itoa(i))
		}
	})
}
