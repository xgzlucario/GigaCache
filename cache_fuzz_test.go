package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func FuzzCacheHashConflict(f *testing.F) {
	m1 := make(map[string]string, 100*10000)

	options := DefaultOptions
	options.DisableEvict = true
	options.HashFn = func(s string) uint64 {
		return uint64(uint16(MemHash(s)))
	}
	m2 := New(options)

	f.Fuzz(func(t *testing.T, key string, val []byte) {
		assert := assert.New(t)

		// set
		m1[key] = string(val)
		m2.Set(key, val)

		// check
		var count int
		for k, v := range m1 {
			res, ts, ok := m2.Get(k)
			assert.Equal(v, string(res))
			assert.Equal(ts, int64(0))
			assert.True(ok)

			count++
			if count > 10 {
				break
			}
		}

		assert.Equal(len(m1), m2.Stat().Len)
	})
}
