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
	dur = time.Second / 10
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
		m.SetEx(key, []byte(key), dur*10)
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
	m.Scan(func(s string, b []byte, i int64) bool {
		assert.True(strings.HasPrefix(s, "key"))
		count++
		return false
	})

	assert.Equal(1000, count)

	// Keys
	assert.ElementsMatch(m.Keys(), maps.Keys(m2))
}

func TestMarshal(t *testing.T) {
	assert := assert.New(t)

	m := New(64)
	m2 := map[string][]byte{}

	for i := 0; i < 1000; i++ {
		key := "key" + strconv.Itoa(i)
		m.Set(key, []byte(key))
		m2[key] = []byte(key)
	}

	for i := 1000; i < 2000; i++ {
		key := "exp" + strconv.Itoa(i)
		m.SetEx(key, []byte(key), dur)
	}

	time.Sleep(dur * 2)

	// Marshal
	data, err := m.MarshalBinary()
	assert.Nil(err)

	// Unmarshal
	m3 := New(64)
	assert.Nil(m3.UnmarshalBinary(data))

	// Check
	for k, v := range m2 {
		val, ts, ok := m3.Get(k)
		assert.Equal(v, val)
		assert.True(ok)
		assert.Equal(ts, int64(0))
	}

	// Error
	err = m3.UnmarshalBinary([]byte("fake news"))
	assert.NotNil(err)
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
	assert.Equal(uint64(99*3), stat.ProbeCount)
	assert.Equal(float64(100), stat.ExpRate())
	assert.Equal(float64(0), stat.EvictRate())

	// Delete
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("%03d", i)
		m.Delete(key)
	}

	stat = m.Stat()
	assert.Equal(uint64(90), stat.Len)
	assert.Equal(uint64(3*100+3*100), stat.BytesAlloc)
	assert.Equal(uint64(3*90+3*90), stat.BytesInused)
	assert.Equal(uint64(0), stat.MigrateTimes)
	assert.Equal(uint64(0), stat.EvictCount)
	assert.Equal(uint64(99*3), stat.ProbeCount)
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
	assert.Equal(uint64(100*3), stat.ProbeCount)
	assert.Equal(float64(91), stat.ExpRate())
	assert.Equal(float64(0), stat.EvictRate())
}
