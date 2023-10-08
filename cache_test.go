package cache

import (
	"math"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	num = 100 * 10000
)

var (
	str = []byte("0123456789")
	sec = time.Second / 20
)

type MyInt int64

func NewInt(i int) *MyInt {
	n := MyInt(i)
	return &n
}

func (i *MyInt) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatInt(int64(*i), 10)), nil
}

func (i *MyInt) UnmarshalJSON(b []byte) error {
	n, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		return err
	}
	*i = MyInt(n)
	return nil
}

func TestCacheSet(t *testing.T) {
	t.Run("Set/Get", func(t *testing.T) {
		assert := assert.New(t)

		m := New[string](100)
		for i := 0; i < 10000; i++ {
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
				assert.Equal(val, nil)
				assert.Equal(ts, int64(0))
				assert.Equal(ok, false)
			}

			val, ts, ok = m.Get(args[1])
			assert.Equal(val, []byte("200"))
			assert.Equal(ts, int64(0))
			assert.Equal(ok, true)
		}

		// Rename not exist
		m.Rename("not-exist", "not-exist2")
		for _, args := range []string{"not-exist", "not-exist2"} {
			val, ts, ok = m.Get(args)
			assert.Equal(val, nil)
			assert.Equal(ts, int64(0))
			assert.Equal(ok, false)
		}

		// Rename expired
		m.SetEx("foo", []byte{1}, sec)
		time.Sleep(sec * 2)
		m.Rename("foo", "foo2")
		val, ts, ok = m.Get("foo")
		assert.Equal(val, nil)
		assert.Equal(ts, int64(0))
		assert.Equal(ok, false)

		// get not exist
		val, ts, ok = m.Get("not-exist")
		assert.Equal(val, nil)
		assert.Equal(ts, int64(0))
		assert.Equal(ok, false)

		// set negetive number
		m.SetTx("no", []byte{1}, -9)
		val, ts, ok = m.Get("no")
		assert.Equal(val, nil)
		assert.Equal(ts, int64(0))
		assert.Equal(ok, false)

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
		assert.GreaterOrEqual(ts, GetUnixNano())
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
		assert.GreaterOrEqual(ts, GetUnixNano())
		assert.Equal(ok, true)
	})

	t.Run("SetAny/GetAny", func(t *testing.T) {
		// assert := assert.New(t)

		// m := New[string](100)
		// for i := 0; i < 10000; i++ {
		// 	m.Set("foo"+strconv.Itoa(i), NewInt(i))
		// }

		// // get
		// v, ts, ok := m.Get("foo123")
		// assert.Equal(v, 123)
		// assert.Equal(ts, int64(0))
		// assert.Equal(ok, true)

		// // get not exist
		// v, ts, ok = m.Get("not-exist")
		// assert.Equal(v, nil)
		// assert.Equal(ts, int64(0))
		// assert.Equal(ok, false)

		// // expired
		// m.SetEx("foo", 1, sec)
		// time.Sleep(sec * 2)
		// v, ts, ok = m.Get("foo")
		// if v != nil || ts != 0 || ok {
		// 	t.Fatalf("%v %v %v", v, ts, ok)
		// }

		// // bytes to any
		// m.Set("test1", []byte{1, 2, 3})
		// m.Set("test1", 123)
		// if v, ts, ok = m.Get("test1"); v.(int) != 123 || ts != 0 || !ok {
		// 	t.Fatalf("%v %v %v", v, ts, ok)
		// }

		// // any to bytes
		// m.Set("test2", 123)
		// m.Set("test2", []byte{1, 2, 3})
		// if v, ts, ok := m.Get("test2"); !assert.Equal([]byte{1, 2, 3}, v) || ts != 0 || !ok {
		// 	t.Fatalf("%v %v %v", v, ts, ok)
		// }

		// // anyTx to anyTx
		// m.SetEx("test3", 123, time.Hour)
		// m.SetEx("test3", 234, time.Hour)
		// if v, ts, ok := m.Get("test3"); v.(int) != 234 || ts == 0 || !ok {
		// 	t.Fatalf("%v %v %v", v, ts, ok)
		// }
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
		m := NewCustom[string, *MyInt](20)
		for i := 0; i < 600; i++ {
			m.Set(strconv.Itoa(i), str)
		}
		for i := 0; i < 200; i++ {
			m.Set(strconv.Itoa(i), NewInt(i))
			m.Set("any"+strconv.Itoa(i), NewInt(i))
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
		m := NewCustom[string, *MyInt](20)
		for i := 0; i < 200; i++ {
			m.Set("noexp"+strconv.Itoa(i), str)
			m.SetEx(strconv.Itoa(i), str, sec)
		}
		for i := 0; i < 200; i++ {
			m.Set("any"+strconv.Itoa(i), NewInt(i))
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
		m := New[string](20)
		m.RandomGet()

		for i := 0; i < 200; i++ {
			if i%2 == 0 {
				m.SetEx(strconv.Itoa(i), str, sec)
			} else {
				m.Set(strconv.Itoa(i), str)
			}
		}

		time.Sleep(sec * 2)

		for i := 0; i < 200; i++ {
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
		m := NewCustom[string, *MyInt](20)

		for i := 0; i < 5000; i++ {
			m.Set("a"+strconv.Itoa(i), []byte(strconv.Itoa(i)))
		}
		for i := 0; i < 5000; i++ {
			m.SetEx("b"+strconv.Itoa(i), []byte(strconv.Itoa(i)), sec)
		}
		for i := 0; i < 5000; i++ {
			m.Set("c"+strconv.Itoa(i), NewInt(i))
		}
		for i := 0; i < 5000; i++ {
			m.SetEx("d"+strconv.Itoa(i), NewInt(i), sec)
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
				assert.Equal(a, NewInt(n))

			case 'd':
				n, _ := strconv.Atoi(id)
				assert.Equal(a, NewInt(n))
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
				assert.Equal(a, NewInt(n))

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
		if s.LenBytes != 700 || s.Len != 100 || s.Alloc != 100 {
			t.Fatalf("%+v", s)
		}

		time.Sleep(sec * 2)
		// trigger migrate
		for i := 0; i < 9999; i++ {
			m.Set("just-for-trig", []byte{})
		}

		// check2
		s = m.Stat()
		if s.LenBytes != 700 || s.Len != 101 {
			t.Fatalf("%+v", s)
		}
	})

	t.Run("Migrate", func(t *testing.T) {
		m := NewCustom[string, *MyInt]()
		m.buckets[0].eliminate()

		for i := 0; i < 100; i++ {
			m.Set("noexpired"+strconv.Itoa(i), []byte{1, 2, 3})
		}
		for i := 0; i < 200; i++ {
			m.SetEx("expired"+strconv.Itoa(i), []byte{1, 2, 3}, sec)
		}
		for i := 0; i < 300; i++ {
			m.Set("noexpired-any"+strconv.Itoa(i), NewInt(i))
		}
		for i := 0; i < 400; i++ {
			m.SetEx("expired-any"+strconv.Itoa(i), NewInt(123), sec)
		}

		// check
		s := m.Stat()
		if s.LenBytes != 2500 || s.Len != 1000 || s.Alloc != 1000 || s.LenAny != 700 {
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
		valid := map[string][]byte{}

		for i := 0; i < 10*10000; i++ {
			key := strconv.Itoa(rand.Int())
			value := []byte(key)

			m.SetEx(key, value, time.Hour)
			valid[key] = value
		}

		src, err := m.MarshalJSON()
		assert.Nil(err)

		m1 := New[string]()
		err = m1.UnmarshalJSON(src)
		assert.Nil(err)

		// check items
		for k, v := range valid {
			res, ts, ok := m1.Get(k)
			assert.Equal(res, v)
			assert.NotEqual(ts, int64(0))
			assert.Equal(ok, true)
		}

		// check count
		assert.Equal(len(valid), int(m1.Stat().Len))

		// unmarshal error
		err = m.UnmarshalJSON([]byte("fake news"))
		assert.NotNil(err)
	})

	t.Run("eliminate", func(t *testing.T) {
		m := NewCustom[string, *MyInt](100)
		for i := 0; i < 3000; i++ {
			m.SetEx(strconv.Itoa(i), NewInt(i), sec)
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
