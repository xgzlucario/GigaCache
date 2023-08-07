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
	sec = time.Second / 10
)

func TestCacheSet(t *testing.T) {
	t.Run("Set/Get", func(t *testing.T) {
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
		if !bytes.Equal(val, []byte("234")) || !ok || ts != 0 {
			t.Fatalf("%v %v %v", val, ts, ok)
		}

		// expired
		m.SetEx("test", []byte{1}, sec)
		time.Sleep(sec * 2)
		val, ts, ok = m.Get("test")
		if val != nil || ts != -1 || ok {
			t.Fatalf("%v %v %v", val, ts, ok)
		}
	})

	t.Run("SetAny/GetAny", func(t *testing.T) {
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
		m.SetAnyEx("test", 1, sec)
		time.Sleep(sec * 2)
		v, ts, ok = m.GetAny("test")
		if v != nil || ts != -1 || ok {
			t.Fatalf("%v %v %v", v, ts, ok)
		}
	})

	t.Run("Stat", func(t *testing.T) {
		m := New[string](10)
		for i := 0; i < 50; i++ {
			m.Set(strconv.Itoa(i), str)
			m.SetAny(strconv.Itoa(i), i)
		}

		s := m.Stat()
		if s.BytesLen != 500 || s.Len != 50 || s.AllocLen != 50 || s.AnyLen != 50 {
			t.Fatalf("%+v", s)
		}
	})

	t.Run("Scan", func(t *testing.T) {
		m := New[string](2)
		m.Set("xgz1", []byte{1, 2, 3})
		m.SetAny("xgz2", []byte{2, 3, 4})
		m.SetEx("xgz3", []byte{3, 4, 5}, sec)
		m.SetAnyEx("xgz4", []byte{4, 5, 6}, sec)

		m.Scan(func(k string, a any, i int64) bool {
			if k == "xgz1" && bytes.Equal(a.([]byte), []byte{1, 2, 3}) {
			} else if k == "xgz2" && bytes.Equal(a.([]byte), []byte{2, 3, 4}) {
			} else if k == "xgz3" && bytes.Equal(a.([]byte), []byte{3, 4, 5}) {
			} else if k == "xgz4" && bytes.Equal(a.([]byte), []byte{4, 5, 6}) {
			} else {
				t.Fatal(k, a)
			}
			return true
		})

		m.Scan(func(k string, a any, i int64) bool {
			if k == "xgz2" || k == "xgz4" {
				t.Fatal(k, a)
			}
			return true
		}, TypeByte)

		m.Scan(func(k string, a any, i int64) bool {
			if k == "xgz1" || k == "xgz3" {
				t.Fatal(k, a)
			}
			return true
		}, TypeAny)
	})

	t.Run("Compress", func(t *testing.T) {
		m := New[string]()

		for i := 0; i < 100; i++ {
			m.Set("noexpired"+strconv.Itoa(i), []byte{1, 2, 3})
		}
		for i := 0; i < 200; i++ {
			m.SetEx("expired"+strconv.Itoa(i), []byte{1, 2, 3}, sec)
		}
		for i := 0; i < 300; i++ {
			m.SetAny("noexpired-any"+strconv.Itoa(i), 123)
		}
		for i := 0; i < 400; i++ {
			m.SetAnyEx("expired-any"+strconv.Itoa(i), 123, sec)
		}

		// check
		s := m.Stat()
		if s.BytesLen != 2500 || s.Len != 1000 || s.AllocLen != 1000 || s.AnyLen != 700 {
			t.Fatalf("%+v", s)
		}

		time.Sleep(sec)
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
