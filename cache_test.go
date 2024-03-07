package cache

import (
	"bytes"
	"encoding/binary"
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

func genOption() Options {
	opt := DefaultOptions
	opt.ShardCount = 1
	return opt
}

func checkData(t *testing.T, m *GigaCache, start, end int) {
	assert := assert.New(t)
	now := time.Now().UnixNano()

	// range
	for i := start; i < end; i++ {
		k, _ := genKV(i)
		val, ts, ok := m.Get(k)
		assert.True(ok)
		assert.Equal(val, []byte(k))
		assert.Greater(ts, now)
	}
}

func checkInvalidData(t *testing.T, m *GigaCache, start, end int) {
	assert := assert.New(t)
	// range
	for i := start; i < end; i++ {
		// get
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

	for _, cpu := range []int{1, runtime.NumCPU()} {
		m.Scan(func(s []byte, b []byte, i int64) bool {
			if string(s) >= beginKey && string(s) < endKey {
				assert.Fail("invalid data")
			}
			assert.Equal(s, b)
			return false
		}, cpu)

		m.Migrate(cpu)
	}
}

func TestSet(t *testing.T) {
	assert := assert.New(t)
	const num = 10000
	m := New(genOption())

	// set data.
	for i := 0; i < num/2; i++ {
		k, v := genKV(i)
		m.Set(k, v)
	}
	for i := num / 2; i < num; i++ {
		k, v := genKV(i)
		m.SetEx(k, v, time.Hour)
	}

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

	checkInvalidData(t, m, num/2, num)

	// delete 0 ~ num/2.
	for i := 0; i < num/2; i++ {
		k, _ := genKV(i)
		ok := m.Delete(k)
		assert.True(ok)
	}

	checkInvalidData(t, m, 0, num)

	assert.Panics(func() {
		opt := DefaultOptions
		opt.ShardCount = 0
		New(opt)
	})
}

func TestEvict(t *testing.T) {
	assert := assert.New(t)
	const num = 10000
	opt := genOption()
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

func TestArena(t *testing.T) {
	const num = 10000
	assert := assert.New(t)

	options := genOption()
	// disabled migrate
	options.MigrateThresRatio = 0
	m := New(options)

	// set data.
	for i := 0; i < num; i++ {
		k, v := genKV(i)
		m.SetEx(k, v, time.Second)
	}

	time.Sleep(time.Second * 2)

	// set data.
	for i := num; i < num*2; i++ {
		k, v := genKV(i)
		m.SetEx(k, v, time.Second*3)
	}

	stat := m.Stat()
	assert.Equal(stat.Migrates, uint64(0))
	// this is a expected value, but not a fixed value.
	assert.Greater(stat.Reused, uint64(100))

	checkInvalidData(t, m, 0, num)
	checkData(t, m, num, num*2)
}

func FuzzSet(f *testing.F) {
	m := New(genOption())

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

func TestVarlen(t *testing.T) {
	assert := assert.New(t)
	for i := 0; i < 10*10000; i++ {
		l1 := varlen[int](i)
		buf := binary.AppendUvarint(nil, uint64(i))

		assert.Equal(l1, len(buf))
	}
}
