package cache

import (
	"fmt"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func genKey(i int) []byte {
	return []byte(fmt.Sprintf("%08x", i))
}

func TestSet(t *testing.T) {
	fmt.Println("===== TestSet =====")
	assert := assert.New(t)
	const num = 10000

	opt := DefaultOption
	opt.ShardCount = 1
	m := New(opt)

	// Set
	for i := 0; i < num; i++ {
		k := genKey(i)
		m.Set(k, k)

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
		k := genKey(i)
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
		k := []byte(fmt.Sprintf("n-%08x", i))
		v, ts, ok := m.Get(k)
		assert.False(ok)
		assert.Nil(v)
		assert.Equal(int64(0), ts)
	}

	// Delete
	for i := 0; i < num; i++ {
		k := genKey(i)
		m.Delete(k)
	}

	// Check ttl
	{
		ttl := time.Now().Add(time.Second * 3).UnixNano()
		m.SetTx([]byte("a"), []byte("b"), ttl)
		v, ts, ok := m.Get([]byte("a"))
		assert.Equal(v, []byte("b"))
		assert.Equal(ts, (ttl/timeCarry)*timeCarry)
		assert.True(ok)

		time.Sleep(time.Second * 2)
		v, ts, ok = m.Get([]byte("a"))
		assert.Equal(v, []byte("b"))
		assert.Equal(ts, (ttl/timeCarry)*timeCarry)
		assert.True(ok)

		time.Sleep(time.Second * 2)
		v, ts, ok = m.Get([]byte("a"))
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
		k := genKey(i)
		if i%2 == 0 {
			m.SetEx(k, k, time.Second*60)
		} else {
			m.SetEx(k, k, time.Second)
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
		k := genKey(i)
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
		k := []byte(fmt.Sprintf("n-%08x", i))
		v, ts, ok := m.Get(k)
		assert.False(ok)
		assert.Nil(v)
		assert.Equal(int64(0), ts)
	}
}

func TestOnEvict(t *testing.T) {
	fmt.Println("===== TestOnEvict =====")
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
	for i := 0; i < 10000; i++ {
		k := []byte(strconv.Itoa(i))
		if i%2 == 1 {
			m.SetEx(k, k, time.Second)
		} else {
			m.SetEx(k, k, time.Minute)
		}
	}

	time.Sleep(time.Second * 2)

	// trigger onEvict
	for i := 0; i < 10000; i++ {
		m.Set([]byte("trig"), []byte("trig"))
	}
}

func TestSpaceCache(t *testing.T) {
	fmt.Println("===== TestSpaceCache =====")
	assert := assert.New(t)

	// the key + value size is 16.
	const size = 8 * 2

	opt := DefaultOption
	opt.ShardCount = 1
	m := New(opt)

	// Set
	for i := 0; i < 1000; i++ {
		k := genKey(i)
		m.Set(k, k)
	}
	// Delete some
	for i := 0; i < 200; i++ {
		k := genKey(i)
		m.Delete(k)
	}

	stat := m.Stat()
	assert.Equal(800, int(stat.Len))
	assert.Equal(1000*size, int(stat.Alloc))
	assert.Equal(800*size, int(stat.Inused))
	assert.Equal(0, int(stat.Reused))
	assert.Equal(0, int(stat.Evict))

	// Set in reuse space.
	for i := 1; i <= opt.SCacheSize; i++ {
		k := genKey(i)
		m.Set(k, k)

		stat := m.Stat()
		assert.Equal(800+i, int(stat.Len))
		assert.Equal(1000*size, int(stat.Alloc))
		assert.Equal((800+i)*size, int(stat.Inused))
		assert.Equal(i*size, int(stat.Reused))
		assert.Equal(0, int(stat.Evict))
	}

	// Set in alloc new space.
	k := []byte("12345678")
	m.Set(k, k)

	stat = m.Stat()
	assert.Equal(800+opt.SCacheSize+1, int(stat.Len))
	assert.Equal(1000*size+16, int(stat.Alloc))
	assert.Equal((800+opt.SCacheSize+1)*size, int(stat.Inused))
	assert.Equal(16*8, int(stat.Reused))
	assert.Equal(0, int(stat.Evict))
}

func TestMigrate(t *testing.T) {
	fmt.Println("===== TestMigrate =====")
	assert := assert.New(t)

	// the key + value size is 16.
	const size = 8 * 2

	opt := DefaultOption
	opt.ShardCount = 1
	m := New(opt)

	for i := 0; i < 1000; i++ {
		k := genKey(i)
		if i%4 == 0 {
			m.SetEx(k, k, time.Second)
		} else {
			m.Set(k, k)
		}
	}

	time.Sleep(time.Second * 2)

	// check stat before migrate.
	stat := m.Stat()
	assert.Equal(1000, int(stat.Len))
	assert.Equal(1000*size, int(stat.Alloc))
	assert.Equal(1000*size, int(stat.Inused))
	assert.Equal(0, int(stat.Evict))

	// evict some.
	for i := 0; i < 5; i++ {
		m.buckets[0].eliminate()
	}
	stat = m.Stat()
	evict := int(stat.Evict)
	assert.Equal(1000-evict, int(stat.Len))
	assert.Equal(1000*size, int(stat.Alloc))
	assert.Equal((1000-evict)*size, int(stat.Inused))
	assert.Equal(evict, int(stat.Evict))

	m.Migrate()

	// check stat after migrate.
	stat = m.Stat()
	assert.Equal(750, int(stat.Len))
	assert.Equal(750*size, int(stat.Alloc))
	assert.Equal(750*size, int(stat.Inused))
	assert.Equal(evict, int(stat.Evict))
}

func TestBufferPool(t *testing.T) {
	fmt.Println("===== TestBufferPool =====")
	assert := assert.New(t)
	bpool := NewBufferPool()

	// miss
	buf := bpool.Get(1024)
	assert.Equal(1024, cap(buf))
	assert.Equal(1024, len(buf))
	assert.Equal(int(bpool.miss.Load()), 1)
	bpool.Put(buf)

	// hit
	buf = bpool.Get(1022)
	assert.Equal(1024, cap(buf))
	assert.Equal(1022, len(buf))
	assert.Equal(int(bpool.hit.Load()), 1)

	runtime.GC()

	// miss
	buf = bpool.Get(1024)
	assert.Equal(1024, cap(buf))
	assert.Equal(1024, len(buf))
	assert.Equal(int(bpool.miss.Load()), 2)
	bpool.Put(buf)

	assert.Panics(func() {
		opt := DefaultOption
		opt.ShardCount = 0
		New(opt)
	})

	assert.Panics(func() {
		m := New(DefaultOption)
		m.Set(nil, nil)
	})
}
