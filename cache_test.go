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

		m := New[string](100)
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

		// Rename
		renameArgs := [][]string{
			{"foo100", "foo100"},
			{"foo100", "foo200"},
			{"foo200", "foo100"},
		}
		for _, args := range renameArgs {
			m.Rename(args[0], args[1])

			if args[0] != args[1] {
				val, ts, ok = m.Get(args[0])
				assertCacheNil(assert, val, ts, ok)
			}

			val, ts, ok = m.Get(args[1])
			assert.Equal(val, []byte("200"))
			assert.Equal(ts, int64(0))
			assert.Equal(ok, true)
		}

		// Rename not exist
		m.Rename("not-exist", "not-exist")
		val, ts, ok = m.Get("not-exist")
		assertCacheNil(assert, val, ts, ok)

		m.Rename("not-exist", "not-exist2")
		for _, args := range []string{"not-exist", "not-exist2"} {
			val, ts, ok = m.Get(args)
			assertCacheNil(assert, val, ts, ok)
		}

		// Rename expired
		m.SetEx("foo", []byte{1}, sec)
		time.Sleep(sec * 2)
		m.Rename("foo", "foo2")
		val, ts, ok = m.Get("foo")
		assertCacheNil(assert, val, ts, ok)

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
			m := New[string](1)
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
		m := New[string](1)

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
		m = New[string](1)
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

	t.Run("int-generic", func(t *testing.T) {
		assert := assert.New(t)
		m := New[int](100)
		m.Set(100, []byte{1})

		// get exist
		val, ts, ok := m.Get(100)
		assert.Equal(val, []byte{1})
		assert.Equal(ts, int64(0))
		assert.Equal(ok, true)

		// get not exist
		val, ts, ok = m.Get(200)
		assertCacheNil(assert, val, ts, ok)

		// get expired
		m.SetEx(200, []byte{1, 2, 3}, sec)
		time.Sleep(sec * 2)

		val, ts, ok = m.Get(200)
		assertCacheNil(assert, val, ts, ok)
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
		if s.LenBytes != 6000 || s.Len != 800 || s.Alloc != 1000 || s.LenAny != 400 {
			t.Fatalf("%+v", s)
		}
		if s.ExpRate() != 80 {
			t.Fatalf("%+v", s.ExpRate())
		}
	})

	t.Run("Keys", func(t *testing.T) {
		m := New[string](20)
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

	t.Run("RandomGet", func(t *testing.T) {
		m := New[string](10)
		m.RandomGet()

		for i := 0; i < 100; i++ {
			if i%2 == 0 {
				m.SetEx(strconv.Itoa(i), str, sec)
			} else {
				m.Set(strconv.Itoa(i), str)
			}
		}

		time.Sleep(sec * 2)

		for i := 0; i < 100; i++ {
			key, _, _, _ := m.RandomGet()
			// if key is odd
			if i%2 != 0 {
				if _, err := strconv.Atoi(key); err != nil {
					t.Fatalf("%+v", err)
				}
			}
		}
	})

	t.Run("Scan", func(t *testing.T) {
		assert := assert.New(t)
		m := New[string](20)

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

	t.Run("Migrate-small", func(t *testing.T) {
		m := New[string]()
		m.buckets[0].eliminate()

		for i := 0; i < 50; i++ {
			m.Set("noexpired"+strconv.Itoa(i), []byte{1, 2, 3})
		}
		for i := 0; i < 50; i++ {
			m.SetEx("expired"+strconv.Itoa(i), []byte{1, 2, 3}, sec)
		}

		// check
		s := m.Stat()
		if s.LenBytes != 300 || s.Len != 100 || s.Alloc != 100 {
			t.Fatalf("%+v", s)
		}

		time.Sleep(sec * 2)
		// trigger migrate
		for i := 0; i < 9999; i++ {
			m.Set("just-for-trig", []byte{})
		}

		// check2
		s = m.Stat()
		if s.LenBytes != 300 || s.Len != 101 {
			t.Fatalf("%+v", s)
		}
	})

	t.Run("Migrate", func(t *testing.T) {
		m := New[string]()
		m.buckets[0].eliminate()

		for i := 0; i < 100; i++ {
			m.Set("noexpired"+strconv.Itoa(i), []byte{1, 2, 3})
		}
		for i := 0; i < 200; i++ {
			m.SetEx("expired"+strconv.Itoa(i), []byte{1, 2, 3}, sec)
		}
		for i := 0; i < 300; i++ {
			m.Set("noexpired-any"+strconv.Itoa(i), i)
		}
		for i := 0; i < 400; i++ {
			m.SetEx("expired-any"+strconv.Itoa(i), 123, sec)
		}

		// check
		s := m.Stat()
		if s.LenBytes != 900 || s.Len != 1000 || s.Alloc != 1000 || s.LenAny != 700 {
			t.Fatalf("%+v", s)
		}

		time.Sleep(sec * 2)
		m.Migrate()

		// check2
		s = m.Stat()
		if s.LenBytes != 300 || s.Len != 400 || s.Alloc != 400 || s.LenAny != 300 {
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
		assert := assert.New(t)
		m := New[string]()

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

		m1 := New[string]()
		assert.Nil(m1.UnmarshalBytes(src))

		// unmarshal error
		assert.NotNil(m.UnmarshalBytes([]byte("fake news")))
	})

	t.Run("eliminate", func(t *testing.T) {
		m := New[string](100)
		for i := 0; i < 1000; i++ {
			m.SetEx(strconv.Itoa(i), i, sec)
		}
		for i := 0; i < 1000; i++ {
			m.SetEx("t"+strconv.Itoa(i), []byte{1}, sec)
		}
		for i := 0; i < 1000; i++ {
			m.SetEx("x"+strconv.Itoa(i), []byte{1}, sec*999)
		}

		time.Sleep(sec * 2)
		for i := 0; i < 1000; i++ {
			m.Set("just-for-trig", []byte{})
		}
	})
}

func FuzzSet(f *testing.F) {
	m := New[string]()

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
