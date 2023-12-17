package cache

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/maps"
)

var (
	dur = time.Second
)

func TestSet(t *testing.T) {
	assert := assert.New(t)

	m := New(64)
	m2 := map[string][]byte{}

	// Set fake datas.
	for i := 0; i < 10000; i++ {
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
	}

	// Remove datas.
	for k := range m2 {
		assert.True(m.Delete(k))
		assert.False(m.Delete(k))
		assert.False(m.Delete("none"))
	}
}

func TestRehash(t *testing.T) {
	assert := assert.New(t)

	m := New(1)
	for i := 0; i < 5; i++ {
		m.Set(strconv.Itoa(i), []byte{','})
	}

	assert.Equal(m.buckets[0].data, []byte{
		'0', ',', '1', ',', '2', ',', '3', ',', '4', ',',
	})

	// remove 3
	m.Delete(strconv.Itoa(3))

	m.buckets[0].rehash = true
	m.buckets[0].initRehashBucket()

	m.Set(strconv.Itoa(9), []byte{','})

	values := strings.Split(string(m.buckets[0].data), ",")
	values = values[:len(values)-1]

	// should be [0, 1, 2, 4, 9]
	assert.ElementsMatch(values, []string{"0", "1", "2", "4", "9"})
}

func TestExpired(t *testing.T) {
	assert := assert.New(t)

	m := New(64)
	m2 := map[string][]byte{}

	for i := 0; i < 1000; i++ {
		key := "key" + strconv.Itoa(i)
		m.SetEx(key, []byte(key), dur*999)
		m2[key] = []byte(key)
	}

	// SetEx
	for i := 1000; i < 2000; i++ {
		key := "exp" + strconv.Itoa(i)
		m.SetEx(key, []byte(key), dur)
	}

	// Wait to expire.
	time.Sleep(dur * 2)

	// Scan
	count := 0
	m.Scan(func(s []byte, b []byte, i int64) bool {
		assert.True(strings.HasPrefix(string(s), "key"))
		count++
		return false
	})

	assert.Equal(1000, count)

	// Keys
	assert.ElementsMatch(m.Keys(), maps.Keys(m2))
}

func TestStat(t *testing.T) {
	assert := assert.New(t)

	m := New(1)

	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("%03d", i)
		m.Set(key, []byte(key))
	}

	stat := m.Stat()
	assert.Equal(uint64(100), stat.Len)
	assert.Equal(uint64(3*100+3*100), stat.BytesAlloc)
	assert.Equal(uint64(3*100+3*100), stat.BytesInused)
	assert.Equal(uint64(0), stat.MigrateTimes)
	assert.Equal(uint64(0), stat.EvictCount)
	assert.Equal(uint64(98*3), stat.ProbeCount)
	assert.Equal(float64(100), stat.ExpRate())
	assert.Equal(float64(0), stat.EvictRate())

	// Delete
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("%03d", i)
		assert.True(m.Delete(key))
	}

	stat = m.Stat()
	assert.Equal(uint64(90), stat.Len)
	assert.Equal(uint64(3*100+3*100), stat.BytesAlloc)
	assert.Equal(uint64(3*90+3*90), stat.BytesInused)
	assert.Equal(uint64(0), stat.MigrateTimes)
	assert.Equal(uint64(0), stat.EvictCount)
	assert.Equal(uint64(10*3+98*3), stat.ProbeCount)
	assert.Equal(float64(90), stat.ExpRate())
	assert.Equal(float64(0), stat.EvictRate())

	// Reuse
	m.Set("000", []byte("000"))

	stat = m.Stat()
	assert.Equal(uint64(91), stat.Len)
	assert.Equal(uint64(3*100+3*100), stat.BytesAlloc)
	assert.Equal(uint64(3*91+3*91), stat.BytesInused)
	assert.Equal(uint64(0), stat.MigrateTimes)
	assert.Equal(uint64(0), stat.EvictCount)
	assert.Equal(uint64(10*3+99*3), stat.ProbeCount)
	assert.Equal(float64(91), stat.ExpRate())
	assert.Equal(float64(0), stat.EvictRate())
}

func TestMigrate(t *testing.T) {
	assert := assert.New(t)

	m := New(1)

	for i := 0; i < 1000; i++ {
		k1 := "key" + strconv.Itoa(i)
		m.Set(k1, []byte(k1))

		k2 := "exp" + strconv.Itoa(i)
		m.SetEx(k2, []byte(k2), dur)
	}

	time.Sleep(dur * 2)

	testCheck := func() {
		for i := 0; i < 1000; i++ {
			key := "key" + strconv.Itoa(i)
			// Get
			v, ts, ok := m.Get(key)
			assert.Equal([]byte(key), v)
			assert.True(ok)
			assert.Equal(ts, int64(0))
			// Delete
			assert.False(m.Delete("none"))
		}

		for i := 0; i < 1000; i++ {
			key := "exp" + strconv.Itoa(i)
			// Get
			v, ts, ok := m.Get(key)
			assert.Nil(v)
			assert.False(ok)
			assert.Equal(ts, int64(0))
			// Delete
			assert.False(m.Delete("none"))
		}
	}

	// Migrate.
	m.buckets[0].migrate()
	for i := 0; i < 10; i++ {
		testCheck()
		m.buckets[0].migrate()
	}

	// Check stats.
	stat := m.Stat()
	assert.Equal(1000, int(stat.Len))
	assert.Greater(int(stat.MigrateTimes), 0)
}

func TestEvict(t *testing.T) {
	assert := assert.New(t)
	m := New(1)

	// init
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("%04d", i)
		m.SetEx(key, []byte(key), time.Millisecond)
	}
	m.buckets[0].migrate()

	time.Sleep(time.Millisecond * 2)

	for i := 1000; i < 2000; i++ {
		key := fmt.Sprintf("%04d", i)
		m.SetEx(key, []byte(key), time.Millisecond)
		// if rehashing
		m.Delete(key)
	}

	stat := m.Stat()
	assert.Greater(stat.MigrateTimes, uint64(0))
}

func TestReuse(t *testing.T) {
	assert := assert.New(t)
	m := New(1)
	b := m.buckets[0]

	m.Set("foo", []byte("bar"))

	assert.Equal(b.data[0:3], []byte("foo"))
	assert.Equal(b.data[3:6], []byte("bar"))

	m.SetEx("exp", []byte("abcd"), dur)
	assert.Equal(b.data[6:9], []byte("exp"))
	assert.Equal(b.data[9:13], []byte("abcd"))

	time.Sleep(dur * 2)

	m.Set("trig", []byte("123")) // "trig" will replace positon of "exp".

	assert.Equal(b.data[6:10], []byte("trig"))
	assert.Equal(b.data[10:13], []byte("123"))

	// stat
	stat := m.Stat()
	assert.Equal(int(stat.BytesAlloc), 13)
	assert.Equal(int(stat.BytesInused), 13)

	m.Set("res", []byte("type"))

	// delete
	ok := m.Delete("foo")
	assert.True(ok)

	stat = m.Stat()
	assert.Equal(int(stat.BytesAlloc), 20)
	assert.Equal(int(stat.BytesInused), 14)
	_ = stat.EvictRate()

	// reuse max space.
	m = New(1)

	for i := 0; i < reuseSpace; i++ {
		m.SetEx("res"+strconv.Itoa(i), []byte("type"), dur)
	}
	time.Sleep(dur * 2)

	m.Set("trig", []byte("1"))

	stat = m.Stat()
	assert.Equal(int(stat.BytesAlloc), (4+4)*8)
	assert.Equal(int(stat.BytesInused), 5)

	m.Set("reuse1", []byte("xx"))

	stat = m.Stat()
	assert.Equal(int(stat.BytesAlloc), (4+4)*8)
	assert.Equal(int(stat.BytesInused), 13)

	m.Set("reuse2", []byte("xx"))

	stat = m.Stat()
	assert.Equal(int(stat.BytesAlloc), (4+4)*8)
	assert.Equal(int(stat.BytesInused), 21)
}

func TestDuplicateScan(t *testing.T) {
	assert := assert.New(t)
	m := New(1)

	for i := 0; i < 10000; i++ {
		key := "key" + strconv.Itoa(i)
		m.Set(key, []byte(key))
	}

	m.buckets[0].migrate()

	for i := 0; i < 10000; i += 500 {
		key := "key" + strconv.Itoa(i)
		m.Set(key, []byte("new_"+key))
	}

	m.Scan(func(key, value []byte, ttl int64) bool {
		keyInt := strings.TrimPrefix(string(key), "key")
		num, _ := strconv.ParseInt(keyInt, 10, 64)

		if num%500 == 0 {
			assert.Equal("new_"+string(key), string(value))
		} else {
			assert.Equal(string(key), string(value))
		}
		return false
	})
}
