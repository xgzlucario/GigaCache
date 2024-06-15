package cache

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zeebo/xxh3"
)

func testSetAndGet(assert *assert.Assertions, options Options) {
	b := newBucket(options)
	for i := 0; i < 100; i++ {
		kstr := fmt.Sprintf("%08d", i)
		key := xxh3.HashString128(kstr)
		b.set(key, []byte(kstr), []byte(kstr), 0)
	}

	for i := 0; i < 100; i++ {
		kstr := fmt.Sprintf("%08d", i)
		key := xxh3.HashString128(kstr)
		val, _, ok := b.get(key)
		assert.Equal(kstr, string(val))
		assert.True(ok)
	}
}

func TestBucket(t *testing.T) {
	assert := assert.New(t)

	options := DefaultOptions
	testSetAndGet(assert, options)

	options.ConcurrencySafe = false
	testSetAndGet(assert, options)

	options.ShardCount = 1
	testSetAndGet(assert, options)
}
