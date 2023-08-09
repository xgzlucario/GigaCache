package cache

import (
	"bytes"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
		assert := assert.New(t)

		m := New[string](100)
		for i := 0; i < 10000; i++ {
			m.Set("foo"+strconv.Itoa(i), []byte(strconv.Itoa(i)))
		}

		// get any
		if v, ts, ok := m.GetAny("foo123"); v != nil || ts != 0 || ok {
			t.Fatalf("%v %v %v", v, ts, ok)
		}

		// update get
		m.Set("foo100", []byte("200"))
		if v, ts, ok := m.Get("foo100"); !bytes.Equal(v, []byte("200")) || !ok || ts != 0 {
			t.Fatalf("%v %v %v", v, ts, ok)
		}

		// get not exist
		val, ts, ok := m.Get("not-exist")
		if val != nil || ts != 0 || ok {
			t.Fatalf("%v %v %v", val, ts, ok)
		}

		// get deleted
		ok = m.Delete("foo5")
		assert.Equal(ok, true, "delete error")

		val, ts, ok = m.Get("foo5")
		if val != nil || ts != 0 || ok {
			t.Fatalf("%v %v %v", val, ts, ok)
		}

		// get expired
		m.SetEx("test", []byte{1}, sec)
		time.Sleep(sec * 2)
		val, ts, ok = m.Get("test")
		if val != nil || ts != -1 || ok {
			t.Fatalf("%v %v %v", val, ts, ok)
		}

		// set after expired
		s1 := m.Stat()
		m.SetEx("test", []byte{1}, sec)
		s2 := m.Stat()
		if s1.BytesLen != s2.BytesLen {
			t.Fatalf("%v %v", s1, s2)
		}
	})

	t.Run("SetAny/GetAny", func(t *testing.T) {
		m := New[string](100)
		for i := 0; i < 10000; i++ {
			m.SetAny("foo"+strconv.Itoa(i), i)
		}

		// get bytes
		if v, ts, ok := m.Get("foo123"); v != nil || ts != 0 || ok {
			t.Fatalf("%v %v %v", v, ts, ok)
		}

		// get any
		v, ts, ok := m.GetAny("foo123")
		if v.(int) != 123 || ts != 0 || !ok {
			t.Fatalf("%v %v %v", v, ts, ok)
		}

		// get not exist
		v, ts, ok = m.GetAny("not-exist")
		if v != nil || ts != 0 || ok {
			t.Fatalf("%v %v %v", v, ts, ok)
		}

		// expired
		m.SetAnyEx("foo", 1, sec)
		time.Sleep(sec * 2)
		v, ts, ok = m.GetAny("foo")
		if v != nil || ts != -1 || ok {
			t.Fatalf("%v %v %v", v, ts, ok)
		}

		// bytes to any
		m.Set("test1", []byte{1, 2, 3})
		m.SetAny("test1", 123)
		if v, ts, ok = m.GetAny("test1"); v.(int) != 123 || ts != 0 || !ok {
			t.Fatalf("%v %v %v", v, ts, ok)
		}

		// any to bytes
		m.SetAny("test2", 123)
		m.Set("test2", []byte{1, 2, 3})
		if v, ts, ok := m.Get("test2"); !bytes.Equal(v, []byte{1, 2, 3}) || ts != 0 || !ok {
			t.Fatalf("%v %v %v", v, ts, ok)
		}

		// anyTx to anyTx
		m.SetAnyEx("test3", 123, time.Hour)
		m.SetAnyEx("test3", 234, time.Hour)
		if v, ts, ok := m.GetAny("test3"); v.(int) != 234 || ts == 0 || !ok {
			t.Fatalf("%v %v %v", v, ts, ok)
		}
	})

	t.Run("int-generic", func(t *testing.T) {
		m := New[int](100)
		// set
		for i := 0; i < 9999; i++ {
			m.Set(i, []byte{1})
		}

		// get exist
		v, ts, ok := m.Get(1234)
		if !bytes.Equal(v, []byte{1}) || ts != 0 || !ok {
			t.Fatalf("%v %v %v", v, ts, ok)
		}

		// get not exist
		v, ts, ok = m.Get(20000)
		if v != nil || ts != 0 || ok {
			t.Fatalf("%v %v %v", v, ts, ok)
		}

		// expired
		m.SetEx(777, []byte{7, 7, 7}, sec)
		time.Sleep(sec * 2)
		v, ts, ok = m.Get(777)
		if v != nil || ts != -1 || ok {
			t.Fatalf("%v %v %v", v, ts, ok)
		}
	})

	t.Run("Stat", func(t *testing.T) {
		m := New[string](10)
		for i := 0; i < 500; i++ {
			m.Set(strconv.Itoa(i), str)
		}
		for i := 0; i < 200; i++ {
			m.Set(strconv.Itoa(i), str)
			m.SetAny("any"+strconv.Itoa(i), str)
		}

		s := m.Stat()
		if s.BytesLen != 5000 || s.Len != 700 || s.AllocLen != 700 || s.AnyLen != 200 {
			t.Fatalf("%+v", s)
		}
		if s.ExpRate() != 100 {
			t.Fatalf("%+v", s.ExpRate())
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

	t.Run("marshal", func(t *testing.T) {
		m := New[string]()
		testNum := 500

		for i := 0; i < testNum; i++ {
			m.SetEx(strconv.Itoa(rand.Int()), []byte{1, 2, 3}, time.Hour)
		}

		src, err := m.MarshalBytes()
		if err != nil {
			t.Fatalf("error: %v", err)
		}

		m1 := New[string]()
		if err := m1.UnmarshalBytes(src); err != nil {
			t.Fatalf("error: %v", err)
		}

		count := 0
		m1.Scan(func(k string, a any, i int64) bool {
			if !bytes.Equal(a.([]byte), []byte{1, 2, 3}) {
				t.Fatalf("error: %v", a)
			}
			count++
			return true
		})
		if count != testNum {
			t.Fatalf("error: %v", count)
		}

		// unmarshal error
		err = m.UnmarshalBytes([]byte("fake news"))
		if err == nil {
			t.Fatalf("error: %v", err)
		}
	})

	t.Run("eliminate", func(t *testing.T) {
		m := New[string]()
		m.Set("just-for-test", []byte{})

		for i := 0; i < 100; i++ {
			m.Set(strconv.Itoa(i), []byte{1})
		}
		for i := 0; i < 100; i++ {
			m.SetEx("t"+strconv.Itoa(i), []byte{1}, sec)
		}

		time.Sleep(sec)
		// trig compress
		for i := 0; i < 10; i++ {
			m.Set("just-for-test", []byte{})
		}
	})
}
