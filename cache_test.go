package cache

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSet(t *testing.T) {
	fmt.Println("===== TestSet =====")
	assert := assert.New(t)
	const num = 10000

	opt := DefaultOption
	opt.ShardCount = 1
	m := New(opt)

	// Set
	for i := 0; i < num; i++ {
		k := fmt.Sprintf("%08x", i)
		m.Set(k, []byte(k))

		// check stat
		stat := m.Stat()
		assert.Equal(i+1, int(stat.Len))
		assert.Equal(16*(i+1), int(stat.Alloc))
		assert.Equal(16*(i+1), int(stat.Inused))
		assert.Equal(0, int(stat.Evict))
		assert.GreaterOrEqual(int(stat.Probe), 0)
		assert.Equal(0, int(stat.Migrates))
		assert.Equal(float64(100), stat.ExpRate())
		if i > 0 {
			assert.Equal(float64(0), stat.EvictRate())
		}
	}

	// Get
	for i := 0; i < num; i++ {
		k := fmt.Sprintf("%08x", i)
		v, ts, ok := m.Get(k)
		assert.True(ok)
		assert.Equal([]byte(k), v)
		assert.Equal(int64(0), ts)
	}

	// Scan
	count := 0
	m.Scan(func(key, value []byte, ttl int64) (stop bool) {
		count++
		assert.Equal(key, value)
		assert.Equal(int64(0), ttl)
		return false
	})
	assert.Equal(count, num)

	// Get none exist
	for i := 0; i < num; i++ {
		k := fmt.Sprintf("n-%08x", i)
		v, ts, ok := m.Get(k)
		assert.False(ok)
		assert.Nil(v)
		assert.Equal(int64(0), ts)
	}

	// Delete
	for i := 0; i < num; i++ {
		k := fmt.Sprintf("%08x", i)
		m.Delete(k)
	}

	// Check ttl
	{
		ttl := time.Now().Add(time.Second * 3).UnixNano()
		m.SetTx("a", []byte("b"), ttl)
		v, ts, ok := m.Get("a")
		assert.Equal(v, []byte("b"))
		assert.Equal(ts, (ttl/timeCarry)*timeCarry)
		assert.True(ok)

		time.Sleep(time.Second * 2)
		v, ts, ok = m.Get("a")
		assert.Equal(v, []byte("b"))
		assert.Equal(ts, (ttl/timeCarry)*timeCarry)
		assert.True(ok)

		time.Sleep(time.Second * 2)
		v, ts, ok = m.Get("a")
		assert.Nil(v)
		assert.Equal(ts, int64(0))
		assert.False(ok)
	}
}

func TestSetExpired(t *testing.T) {
	fmt.Println("===== TestSetExpired =====")
	assert := assert.New(t)
	const num = 10000
	startUnix := time.Now().Add(time.Second*60).Unix() * timeCarry

	opt := DefaultOption
	opt.ShardCount = 1
	m := New(opt)

	// Set
	for i := 0; i < num; i++ {
		// odd number will be expired.
		k := fmt.Sprintf("%08x", i)
		if i%2 == 0 {
			m.SetEx(k, []byte(k), time.Second*60)
		} else {
			m.SetEx(k, []byte(k), time.Second)
		}
	}

	time.Sleep(time.Second * 2)

	// Scan
	count := 0
	m.Scan(func(key, value []byte, ttl int64) (stop bool) {
		count++
		assert.Equal(key, value)
		return false
	})
	assert.Equal(count, num/2)

	// Get
	for i := 0; i < num; i++ {
		k := fmt.Sprintf("%08x", i)
		v, ts, ok := m.Get(k)
		if i%2 == 0 {
			assert.True(ok)
			assert.Equal([]byte(k), v)
			assert.GreaterOrEqual(ts, startUnix)
		} else {
			assert.False(ok)
			assert.Nil(v)
			assert.Equal(int64(0), ts)
		}
	}

	// Get none exist
	for i := 0; i < num; i++ {
		k := fmt.Sprintf("n-%08x", i)
		v, ts, ok := m.Get(k)
		assert.False(ok)
		assert.Nil(v)
		assert.Equal(int64(0), ts)
	}
}

func TestOnEvict(t *testing.T) {
	fmt.Println("===== TestSpaceCache =====")
	assert := assert.New(t)

	opt := DefaultOption
	opt.ShardCount = 1
	opt.OnEvict = func(key, value []byte) {
		keyNum, _ := strconv.ParseInt(string(key), 10, 0)
		assert.Equal(keyNum%2, int64(1))
		assert.Equal(key, value)
	}
	m := New(opt)

	// SetEx
	for i := 0; i < 1000; i++ {
		k := strconv.Itoa(i)
		if i%2 == 1 {
			m.SetEx(k, []byte(k), time.Second)
		} else {
			m.SetEx(k, []byte(k), time.Minute)
		}
	}

	time.Sleep(time.Second * 2)

	// trigger onEvict
	for i := 0; i < 1000; i++ {
		m.Set("trig", []byte("trig"))
	}
}

func TestSpaceCache(t *testing.T) {
	fmt.Println("===== TestSpaceCache =====")
	assert := assert.New(t)

	opt := DefaultOption
	opt.ShardCount = 1
	m := New(opt)

	// Set
	for i := 0; i < 1000; i++ {
		k := fmt.Sprintf("%04x", i)
		m.Set(k, []byte(k))
	}

	// Delete some
	for i := 0; i < 200; i++ {
		k := fmt.Sprintf("%04x", i)
		m.Delete(k)
	}

	stat := m.Stat()
	assert.Equal(800, int(stat.Len))
	assert.Equal(1000*8, int(stat.Alloc))
	assert.Equal(800*8, int(stat.Inused))
	assert.Equal(0, int(stat.Reused))
	assert.Equal(0, int(stat.Evict))

	// Set in reuse space.
	for i := 1; i <= opt.SCacheSize; i++ {
		k := fmt.Sprintf("%04x", i)
		m.Set(k, []byte(k))

		stat := m.Stat()
		assert.Equal(800+i, int(stat.Len))
		assert.Equal(1000*8, int(stat.Alloc))
		assert.Equal((800+i)*8, int(stat.Inused))
		assert.Equal(i*8, int(stat.Reused))
		assert.Equal(0, int(stat.Evict))
	}

	// Set in alloc new space.
	k := "abcd"
	m.Set(k, []byte(k))

	stat = m.Stat()
	assert.Equal(800+opt.SCacheSize+1, int(stat.Len))
	assert.Equal(1000*8+8, int(stat.Alloc))
	assert.Equal((800+opt.SCacheSize+1)*8, int(stat.Inused))
	assert.Equal(8*8, int(stat.Reused))
	assert.Equal(0, int(stat.Evict))
}

func TestMigrate(t *testing.T) {
	fmt.Println("===== TestMigrate =====")
	assert := assert.New(t)

	opt := DefaultOption
	opt.ShardCount = 1
	m := New(opt)

	for i := 0; i < 1000; i++ {
		k := fmt.Sprintf("%04x", i)
		if i%4 == 0 {
			m.SetEx(k, []byte(k), time.Second)
		} else {
			m.Set(k, []byte(k))
		}
	}

	time.Sleep(time.Second * 2)

	// check stat before migrate.
	stat := m.Stat()
	assert.Equal(1000, int(stat.Len))
	assert.Equal(1000*8, int(stat.Alloc))
	assert.Equal(1000*8, int(stat.Inused))
	assert.Equal(0, int(stat.Evict))

	// evict some.
	for i := 0; i < 5; i++ {
		m.buckets[0].eliminate()
	}
	stat = m.Stat()
	evict := int(stat.Evict)
	assert.Equal(1000-evict, int(stat.Len))
	assert.Equal(1000*8, int(stat.Alloc))
	assert.Equal((1000-evict)*8, int(stat.Inused))
	assert.Equal(evict, int(stat.Evict))

	m.Migrate()

	// check stat after migrate.
	stat = m.Stat()
	assert.Equal(750, int(stat.Len))
	assert.Equal(750*8, int(stat.Alloc))
	assert.Equal(750*8, int(stat.Inused))
	assert.Equal(evict, int(stat.Evict))
}

func TestBufferPool(t *testing.T) {
	fmt.Println("===== TestBufferPool =====")
	assert := assert.New(t)

	bpool := NewBufferPool(128)
	{
		bpool.Put(make([]byte, 129))
		bpool.Put(make([]byte, 130))
		bpool.Get(131)
		bpool.Get(132)
	}
	for i := 0; i < 1024; i++ {
		buf := bpool.Get(i)
		assert.Equal(len(buf), i)
		assert.GreaterOrEqual(cap(buf), i)
		bpool.Put(buf)
	}
	for i := 1024; i > 0; i-- {
		buf := bpool.Get(i)
		assert.Equal(len(buf), i)
		assert.GreaterOrEqual(cap(buf), i)
		bpool.Put(buf)
	}

	fmt.Println(bpool)

	assert.Panics(func() {
		NewBufferPool(-1)
	})

	assert.Panics(func() {
		opt := DefaultOption
		opt.ShardCount = 0
		New(opt)
	})
}
