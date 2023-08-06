package cache

import (
	"bytes"
	"strconv"
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
		m.SetEx("test", []byte{1}, time.Millisecond)
		time.Sleep(time.Millisecond * 2)
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
		m.SetAnyEx("test", 1, time.Millisecond)
		time.Sleep(time.Millisecond * 2)
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

	t.Run("compress", func(t *testing.T) {
		m := New[string]()

		for i := 0; i < 100; i++ {
			m.Set("noexpired"+strconv.Itoa(i), []byte{1, 2, 3})
		}
		for i := 0; i < 200; i++ {
			m.SetEx("expired"+strconv.Itoa(i), []byte{1, 2, 3}, time.Millisecond)
		}
		for i := 0; i < 300; i++ {
			m.SetAny("noexpired-any"+strconv.Itoa(i), 123)
		}
		for i := 0; i < 400; i++ {
			m.SetAnyEx("expired-any"+strconv.Itoa(i), 123, time.Millisecond)
		}

		// check
		s := m.Stat()
		if s.BytesLen != 2500 || s.Len != 1000 || s.AllocLen != 1000 || s.AnyLen != 700 {
			t.Fatalf("%+v", s)
		}

		time.Sleep(time.Millisecond * 2)
		m.Compress()

		// check2
		s = m.Stat()
		if s.BytesLen != 300 || s.Len != 400 || s.AllocLen != 400 || s.AnyLen != 300 {
			t.Fatalf("%+v", s)
		}

		// check3
		m.Scan(func(k string, a any, i int64) bool {
			if k[:3] == "exp" {
				t.Fatal(k)
			}
			return true
		})
	})
}
