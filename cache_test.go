package cache

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	num = 10000
)

var (
	str = []byte("0123456789")
	sec = time.Second / 20
)

func assertCacheNil(a *assert.Assertions, val any, ts int64, ok bool) {
	a.Equal(val, nil)
	a.Equal(ts, int64(0))
	a.Equal(ok, false)
}

func TestCacheSet(t *testing.T) {
	t.Run("Set/Get", func(t *testing.T) {
		assert := assert.New(t)

		m := New(100)
		for i := 0; i < num; i++ {
			m.Set("foo"+strconv.Itoa(i), []byte(strconv.Itoa(i)))
		}

		// get exist
		val, ts, ok := m.Get("foo123")
		assert.Equal(val, []byte("123"))
		assert.Equal(ts, int64(0))
		assert.Equal(ok, true)

		// update get
		m.Set("foo100", []byte("200"))
		val, ts, ok = m.Get("foo100")
		assert.Equal(val, []byte("200"))
		assert.Equal(ts, int64(0))
		assert.Equal(ok, true)

		// has
		assert.Equal(m.Has("foo100"), true)

		// get not exist
		val, ts, ok = m.Get("not-exist")
		assertCacheNil(assert, val, ts, ok)

		// set negetive number
		m.SetTx("no", []byte{1}, -9)
		val, ts, ok = m.Get("no")
		assertCacheNil(assert, val, ts, ok)

		// get deleted
		ok = m.Delete("foo5")
		assert.Equal(ok, true, "delete error")

		val, ts, ok = m.Get("foo5")
		assertCacheNil(assert, val, ts, ok)

		// get expired
		m.SetEx("test", []byte{1}, sec)
		time.Sleep(sec * 2)
		val, ts, ok = m.Get("test")
		assertCacheNil(assert, val, ts, ok)

		{
			m := New(1)
			// test set inplace
			m.Set("myInt", 1)
			assert.Equal(len(m.buckets[0].items), 1)
			m.Set("myInt", 2)
			assert.Equal(len(m.buckets[0].items), 1)
			m.Set("myInt2", 3)
			assert.Equal(len(m.buckets[0].items), 2)
		}
	})

	t.Run("Nocopy", func(t *testing.T) {
		assert := assert.New(t)
		m := New(1)

		// get nocopy
		m.SetEx("nocopy", []byte{1, 2, 3, 4}, time.Minute)

		m.buckets[0].scan(func(k string, val any, i int64) bool {
			if val, ok := val.([]byte); ok {
				copy(val, []byte{8, 8, 8, 8})
			}
			return true
		}, true)

		val, ts, ok := m.Get("nocopy")
		assert.Equal(val, []byte{8, 8, 8, 8})
		assert.GreaterOrEqual(ts, GetClock())
		assert.Equal(ok, true)

		// get copy
		m = New(1)
		m.SetEx("copy", []byte{1, 2, 3, 4}, time.Minute)

		m.buckets[0].scan(func(k string, val any, i int64) bool {
			if val, ok := val.([]byte); ok {
				copy(val, []byte{8, 8, 8, 8})
			}
			return true
		})

		val, ts, ok = m.Get("copy")
		assert.Equal(val, []byte{1, 2, 3, 4})
		assert.GreaterOrEqual(ts, GetClock())
		assert.Equal(ok, true)
	})

	t.Run("Alloc", func(t *testing.T) {
		assert := assert.New(t)
		m := New(10)

		// init
		var alloc, inused int

		test := func() {
			stat := m.Stat()
			assert.Equal(stat.ExpRate(), float64(inused)/float64(alloc)*100)
			assert.Equal(alloc, int(stat.BytesAlloc))
			assert.Equal(inused, int(stat.BytesInused))
		}

		// bytes
		for i := 0; i < 1000; i++ {
			k := "key" + strconv.Itoa(i)
			m.Set(k, str)

			alloc += (len(k) + len(str))
			inused += (len(k) + len(str))
			test()
		}

		// any
		for i := 0; i < 1000; i++ {
			k := "any" + strconv.Itoa(i)
			m.Set(k, i)

			alloc += 8
			inused += 8
			test()
		}

		// expired
		for i := 0; i < 1000; i++ {
			k := "exp" + strconv.Itoa(i)
			m.SetEx(k, str, sec)

			alloc += (len(k) + len(str))
			inused += (len(k) + len(str))
			test()
		}

		time.Sleep(sec * 2)

		for i := 0; i < 1000; i++ {
			k := "exp" + strconv.Itoa(i)
			inused -= (len(k) + len(str))
		}

		m.Migrate()

		stat := m.Stat()
		assert.Equal(inused, int(stat.BytesInused))
	})

	t.Run("Keys", func(t *testing.T) {
		m := New(20)
		for i := 0; i < 200; i++ {
			m.Set("noexp"+strconv.Itoa(i), str)
			m.SetEx(strconv.Itoa(i), str, sec)
		}
		for i := 0; i < 200; i++ {
			m.Set("any"+strconv.Itoa(i), i)
		}

		keys := m.Keys()
		if len(keys) != 600 {
			t.Fatalf("%+v", len(keys))
		}

		time.Sleep(sec * 2)

		keys = m.Keys()
		if len(keys) != 400 {
			t.Fatalf("%+v", len(keys))
		}
	})

	t.Run("Scan", func(t *testing.T) {
		assert := assert.New(t)
		m := New(20)

		for i := 0; i < 1000; i++ {
			m.Set("a"+strconv.Itoa(i), []byte(strconv.Itoa(i)))
			m.SetEx("b"+strconv.Itoa(i), []byte(strconv.Itoa(i)), sec)
			m.Set("c"+strconv.Itoa(i), i)
			m.SetEx("d"+strconv.Itoa(i), i, sec)
		}

		m.Scan(func(k string, a any, i int64) bool {
			id := k[1:]
			switch k[0] {
			case 'a':
				assert.Equal(string(a.([]byte)), id)

			case 'b':
				assert.Equal(string(a.([]byte)), id)

			case 'c':
				n, _ := strconv.Atoi(id)
				assert.Equal(a, n)

			case 'd':
				n, _ := strconv.Atoi(id)
				assert.Equal(a, n)
			}
			return true
		})

		time.Sleep(sec * 2)

		m.Scan(func(k string, a any, i int64) bool {
			id := k[1:]
			switch k[0] {
			case 'a':
				assert.Equal(string(a.([]byte)), id)

			case 'c':
				n, _ := strconv.Atoi(id)
				assert.Equal(a, n)

			case 'b', 'd':
				t.Fatalf("want expired, got %v", a)
			}
			return true
		})
	})

	t.Run("marshal", func(t *testing.T) {
		assert := assert.New(t)
		m := New()

		for i := 0; i < num; i++ {
			key := strconv.Itoa(i)
			value := []byte(key)

			m.SetEx("any"+key, i, time.Minute)
			m.SetEx(key, value, time.Minute)
		}

		{
			var anyCount int
			_, err := m.MarshalBytesFunc(func(k string, a any, i int64) {
				anyCount++
			})
			assert.Nil(err)
			assert.Equal(anyCount, num)
		}

		src, err := m.MarshalBytes()
		assert.Nil(err)

		m1 := New()
		assert.Nil(m1.UnmarshalBytes(src))

		// unmarshal error
		assert.NotNil(m.UnmarshalBytes([]byte("fake news")))
	})

	t.Run("eliminate", func(t *testing.T) {
		m := New()
		for i := 0; i < 10000; i++ {
			m.SetEx(strconv.Itoa(i), i, sec)
		}
		for i := 0; i < 10000; i++ {
			m.SetEx("t"+strconv.Itoa(i), str, sec)
		}
		for i := 0; i < 10000; i++ {
			m.SetEx("x"+strconv.Itoa(i), str, sec*999)
		}

		time.Sleep(sec * 2)

		for i := 0; i < 10000; i++ {
			m.Set("just-for-trig", []byte{})
		}
	})

	t.Run("alloc", func(t *testing.T) {
		assert := assert.New(t)
		m := New(1)
		b := m.buckets[0]

		m.Set("foo", []byte("bar"))

		assert.Equal(b.bytes[0:3], []byte("foo"))
		assert.Equal(b.bytes[3:6], []byte("bar"))

		m.SetEx("exp", []byte("abcd"), sec)
		assert.Equal(b.bytes[6:9], []byte("exp"))
		assert.Equal(b.bytes[9:13], []byte("abcd"))

		time.Sleep(sec * 2)

		m.Set("trig", []byte("123")) // "trig" will replace positon of "exp".

		assert.Equal(b.bytes[6:10], []byte("trig"))
		assert.Equal(b.bytes[10:13], []byte("123"))

		// stat
		stat := m.Stat()
		assert.Equal(int(stat.BytesAlloc), 13)
		assert.Equal(int(stat.BytesInused), 13)

		m.Set("res", []byte("type"))

		// delete
		ok := m.Delete("foo")
		assert.True(ok)

		stat = m.Stat()
		assert.Equal(int(stat.BytesAlloc), 20)
		assert.Equal(int(stat.BytesInused), 14)
		_ = stat.EvictRate()
	})
}

func FuzzSet(f *testing.F) {
	m := New()

	f.Fuzz(func(t *testing.T, key string, val []byte, ts int64) {
		f := func(ts int64) {
			now := GetClock()
			m.SetTx(key, val, ts)
			v, ttl, ok := m.Get(key)

			// no ttl
			if ts == 0 {
				assert.Equal(t, v, val)
				assert.Equal(t, ttl, int64(0))
				assert.Equal(t, ok, true)

				// expired
			} else if ts < now {
				assertCacheNil(assert.New(t), v, ttl, ok)

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
