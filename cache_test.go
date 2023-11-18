package cache

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func equalMapAndCache(assert *assert.Assertions, m map[string][]byte, c *GigaCache) {
	for k := range m {
		ok := c.Has(k)
		assert.True(ok)
	}
}

func TestSet(t *testing.T) {
	assert := assert.New(t)

	m := New(64)
	m2 := map[string][]byte{}

	// Set fake datas.
	for i := 0; i < 10*10000; i++ {
		key := "key" + strconv.Itoa(i)
		value := []byte(key)

		m.Set(key, value)
		m2[key] = value
	}

	// Check datas.
	for k, v := range m2 {
		// Get
		val, ts, ok := m.Get(k)
		assert.Equal(v, val)
		assert.True(ok)
		assert.Equal(ts, int64(0))

		// Get none.
		val, ts, ok = m.Get("none")
		assert.Nil(val)
		assert.False(ok)
		assert.Equal(ts, int64(0))

		// Has
		assert.True(m.Has(k))

		// Has none.
		assert.False(m.Has("none"))
	}

	// Remove datas.
	for k := range m2 {
		assert.True(m.Delete(k))
		assert.False(m.Delete(k))
		assert.False(m.Delete("none"))
	}
}

func TestSetAndDelete(t *testing.T) {
	assert := assert.New(t)

	m := New(64)
	m2 := map[string][]byte{}

	// Set fake datas.
	for i := 0; i < 100*10000; i++ {
		key := "key" + strconv.Itoa(i)
		value := []byte(key)

		m.Set(key, value)
		m2[key] = value

		if i%2 == 0 {
			m.Delete(key)
			delete(m2, key)
		}
	}

	equalMapAndCache(assert, m2, m)
}
