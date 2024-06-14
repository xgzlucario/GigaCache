package cache

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func genKV(i int) (string, []byte) {
	k := fmt.Sprintf("%08x", i)
	return k, []byte(k)
}

func getOptions(num int, interval uint8) Options {
	opt := DefaultOptions
	opt.ShardCount = 1
	opt.EvictInterval = interval
	opt.IndexSize = num
	return opt
}

func checkValidData(assert *assert.Assertions, m *GigaCache, start, end int) {
	for i := start; i < end; i++ {
		k, _ := genKV(i)
		val, ts, ok := m.Get(k)
		assert.True(ok)
		assert.Equal(string(val), k)
		assert.GreaterOrEqual(ts, int64(0))
	}
	// scan
	beginKey, _ := genKV(start)
	endKey, _ := genKV(end)

	var count int
	m.Scan(func(key, val []byte, i int64) bool {
		if string(key) < beginKey || string(key) >= endKey {
			assert.Fail("invalid data")
		}
		assert.Equal(key, val)
		count++
		return true
	})
	assert.Equal(count, end-start)

	// scan break
	count = 0
	m.Scan(func(key, val []byte, i int64) bool {
		count++
		return count < (end-start)/2
	})
	assert.Equal(count, (end-start)/2)
}

func checkInvalidData(assert *assert.Assertions, m *GigaCache, start, end int) {
	// range
	for i := start; i < end; i++ {
		k, _ := genKV(i)
		val, ts, ok := m.Get(k)
		assert.False(ok)
		assert.Nil(val)
		assert.Equal(ts, int64(0))
		// setTTL
		ok = m.SetTTL(k, time.Now().UnixNano())
		assert.False(ok)
		// remove
		m.Remove(k)
	}
	// scan
	beginKey, _ := genKV(start)
	endKey, _ := genKV(end)

	m.Scan(func(key, val []byte, i int64) bool {
		if string(key) >= beginKey && string(key) < endKey {
			assert.Fail("invalid data")
		}
		assert.Equal(key, val)
		return true
	})
}

func TestCache(t *testing.T) {
	assert := assert.New(t)
	const num = 1000
	m := New(getOptions(num, 3))

	// init cache.
	for i := 0; i < num/3; i++ {
		k, v := genKV(i)
		m.Set(k, v)
	}
	for i := num / 3; i < num*2/3; i++ {
		k, v := genKV(i)
		m.SetEx(k, v, time.Hour)
	}
	for i := num * 2 / 3; i < num; i++ {
		k, v := genKV(i)
		m.SetEx(k, v, time.Second)
	}

	// wait for expired.
	time.Sleep(time.Second)

	// check.
	{
		checkValidData(assert, m, 0, num*2/3)
		checkInvalidData(assert, m, num*2/3, num)
		m.Migrate()
		checkValidData(assert, m, 0, num*2/3)
		checkInvalidData(assert, m, num*2/3, num)
	}

	// setTTL
	ts := time.Now().UnixNano()
	for i := num / 3; i < num*2/3; i++ {
		k, _ := genKV(i)
		assert.True(m.SetTTL(k, ts))
	}
	time.Sleep(time.Second)

	// check.
	{
		checkValidData(assert, m, 0, num/3)
		checkInvalidData(assert, m, num/3, num)
		m.Migrate()
		checkValidData(assert, m, 0, num/3)
		checkInvalidData(assert, m, num/3, num)
	}

	// remove all.
	for i := 0; i < num/3; i++ {
		k, _ := genKV(i)
		m.Remove(k)
	}
	for i := num / 3; i < num; i++ {
		k, _ := genKV(i)
		m.Remove(k)
	}

	// check.
	{
		checkInvalidData(assert, m, 0, num)
		m.Migrate()
		checkInvalidData(assert, m, 0, num)
	}

	assert.Panics(func() {
		opt := DefaultOptions
		opt.ShardCount = 0
		New(opt)
	})
}

func TestEvict(t *testing.T) {
	assert := assert.New(t)
	const num = 1000
	opt := getOptions(num, 1)
	m := New(opt)

	// set data.
	for i := 0; i < num; i++ {
		k, v := genKV(i)
		m.SetEx(k, v, time.Second)
	}
	time.Sleep(time.Second)

	// stat
	stat := m.GetStats()
	assert.Equal(stat.Len, num)
	assert.Equal(stat.Alloc, uint64(stat.Len*(16+2)))
	assert.Equal(stat.Unused, uint64(0))
	assert.Equal(stat.Evictions, uint64(0))
	assert.Greater(stat.Probes, uint64(0))
	assert.Equal(stat.EvictionRate(), float64(0))
	assert.Equal(stat.UnusedRate(), float64(0))

	// trig evict.
	m.Set("trig1234", []byte("trig1234"))

	stat = m.GetStats()
	assert.Equal(stat.Len, 1)
	assert.Equal(stat.Alloc, uint64(16+2))
	assert.Equal(stat.Unused, uint64(0))
	assert.Equal(stat.Migrates, uint64(1))
}

func TestDataAlloc(t *testing.T) {
	assert := assert.New(t)

	t.Run("memhash", func(t *testing.T) {
		opt := DefaultOptions
		opt.ShardCount = 1
		opt.DisableEvict = true
		m := New(opt)
		m.Set("hello", []byte("world"))

		m.Set("abc", []byte("123"))
		// stat
		stat := m.GetStats()
		assert.Equal(stat.Len, 2)
		assert.Equal(stat.Alloc, uint64(12+8))
		assert.Equal(stat.Unused, uint64(0))

		// set same data(update inplaced).
		m.Set("abc", []byte("234"))

		stat = m.GetStats()
		assert.Equal(stat.Len, 2)
		assert.Equal(stat.Alloc, uint64(12+8))
		assert.Equal(stat.Unused, uint64(0))

		// set great.
		m.Set("abc", []byte("12345"))

		stat = m.GetStats()
		assert.Equal(stat.Len, 2)
		assert.Equal(stat.Alloc, uint64(12+8+10))
		assert.Equal(stat.Unused, uint64(8))
	})
}

func TestScanSmall(t *testing.T) {
	assert := assert.New(t)
	m := New(DefaultOptions)

	for i := 0; i < 100; i++ {
		k, v := genKV(i)
		m.Set(k, v)
	}

	var count int
	m.Scan(func(key, val []byte, ttl int64) (next bool) {
		assert.Equal(key, val)
		assert.Equal(ttl, int64(0))
		count++
		return true
	})
	assert.Equal(count, 100)
}

func TestUtils(t *testing.T) {
	_ = SizeUvarint(1)
}

func TestHSetNewField(t *testing.T) {
	assert := assert.New(t)
	m := New(DefaultOptions)

	newField := m.Set("k1", []byte("v1"))
	assert.True(newField)

	newField = m.Set("k1", []byte("v1"))
	assert.False(newField)

	m.Remove("k1")
	newField = m.Set("k1", []byte("v1"))
	assert.True(newField)
}

func TestEvictManual(t *testing.T) {
	assert := assert.New(t)
	options := DefaultOptions
	options.ShardCount = 1
	options.DisableEvict = true

	m := New(options)

	m.SetEx("foo", []byte("bar"), time.Second)
	time.Sleep(time.Second)

	m.Set("test", []byte("test"))
	stat := m.GetStats()
	assert.Equal(stat.Len, 2)

	m.EvictExpiredKeys()
	stat = m.GetStats()
	assert.Equal(stat.Len, 1)
	assert.Equal(stat.Evictions, uint64(1))
}
