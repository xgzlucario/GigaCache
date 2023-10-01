package cache

import (
	"math"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tidwall/hashmap"
)

const (
	num = 100 * 10000
)

var (
	str = []byte("0123456789")
	sec = time.Second / 20
)

func TestCacheSet(t *testing.T) {
	t.Run("Set/Get", func(t *testing.T) {
		assert := assert.New(t)

		m := New[string](100)
		for i := 0; i < 10000; i++ {
			m.Set("foo"+strconv.Itoa(i), []byte(strconv.Itoa(i)))
		}

		// get exist
		v, ts, ok := m.Get("foo123")
		assert.Equal(v, []byte("123"))
		assert.Equal(ts, int64(0))
		assert.Equal(ok, true)

		// update get
		m.Set("foo100", []byte("200"))
		v, ts, ok = m.Get("foo100")
		assert.Equal(v, []byte("200"))
		assert.Equal(ts, int64(0))
		assert.Equal(ok, true)

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
		m := New[string](20)
		for i := 0; i < 600; i++ {
			m.Set(strconv.Itoa(i), str)
		}
		for i := 0; i < 200; i++ {
			m.Set(strconv.Itoa(i), i)
			m.Set("any"+strconv.Itoa(i), i)
		}

		s := m.Stat()
		if s.LenBytes != 6000 || s.Len != 800 || s.AllocTimes != 1000 || s.LenAny != 400 {
			t.Fatalf("%+v", s)
		}
		if s.ExpRate() != 80 {
			t.Fatalf("%+v", s.ExpRate())
		}
	})

	t.Run("Scan", func(t *testing.T) {
		m := New[string](20)
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

	t.Run("migrate", func(t *testing.T) {
		const NUM = 1000

		m := New[string](1)
		assert := assert.New(t)

		for i := 0; i < NUM; i++ {
			m.Set(strconv.Itoa(i), []byte{byte(i)})
			m.SetEx("t"+strconv.Itoa(i), []byte{byte(i)}, sec)
			m.SetEx("x"+strconv.Itoa(i), []byte{byte(i)}, sec*999)
		}

		// valid
		for i := 0; i < NUM; i++ {
			// noTTL
			v, ts, ok := m.Get(strconv.Itoa(i))
			assert.Equal(v, []byte{byte(i)})
			assert.Equal(ts, int64(0))
			assert.Equal(ok, true)

			// not expired
			v, ts, ok = m.Get("t" + strconv.Itoa(i))
			assert.Equal(v, []byte{byte(i)})
			assert.GreaterOrEqual(GetUnixNano()+int64(sec), ts)
			assert.Equal(ok, true)

			// not expired
			v, ts, ok = m.Get("x" + strconv.Itoa(i))
			assert.Equal(v, []byte{byte(i)})
			assert.GreaterOrEqual(GetUnixNano()+int64(sec*999), ts)
			assert.Equal(ok, true)
		}

		time.Sleep(sec * 2)

		// make some expired data to trigger migrate.
		for i := 0; i < NUM*5; i++ {
			m.SetEx("r"+strconv.Itoa(i), []byte{byte(i)}, 1)
		}

		// valid function
		validFunc := func() {
			for i := 0; i < NUM; i++ {
				// noTTL
				v, ts, ok := m.Get(strconv.Itoa(i))
				assert.Equal(v, []byte{byte(i)})
				assert.Equal(ts, int64(0))
				assert.Equal(ok, true)

				// expired
				v, ts, ok = m.Get("t" + strconv.Itoa(i))
				assert.Equal(v, nil)
				assert.Equal(ts, int64(0))
				assert.Equal(ok, false)

				// not expired
				v, ts, ok = m.Get("x" + strconv.Itoa(i))
				assert.Equal(v, []byte{byte(i)})
				assert.GreaterOrEqual(GetUnixNano()+int64(sec*999), ts)
				assert.Equal(ok, true)
			}
		}

		b := m.buckets[0]
		// force rehash
		b.rehashing = true
		b.nb = &bucket[string]{
			idx:    hashmap.New[string, Idx](b.idx.Len()),
			bytes:  bpool.Get(),
			anyArr: make([]*anyItem, 0),
		}

		var last1, last2 int

		// some set operations to tigger migrate.
		for i := 0; i < NUM; i++ {
			m.Set("none"+strconv.Itoa(i), []byte{1})

			if b.rehashing {
				if last1 > 0 && last1-b.idx.Len() != 100 {
					t.Fatalf("error: %v %v", b.idx.Len(), last1)
				}

				if last2 > 0 && b.nb.idx.Len()-last2 > 100 {
					t.Fatalf("error: %v %v", b.idx.Len(), last2)
				}

				last1 = b.idx.Len()
				last2 = b.nb.idx.Len()

				// scan
				b.scan(func(k string, a any, i int64) bool {
					return true
				})

			} else {
				last1 = 0
				last2 = 0
			}

			validFunc()
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
		f := func(ts int64) {
			now := GetUnixNano()
			m.SetTx(key, val, ts)
			v, ttl, ok := m.Get(key)

			// no ttl
			if ts == 0 {
				assert.Equal(t, v, val)
				assert.Equal(t, ttl, int64(0))
				assert.Equal(t, ok, true)

				// expired
			} else if ts < now {
				assert.Equal(t, v, nil)
				assert.Equal(t, ttl, int64(0))
				assert.Equal(t, ok, false)

				// not expired
			} else if ts > now {
				assert.Equal(t, v, val)
				assert.Equal(t, ts, ttl)
				assert.Equal(t, ok, true)
			}
		}

		f(ts)
		f(math.MaxInt64 - ts)
	})
}
