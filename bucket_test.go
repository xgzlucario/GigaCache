package cache

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var (
	nilBytes []byte
)

func getBucket() *bucket {
	options := DefaultOptions
	options.EvictInterval = 1
	m := newBucket(options)

	for i := 0; i < 100; i++ {
		k := fmt.Sprintf("key%d", i)
		key := Key(i / 10)
		m.set(key, []byte(k), []byte(k), 0)
	}
	return m
}

func TestBucket(t *testing.T) {
	assert := assert.New(t)

	m := getBucket()
	assert.Equal(10, m.index.Len())
	assert.Equal(90, m.conflict.Len())

	m.eliminate()
	m.migrate()
	assert.Equal(10, m.index.Len())
	assert.Equal(90, m.conflict.Len())

	var count int
	m.scan(func(key, val []byte, ttl int64) bool {
		assert.Equal(key, val)
		assert.Equal(ttl, int64(0))
		count++
		return true
	})
	assert.Equal(100, count)
}

func TestBucketExpired(t *testing.T) {
	assert := assert.New(t)

	m := getBucket()
	ttl := time.Now().Add(time.Second).UnixNano()
	for i := 0; i < 100; i++ {
		k := fmt.Sprintf("key%d", i)
		key := Key(i / 10)
		// set
		m.set(key, []byte(k), []byte(k), ttl)
		// get
		val, ts, ok := m.get(k, key)
		assert.True(ok)
		assert.Equal(val, []byte(k))
		assert.Equal(ts, ttl/timeCarry*timeCarry)
	}

	time.Sleep(time.Second * 2)
	assert.Equal(90, m.conflict.Len())
	assert.Equal(10, m.index.Len())
	m.eliminate()
	assert.Equal(0, m.conflict.Len())
	assert.Equal(0, m.index.Len())
}

func TestBucketMigrate(t *testing.T) {
	assert := assert.New(t)

	m := getBucket()
	ttl := time.Now().Add(time.Second).UnixNano()
	for i := 0; i < 100; i++ {
		k := fmt.Sprintf("key%d", i)
		key := Key(i / 10)
		// setTTL
		ok := m.setTTL(key, k, ttl)
		assert.True(ok)
		// get
		val, ts, ok := m.get(k, key)
		assert.True(ok)
		assert.Equal(val, []byte(k))
		assert.Equal(ts, ttl/timeCarry*timeCarry)
	}

	time.Sleep(time.Second * 2)
	assert.Equal(90, m.conflict.Len())
	assert.Equal(10, m.index.Len())
	m.migrate()
	assert.Equal(0, m.conflict.Len())
	assert.Equal(0, m.index.Len())
}

func TestBucketRemove(t *testing.T) {
	assert := assert.New(t)

	m := getBucket()
	for i := 0; i < 100; i++ {
		k := fmt.Sprintf("key%d", i)
		key := Key(i / 10)
		// remove
		m.remove(key, k)
		// get
		val, ts, ok := m.get(k, key)
		assert.False(ok)
		assert.Equal(val, nilBytes)
		assert.Equal(ts, int64(0))
	}
	assert.Equal(0, m.conflict.Len())
	assert.Equal(0, m.index.Len())
}
