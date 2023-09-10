package cache

import (
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

		// get exist
		if v, ts, ok := m.Get("foo123"); v == nil || ts != 0 || !ok {
			t.Fatalf("%v %v %v", v, ts, ok)
		}

		// update get
		m.Set("foo100", []byte("200"))
		if v, ts, ok := m.Get("foo100"); !assert.Equal([]byte("200"), v, "error1") || !ok || ts != 0 {
			t.Fatalf("%v %v %v", v, ts, ok)
		}

		// get not exist
		val, ts, ok := m.Get("not-exist")
		if val != nil || ts != 0 || ok {
			t.Fatalf("%v %v %v", val, ts, ok)
		}

		// set negetive number
		m.SetTx("no", []byte{1}, -9)
		if val, ts, ok := m.Get("no"); val != nil || ts != 0 || ok {
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
		if val != nil || ts != 0 || ok {
			t.Fatalf("%v %v %v", val, ts, ok)
		}
	})

	t.Run("SetAny/GetAny", func(t *testing.T) {
		assert := assert.New(t)

		m := New[string](100)
		for i := 0; i < 10000; i++ {
			m.Set("foo"+strconv.Itoa(i), i)
		}

		// get
		if v, ts, ok := m.Get("foo123"); v == nil || ts != 0 || !ok {
			t.Fatalf("%v %v %v", v, ts, ok)
		}

		// get any
		v, ts, ok := m.Get("foo123")
		if v.(int) != 123 || ts != 0 || !ok {
			t.Fatalf("%v %v %v", v, ts, ok)
		}

		// get not exist
		v, ts, ok = m.Get("not-exist")
		if v != nil || ts != 0 || ok {
			t.Fatalf("%v %v %v", v, ts, ok)
		}

		// expired
		m.SetEx("foo", 1, sec)
		time.Sleep(sec * 2)
		v, ts, ok = m.Get("foo")
		if v != nil || ts != 0 || ok {
			t.Fatalf("%v %v %v", v, ts, ok)
		}

		// bytes to any
		m.Set("test1", []byte{1, 2, 3})
		m.Set("test1", 123)
		if v, ts, ok = m.Get("test1"); v.(int) != 123 || ts != 0 || !ok {
			t.Fatalf("%v %v %v", v, ts, ok)
		}

		// any to bytes
		m.Set("test2", 123)
		m.Set("test2", []byte{1, 2, 3})
		if v, ts, ok := m.Get("test2"); !assert.Equal([]byte{1, 2, 3}, v) || ts != 0 || !ok {
			t.Fatalf("%v %v %v", v, ts, ok)
		}

		// anyTx to anyTx
		m.SetEx("test3", 123, time.Hour)
		m.SetEx("test3", 234, time.Hour)
		if v, ts, ok := m.Get("test3"); v.(int) != 234 || ts == 0 || !ok {
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
		if !assert.Equal(t, []byte{1}, v) || ts != 0 || !ok {
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
		if v != nil || ts != 0 || ok {
			t.Fatalf("%v %v %v", v, ts, ok)
		}
	})

	t.Run("Stat", func(t *testing.T) {
		m := New[string](10)
		for i := 0; i < 500; i++ { // 500 bytes value
			m.Set(strconv.Itoa(i), str)
		}
		for i := 0; i < 200; i++ { // 200 any value
			m.Set(strconv.Itoa(i), i)
			m.Set("any"+strconv.Itoa(i), i)
		}

		s := m.Stat()
		if s.BytesLen != 5000 || s.Len != 700 || s.AllocLen != 700 || s.AnyLen != 400 {
			t.Fatalf("%+v", s)
		}
		if s.ExpRate() != 100 {
			t.Fatalf("%+v", s.ExpRate())
		}
	})

	t.Run("Scan", func(t *testing.T) {
		m := New[string](50)
		for i := 0; i < 5000; i++ {
			m.Set("a"+strconv.Itoa(i), []byte(strconv.Itoa(i)))
		}
		for i := 0; i < 5000; i++ {
			m.SetEx("b"+strconv.Itoa(i), []byte(strconv.Itoa(i)), sec)
		}
		for i := 0; i < 5000; i++ {
			m.Set("c"+strconv.Itoa(i), i)
		}
		for i := 0; i < 5000; i++ {
			m.SetEx("d"+strconv.Itoa(i), i, sec)
		}

		m.Scan(func(k string, a any, i int64) bool {
			id := k[1:]
			switch k[0] {
			case 'a':
				if string(a.([]byte)) != id {
					t.Fatalf("want %v, got %v", id, a)
				}

			case 'b':
				if string(a.([]byte)) != id {
					t.Fatalf("want %v, got %v", id, a)
				}

			case 'c':
				n, _ := strconv.Atoi(id)
				if a.(int) != n {
					t.Fatalf("want %v, got %v", id, a)
				}

			case 'd':
				n, _ := strconv.Atoi(id)
				if a.(int) != n {
					t.Fatalf("want %v, got %v", id, a)
				}
			}
			return true
		})

		time.Sleep(sec * 2)

		m.Scan(func(k string, a any, i int64) bool {
			id := k[1:]
			switch k[0] {
			case 'a':
				if string(a.([]byte)) != id {
					t.Fatalf("want %v, got %v", id, a)
				}

			case 'c':
				n, _ := strconv.Atoi(id)
				if a.(int) != n {
					t.Fatalf("want %v, got %v", id, a)
				}

			case 'b', 'd':
				t.Fatalf("want expired, got %v", a)
			}
			return true
		})
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
			m.Set("noexpired-any"+strconv.Itoa(i), 123)
		}
		for i := 0; i < 400; i++ {
			m.SetEx("expired-any"+strconv.Itoa(i), 123, sec)
		}

		// check
		s := m.Stat()
		if s.BytesLen != 2500 || s.Len != 1000 || s.AllocLen != 1000 || s.AnyLen != 700 {
			t.Fatalf("%+v", s)
		}

		time.Sleep(sec * 2)
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
		valid := map[string][]byte{}

		for i := 0; i < 10*10000; i++ {
			key := strconv.Itoa(rand.Int())
			value := []byte(key)

			m.SetEx(key, value, time.Hour)
			valid[key] = value
		}

		src, err := m.MarshalBytes()
		if err != nil {
			t.Fatalf("error: %v", err)
		}

		m1 := New[string]()
		if err := m1.UnmarshalBytes(src); err != nil {
			t.Fatalf("error: %v", err)
		}

		// check items
		for k, v := range valid {
			res, ts, ok := m1.Get(k)
			if !assert.Equal(t, res, v) || ts == 0 || !ok {
				t.Fatalf("error: %v %v %v", res, ts, ok)
			}
		}

		// check count
		if len(valid) != int(m.Stat().Len) {
			t.Fatalf("error: %v", m.Stat())
		}

		// unmarshal error
		err = m.UnmarshalBytes([]byte("fake news"))
		if err == nil {
			t.Fatalf("error: %v", err)
		}
	})

	t.Run("eliminate", func(t *testing.T) {
		m := New[string](100)
		for i := 0; i < 3000; i++ {
			m.SetEx(strconv.Itoa(i), 1, sec)
		}
		for i := 0; i < 3000; i++ {
			m.SetEx("t"+strconv.Itoa(i), []byte{1}, sec)
		}
		for i := 0; i < 3000; i++ {
			m.SetEx("x"+strconv.Itoa(i), []byte{1}, sec*999)
		}

		time.Sleep(sec * 2)
		for i := 0; i < 1000; i++ {
			m.Set("just-for-trig", []byte{})
		}
	})

	t.Run("clock", func(t *testing.T) {
		if GetUnixNano() != clock {
			t.Fatalf("error: %v", GetUnixNano())
		}
	})
}

func FuzzSet(f *testing.F) {
	m := New[string]()

	f.Fuzz(func(t *testing.T, key string, val []byte, ts int64) {
		now := GetUnixNano()
		m.SetTx(key, val, ts)
		v, ttl, ok := m.Get(key)

		// no ttl
		if ts == 0 {
			if v == nil || ttl != 0 || !ok {
				t.Fatalf("[0] set: %v %s %v get: %s %v %v", key, val, ts, v, ttl, ok)
			}

			// expired
		} else if ts < now {
			if v != nil || ttl != 0 || ok {
				t.Fatalf("[1] set: %v %s %v get: %s %v %v", key, val, ts, v, ttl, ok)
			}

			// not expired
		} else if ts > now {
			if !assert.Equal(t, v, val) || ts != ttl || !ok {
				t.Fatalf("[2] set: %v %s %v get: %s %v %v", key, val, ts, v, ttl, ok)
			}
		}
	})
}

func FuzzSetAny(f *testing.F) {
	m := New[string]()

	f.Fuzz(func(t *testing.T, key string, val int, ts int64) {
		now := GetUnixNano()
		m.SetTx(key, val, ts)
		v, ttl, ok := m.Get(key)

		// no ttl
		if ts == 0 {
			if v == nil || ttl != 0 || !ok {
				t.Fatalf("[0] set: %v %v %v get: %v %v %v", key, val, ts, v, ttl, ok)
			}

			// expired
		} else if ts < now {
			if v != nil || ttl != 0 || ok {
				t.Fatalf("[1] set: %v %v %v get: %v %v %v", key, val, ts, v, ttl, ok)
			}

			// not expired
		} else if ts > now {
			if v.(int) != val || ts != ttl || !ok {
				t.Fatalf("[2] set: %v %v %v get: %v %v %v", key, val, ts, v, ttl, ok)
			}
		}
	})
}
