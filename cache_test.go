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
	t.Run("case-bytes", func(t *testing.T) {
		m := New[string](2)
		// set
		m.Set("foo", []byte("123"))
		m.Set("bar", []byte("456"))

		if m.Stat().BytesLen != 6 {
			t.Fatalf("bytes len error: %d", m.Stat().BytesLen)
		}
		// update
		m.Set("foo", []byte("234"))

		if m.Stat().BytesLen != 6 {
			t.Fatalf("bytes len error: %d", m.Stat().BytesLen)
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
		// expired
		m.SetEx("test", []byte{1}, time.Second)
		time.Sleep(time.Second * 2)
		val, ts, ok = m.Get("test")
		if val != nil || ts != -1 || ok {
			t.Fatalf("%v %v %v", val, ts, ok)
		}
	})

	t.Run("case-any", func(t *testing.T) {
		m := New[string](2)
		// setAny
		m.SetAny("foo", 123)
		m.SetAny("bar", 456)

		// getAny
		v, ts, ok := m.GetAny("foo")
		if v.(int) != 123 || ts != 0 || !ok {
			t.Fatalf("%v %v %v", v, ts, ok)
		}

		// expired
		m.SetAnyEx("test", 1, time.Second)
		time.Sleep(time.Second * 2)
		v, ts, ok = m.GetAny("test")
		if v != nil || ts != -1 || ok {
			t.Fatalf("%v %v %v", v, ts, ok)
		}
	})

	t.Run("case-eliminate", func(t *testing.T) {
		m := New[string](10)
		for i := 0; i < 50; i++ {
			m.Set(strconv.Itoa(i), str)
			m.SetAny(strconv.Itoa(i), i)
		}

		stat := m.Stat()

		if stat.BytesLen != 500 {
			t.Fatalf("bytes len error: %d", stat.BytesLen)
		}
		if stat.Len != 50 {
			t.Fatalf("len != %d", stat.Len)
		}
	})
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
