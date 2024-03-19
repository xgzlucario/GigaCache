package cache

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func genKV(i int) (string, []byte) {
	k := fmt.Sprintf("%08x", i)
	return k, []byte(k)
}

func getOptions(num, interval int) Options {
	opt := DefaultOptions
	opt.ShardCount = 1
	opt.EvictInterval = interval
	opt.IndexSize = num
	opt.BufferSize = num * 16 * 2
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
	beginKey := fmt.Sprintf("%08x", start)
	endKey := fmt.Sprintf("%08x", end)

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
		ok = m.Remove(k)
		assert.False(ok)
	}
	// scan
	beginKey := fmt.Sprintf("%08x", start)
	endKey := fmt.Sprintf("%08x", end)

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
	const num = 10000
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
	time.Sleep(time.Second * 2)

	// check.
	{
		checkValidData(assert, m, 0, num*2/3)
		checkInvalidData(assert, m, num*2/3, num)
		m.Migrate(1)
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
		m.Migrate(runtime.NumCPU())
		checkValidData(assert, m, 0, num/3)
		checkInvalidData(assert, m, num/3, num)
	}

	// remove all.
	for i := 0; i < num/3; i++ {
		k, _ := genKV(i)
		assert.True(m.Remove(k))
	}
	for i := num / 3; i < num; i++ {
		k, _ := genKV(i)
		assert.False(m.Remove(k))
	}

	// check.
	{
		checkInvalidData(assert, m, 0, num)
		m.Migrate(runtime.NumCPU())
		checkInvalidData(assert, m, 0, num)
	}

	assert.Panics(func() {
		opt := DefaultOptions
		opt.ShardCount = 0
		New(opt)
	})

	assert.Panics(func() {
		opt := DefaultOptions
		opt.EvictInterval = -1
		New(opt)
	})
}

func TestEvict(t *testing.T) {
	assert := assert.New(t)
	const num = 10000
	opt := getOptions(num, 1)
	opt.OnRemove = func(k, v []byte) {
		assert.Equal(k, v)
	}
	m := New(opt)

	// set data.
	for i := 0; i < num; i++ {
		k, v := genKV(i)
		m.SetEx(k, v, time.Second)
	}
	time.Sleep(time.Second * 2)

	// stat
	stat := m.Stat()
	assert.Equal(stat.Len, num)
	assert.Equal(stat.Alloc, uint64(stat.Len*(16+2)))
	assert.Equal(stat.Unused, uint64(0))
	assert.Equal(stat.Evict, uint64(0))
	assert.Greater(stat.Probe, uint64(0))
	assert.Equal(stat.EvictRate(), float64(0))
	assert.Equal(stat.UnusedRate(), float64(0))

	// trig evict.
	m.Set("trig1234", []byte("trig1234"))

	stat = m.Stat()
	assert.Equal(stat.Len, int(num-stat.Evict+1))
	assert.Equal(stat.Alloc, uint64(16+2))
	assert.Equal(stat.Unused, uint64(0))
	assert.Equal(stat.Migrates, uint64(1))
}

func TestDisableEvict(t *testing.T) {
	assert := assert.New(t)

	opt := DefaultOptions
	opt.ShardCount = 1
	opt.DisableEvict = true
	opt.IndexSize = num

	m := New(opt)

	// set data.
	for i := 0; i < num; i++ {
		k, v := genKV(i)
		m.Set(k, v)
	}

	// stat
	stat := m.Stat()
	assert.Equal(stat.Len, num)
	assert.Equal(stat.Alloc, uint64(stat.Len*(16+2)))
	assert.Equal(stat.Unused, uint64(0))
	assert.Equal(stat.Migrates, uint64(0))
	assert.Equal(stat.Evict, uint64(0))
	assert.Equal(stat.Probe, uint64(0))

	// set same data.
	for i := 0; i < num/5; i++ {
		k, v := genKV(i)
		m.Set(k, v)
	}
	m.Set("trig1234", []byte("trig1234"))

	stat = m.Stat()
	assert.Equal(stat.Len, num+1)
	assert.Equal(stat.Alloc, uint64((num+1+num/5)*(16+2)))
	assert.Equal(stat.Unused, uint64(num/5*(16+2)))
	assert.Equal(stat.Migrates, uint64(0))
	assert.Equal(stat.Evict, uint64(0))
	assert.Equal(stat.Probe, uint64(0))
}

// hashTest is only for test.
func hashTest(str string) uint64 {
	return 1
}

func TestHashConflict(t *testing.T) {
	assert := assert.New(t)
	opt := DefaultOptions
	opt.ShardCount = 1
	opt.OnHashConflict = func(key, val []byte) {
		assert.NotEqual(key, val)
	}
	opt.HashFn = hashTest
	m := New(opt)

	for i := 0; i < 100; i++ {
		k, v := genKV(i)
		m.Set(k, v)
	}

	// max key size
	assert.Panics(func() {
		m.Set(string(make([]byte, maxKeySize+1)), []byte("hello"))
	})
}

func TestScanSmall(t *testing.T) {
	assert := assert.New(t)
	opt := DefaultOptions
	opt.ShardCount = 1024
	m := New(opt)

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
