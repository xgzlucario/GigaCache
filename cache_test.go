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

func getTestOption(num int) Option {
	opt := DefaultOption
	opt.ShardCount = 1
	opt.DefaultIdxMapSize = uint32(num)
	opt.DefaultBufferSize = 16 * 6
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
}

func TestSet(t *testing.T) {
	fmt.Println("===== TestSet =====")
	assert := assert.New(t)
	const num = 10000
	m := New(getTestOption(num))

	// set data.
	for i := 0; i < num/2; i++ {
		k, v := genKV(i)
		m.Set(k, v)
	}
	for i := num / 2; i < num; i++ {
		k, v := genKV(i)
		m.SetEx(k, v, time.Hour)
	}

	m.Migrate()

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
	m.Migrate()
	checkInvalidData(assert, m, num/2, num)

	// delete 0 ~ num/2.
	for i := 0; i < num/2; i++ {
		k, _ := genKV(i)
		ok := m.Delete(k)
		assert.True(ok)
	}
	checkInvalidData(assert, m, 0, num)
	m.Migrate()
	checkInvalidData(assert, m, 0, num)

	assert.Panics(func() {
		opt := DefaultOption
		opt.ShardCount = 0
		New(opt)
	})
}
