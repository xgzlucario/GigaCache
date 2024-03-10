package cache

import (
	"bytes"
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

func getTestOption(num, interval int) Options {
	opt := DefaultOptions
	opt.ShardCount = 1
	opt.EvictInterval = interval
	opt.IndexSize = uint32(num)
	return opt
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
		// delete
		ok = m.Delete(k)
		assert.False(ok)
	}
	// scan
	beginKey := fmt.Sprintf("%08x", start)
	endKey := fmt.Sprintf("%08x", end)

	m.Scan(func(s []byte, b []byte, i int64) bool {
		if string(s) >= beginKey && string(s) < endKey {
			assert.Fail("invalid data")
		}
		assert.Equal(s, b)
		return false
	})

	m.Scan(func(s []byte, b []byte, i int64) bool {
		if string(s) >= beginKey && string(s) < endKey {
			assert.Fail("invalid data")
		}
		assert.Equal(s, b)
		return false
	}, WalkOptions{NumCPU: runtime.NumCPU(), NoCopy: true})
}

func TestSet(t *testing.T) {
	assert := assert.New(t)
	const num = 10000
	m := New(getTestOption(num, 3))

	// set data.
	for i := 0; i < num/2; i++ {
		k, v := genKV(i)
		m.Set(k, v)
	}
	for i := num / 2; i < num; i++ {
		k, v := genKV(i)
		m.SetEx(k, v, time.Hour)
	}

	m.Migrate(1)

	// get data.
	for i := 0; i < num/2; i++ {
		k, v := genKV(i)
		val, ts, ok := m.Get(k)
		assert.True(ok)
		assert.Equal(v, val)
		assert.Equal(ts, int64(0))
	}
	for i := num / 2; i < num; i++ {
		k, v := genKV(i)
		val, ts, ok := m.Get(k)
		assert.True(ok)
		assert.Equal(v, val)
		assert.Greater(ts, time.Now().UnixNano())
	}

	// setTTL
	ts := time.Now().UnixNano()
	for i := num / 2; i < num; i++ {
		k, _ := genKV(i)
		ok := m.SetTTL(k, ts)
		assert.True(ok)
	}

	// sleep.
	time.Sleep(time.Second)

	checkInvalidData(assert, m, num/2, num)
	m.Migrate(runtime.NumCPU())
	checkInvalidData(assert, m, num/2, num)

	// delete 0 ~ num/2.
	for i := 0; i < num/2; i++ {
		k, _ := genKV(i)
		ok := m.Delete(k)
		assert.True(ok)
	}
	checkInvalidData(assert, m, 0, num)
	m.Migrate(runtime.NumCPU())
	checkInvalidData(assert, m, 0, num)

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
	opt := getTestOption(num, 1)
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
	assert.Equal(stat.Len, uint64(num))
	assert.Equal(stat.Alloc, uint64(stat.Len*(16+2)))
	assert.Equal(stat.Inused, uint64(stat.Len*(16+2)))
	assert.Equal(stat.Evict, uint64(0))
	assert.Greater(stat.Probe, uint64(0))
	assert.Equal(stat.EvictRate(), float64(0))
	assert.Equal(stat.ExpRate(), float64(100))

	// trig evict.
	m.Set("trig1234", []byte("trig1234"))

	stat = m.Stat()
	assert.Equal(stat.Len, uint64(num-stat.Evict+1))
	assert.Equal(stat.Alloc, uint64(16+2))
	assert.Equal(stat.Inused, uint64(16+2))
	assert.Equal(stat.Migrates, uint64(1))
}

func TestDisableEvict(t *testing.T) {
	assert := assert.New(t)

	opt := DefaultOptions
	opt.ShardCount = 1
	opt.DisableEvict = true
	opt.IndexSize = uint32(num)

	m := New(opt)

	// set data.
	for i := 0; i < num; i++ {
		k, v := genKV(i)
		m.Set(k, v)
	}

	// stat
	stat := m.Stat()
	assert.Equal(stat.Len, uint64(num))
	assert.Equal(stat.Alloc, uint64(stat.Len*(16+2)))
	assert.Equal(stat.Inused, uint64(stat.Len*(16+2)))
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
	assert.Equal(stat.Len, uint64(num+1))
	assert.Equal(stat.Alloc, uint64((num+1)*(16+2)))
	assert.Equal(stat.Inused, uint64((num+1)*(16+2)))
	assert.Equal(stat.Migrates, uint64(0))
	assert.Equal(stat.Evict, uint64(0))
	assert.Equal(stat.Probe, uint64(0))
}

func FuzzSet(f *testing.F) {
	const num = 1000 * 10000
	m := New(getTestOption(num, 1))

	f.Fuzz(func(t *testing.T, k string, v []byte, u64ts uint64) {
		sec := GetNanoSec()
		ts := int64(u64ts)

		// set
		m.SetTx(k, v, ts)
		// get
		val, ts, ok := m.Get(k)

		switch {
		// no ttl
		case ts == 0:
			if !ok {
				t.Error("no ttl, but not found")
			}
			if !bytes.Equal(v, val) {
				t.Error("no ttl, but not equal")
			}
			if ts != 0 {
				t.Error("no ttl, but ts is not 0")
			}

		// expired
		case ts < sec:
			if ok {
				t.Error("expired, but found")
			}
			if ts != 0 {
				t.Error("expired, but ts is not 0")
			}
			if val != nil {
				t.Error("expired, but val is not nil")
			}

		// not expired
		case ts > sec:
			if !ok {
				t.Error("not expired, but not found")
			}
			if !bytes.Equal(v, val) {
				t.Error("not expired, but not equal")
			}
			if ts != (ts/timeCarry)*timeCarry {
				t.Error("not expired, ttl")
			}
		}
	})
}
