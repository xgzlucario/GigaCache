package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var (
	nilBytes []byte
)

func getBucket(options ...Options) *bucket {
	var opt Options
	if len(options) > 0 {
		opt = options[0]
	} else {
		opt = DefaultOptions
		opt.EvictInterval = 1
	}
	m := newBucket(opt)

	for i := 0; i < 100; i++ {
		k, v := genKV(i)
		key := Key(i / 10)
		m.set(key, []byte(k), v, 0)
	}
	return m
}

func TestBucket(t *testing.T) {
	assert := assert.New(t)

	testHashFn := func(s string) uint64 {
		return 0
	}

	for i, hashFn := range []HashFn{MemHash, testHashFn} {
		opt := DefaultOptions
		opt.HashFn = hashFn

		m := getBucket(opt)
		scanCheck := func() {
			var count int
			m.scan(func(key, val []byte, ttl int64) bool {
				count++
				return true
			})
			assert.Equal(100, count)
		}

		assert.Equal(10, len(m.index))
		assert.Equal(90, len(m.conflictMap))

		m.evictExpiredItems()
		scanCheck()
		m.migrate()

		if i == 0 {
			assert.Equal(100, len(m.index)) // migrate use memhash and migrate all keys to index.
			assert.Equal(0, len(m.conflictMap))
		} else {
			assert.Equal(10, len(m.index))
			assert.Equal(90, len(m.conflictMap))
		}
		scanCheck()
	}
}

func TestBucketExpired(t *testing.T) {
	assert := assert.New(t)

	t.Run("1", func(t *testing.T) {
		m := getBucket()
		ttl := time.Now().Add(time.Second).UnixNano()
		for i := 0; i < 100; i++ {
			k, v := genKV(i)
			key := Key(i / 10)
			// set
			m.set(key, []byte(k), v, ttl)
			// get
			val, ts, ok := m.get(k, key)
			assert.True(ok)
			assert.Equal(val, v)
			assert.Equal(ts, ttl/timeCarry*timeCarry)
		}

		assert.Equal(90, len(m.conflictMap))
		assert.Equal(10, len(m.index))

		// expired
		time.Sleep(time.Second * 2)

		var count int
		m.scan(func(key, val []byte, ttl int64) (next bool) {
			count++
			return true
		})
		assert.Equal(count, 0)

		m.evictExpiredItems()
		assert.Equal(0, len(m.conflictMap))
		assert.Equal(0, len(m.index))
	})
}

func TestBucketMigrate(t *testing.T) {
	assert := assert.New(t)

	m := getBucket()
	ttl := time.Now().Add(time.Second).UnixNano()
	for i := 0; i < 100; i++ {
		k, v := genKV(i)
		key := Key(i / 10)
		// setTTL
		ok := m.setTTL(key, k, ttl)
		assert.True(ok)
		// get
		val, ts, ok := m.get(k, key)
		assert.True(ok)
		assert.Equal(val, v)
		assert.Equal(ts, ttl/timeCarry*timeCarry)
	}

	time.Sleep(time.Second * 2)
	assert.Equal(90, len(m.conflictMap))
	assert.Equal(10, len(m.index))
	m.migrate()
	assert.Equal(0, len(m.conflictMap))
	assert.Equal(0, len(m.index))
}

func TestBucketRemove(t *testing.T) {
	assert := assert.New(t)

	t.Run("remove", func(t *testing.T) {
		m := getBucket()
		for i := 0; i < 100; i++ {
			k, _ := genKV(i)
			key := Key(i / 10)
			// remove
			m.remove(key, k)
			// get
			val, ts, ok := m.get(k, key)
			assert.False(ok)
			assert.Equal(val, nilBytes)
			assert.Equal(ts, int64(0))
		}
		assert.Equal(0, len(m.conflictMap))
		assert.Equal(0, len(m.index))
	})

	t.Run("remove-ttl", func(t *testing.T) {
		options := DefaultOptions
		options.EvictInterval = 1
		m := newBucket(options)

		ts1 := time.Now().Add(time.Hour).UnixNano()
		for i := 0; i < 100; i++ {
			k, v := genKV(i)
			key := Key(i / 10)
			m.set(key, []byte(k), v, ts1)
		}
		ts2 := time.Now().UnixNano()
		for i := 100; i < 200; i++ {
			k, v := genKV(i)
			key := Key(i / 10)
			m.set(key, []byte(k), v, ts2)
		}

		time.Sleep(time.Second)

		// remove
		for i := 0; i < 100; i++ {
			k, _ := genKV(i)
			key := Key(i / 10)
			ok := m.remove(key, k)
			assert.True(ok)
		}
		for i := 100; i < 200; i++ {
			k, _ := genKV(i)
			key := Key(i / 10)
			ok := m.remove(key, k)
			assert.False(ok) // false because of expired.
		}
	})
}

func TestBucketScan(t *testing.T) {
	assert := assert.New(t)
	m := getBucket()

	t.Run("scan", func(t *testing.T) {
		var count int
		m.scan(func(key, val []byte, _ int64) bool {
			assert.Equal(key, val)
			count++
			return true
		})
		assert.Equal(100, count)
	})

	t.Run("scan-break", func(t *testing.T) {
		var count int
		m.scan(func(_, _ []byte, _ int64) bool {
			count++
			return count < 50
		})
		assert.Equal(50, count)
	})
}
