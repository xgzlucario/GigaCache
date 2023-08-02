package cache

import (
	"bytes"
	"strconv"
	"sync"
	"testing"
	"time"

	"golang.org/x/exp/rand"
)

const (
	num = 1000 * 10000
)

var (
	str = []byte("0123456789")
)

func TestIdx(t *testing.T) {
	for i := 0; i < 1e8; i++ {
		a, b := int(rand.Uint32()>>3), int(rand.Uint32()>>3)
		idx := newIdx(a, b, i%2 == 0, false)

		if idx.start() != a {
			t.Fatalf("%v != %v", idx.start(), a)
		}
		if idx.offset() != b {
			t.Fatalf("%v != %v", idx.offset(), b)
		}

		if i%2 == 0 {
			if !idx.hasTTL() {
				t.Fatal("a")
			}
		} else {
			if idx.hasTTL() {
				t.Fatal("b")
			}
		}
	}
}

func TestSetEx(t *testing.T) {
	m := New[string](1)
	m.Set("base", []byte("123"))

	m.Set("foo", []byte("1234"))
	l1 := m.bytesLen()
	m.Set("foo", []byte("789"))
	l2 := m.bytesLen()

	buf, ok := m.Get("foo")
	if !ok {
		t.Fatal("1")
	}
	if !bytes.Equal(buf, []byte("789")) {
		t.Fatal("2")
	}
	if l1 != l2 {
		t.Fatal("3")
	}

	m.Set("bar", []byte("000"))
	l3 := m.bytesLen()
	if l3 != l2+3 {
		t.Fatal("4")
	}

	m = New[string](1)
	m.Set("base", []byte("123"))

	m.SetEx("foo", []byte("1234"), time.Second)
	l1 = m.bytesLen()
	m.Set("foo", []byte("012345"))
	l2 = m.bytesLen()

	buf, ok = m.Get("foo")
	if !ok {
		t.Fatal("5")
	}
	if !bytes.Equal(buf, []byte("012345")) {
		t.Fatal("6")
	}
	if l1 != l2 {
		t.Fatal("7")
	}

	for i := 0; i < 100; i++ {
		m.SetAny("xgz"+strconv.Itoa(i), i)
	}
	for i := 0; i < 100; i++ {
		v, ok := m.GetAny("xgz" + strconv.Itoa(i))
		if !ok {
			t.Fatal("8")
		}
		if v.(int) != i {
			t.Fatal("9")
		}
	}
}

func TestCache(t *testing.T) {
	m := New[string]()
	vmap := make(map[string][]byte, num)
	tmap := make(map[string]int64, num)

	for i := 0; i < num/10; i++ {
		si := strconv.Itoa(i)
		t := time.Now().Add(time.Minute)

		m.SetTx(si, []byte(si), t.UnixNano())
		vmap[si] = []byte(si)
		tmap[si] = t.Unix() * int64(timeCarry)
	}

	// check value
	for k, v := range vmap {
		vv, ok := m.Get(k)
		if !ok {
			t.Fatal("not found")
		}
		if !bytes.Equal(v, vv) {
			t.Fatal("value not equal")
		}
	}

	// check time
	for k, v := range tmap {
		_, ts, ok := m.GetTx(k)
		if !ok {
			t.Fatal("not found")
		}
		if v != ts {
			t.Fatalf("time not equal: %v != %v", ts, v)
		}
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
		m4.SetEx(strconv.Itoa(i), str, time.Minute)
	}
	b.Run("gigacache/Tx", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m4.Delete(strconv.Itoa(i))
		}
	})
}
