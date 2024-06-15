package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func FuzzCacheHashConflict(f *testing.F) {
	m1 := make(map[string]string, 100*10000)

	options := DefaultOptions
	options.EvictInterval = -1

	m2 := New(options)

	f.Fuzz(func(t *testing.T, key string, val []byte, n byte) {
		assert := assert.New(t)

		// set
		m1[key] = string(val)
		m2.Set(key, val)

		if n%2 == 0 {
			// delete
			for k := range m1 {
				delete(m1, k)
				ok := m2.Remove(k)
				assert.True(ok)
				break
			}

		} else {
			// check length
			var count int
			for k, v := range m1 {
				res, ts, ok := m2.Get(k)
				assert.Equal(v, string(res))
				assert.Equal(ts, int64(0))
				assert.True(ok)

				count++
				if count > 100 {
					break
				}
			}
			assert.Equal(len(m1), m2.GetStats().Len)
		}
	})
}
