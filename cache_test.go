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

func TestSet(t *testing.T) {
	fmt.Println("===== TestSet =====")
	assert := assert.New(t)
	const num = 10000

	opt := DefaultOption
	opt.ShardCount = 1
	opt.DefaultIdxMapSize = num
	opt.DefaultBufferSize = num * 16
	m := New(opt)

	// set data.
	for i := 0; i < num; i++ {
		k, v := genKV(i)
		if i%2 == 0 {
			m.Set(k, v)
		} else {
			m.SetEx(k, v, time.Second)
		}
	}

	// get data.
	for i := 0; i < num; i++ {
		k, v := genKV(i)
		val, ts, ok := m.Get(k)
		assert.True(ok)
		assert.Equal(v, val)
		if i%2 == 0 {
			assert.Equal(ts, int64(0))
		} else {
			assert.Greater(ts, time.Now().UnixNano())
		}
	}

	// sleep.
	time.Sleep(time.Second * 2)

	// get valid and expired data.
	for i := 0; i < num; i++ {
		k, v := genKV(i)
		val, ts, ok := m.Get(k)
		if i%2 == 0 {
			assert.True(ok)
			assert.Equal(v, val)
			assert.Equal(ts, int64(0))
		} else {
			assert.False(ok)
			assert.Nil(val)
			assert.Equal(ts, int64(0))
		}
	}
}
