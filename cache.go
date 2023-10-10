package cache

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"slices"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/exp/rand"

	"github.com/tidwall/hashmap"
	"github.com/zeebo/xxh3"
)

const (
	noTTL = 0

	// for ttl
	ttlBytes  = 8
	timeCarry = 1e9

	defaultShardsCount = 1024
	bufferSize         = 1024

	// eliminate probing
	probeInterval = 3
	probeCount    = 100
	probeSpace    = 3

	// migrateThres defines the conditions necessary to trigger a migrate operation.
	// Ratio recommended between 0.6 and 0.7, Delta recommended 128, see bench data for details.
	migrateThresRatio = 0.6
	migrateThresDelta = 128

	maxFailCount = 5
)

var (
	// When using LittleEndian, byte slices can be converted to uint64 unsafely.
	order = binary.LittleEndian

	// Reuse buffer to reduce memory allocation.
	bpool = NewBytePoolCap(defaultShardsCount, 0, bufferSize)

	// rand source
	source = rand.NewSource(uint64(time.Now().UnixNano()))
)

// GigaCache return a new GigaCache instance.
type GigaCache[K comparable] struct {
	kstr    bool
	ksize   int
	mask    uint64
	buckets []*bucket[K]
}

// bucket
type bucket[K comparable] struct {
	idx    *hashmap.Map[K, Idx]
	alloc  int64
	mtimes int64
	bytes  []byte
	items  []*item
	sync.RWMutex
}

// item
type item struct {
	V any
	T int64
}

// New returns new GigaCache instance.
func New[K comparable](shard ...int) *GigaCache[K] {
	num := defaultShardsCount
	if len(shard) > 0 {
		num = shard[0]
	}

	cache := &GigaCache[K]{
		mask:    uint64(num - 1),
		buckets: make([]*bucket[K], num),
	}

	var k K
	switch any(k).(type) {
	case string:
		cache.kstr = true
	default:
		cache.ksize = int(unsafe.Sizeof(k))
	}

	for i := range cache.buckets {
		cache.buckets[i] = &bucket[K]{
			idx:   hashmap.New[K, Idx](0),
			bytes: bpool.Get(),
			items: make([]*item, 0),
		}
	}

	return cache
}

// Hash is the real hash function.
func (c *GigaCache[K]) hash(key K) uint64 {
	var strKey string
	if c.kstr {
		strKey = *(*string)(unsafe.Pointer(&key))
	} else {
		strKey = *(*string)(unsafe.Pointer(&struct {
			data unsafe.Pointer
			len  int
		}{unsafe.Pointer(&key), c.ksize}))
	}

	return xxh3.HashString(strKey) & c.mask
}

// getShard returns the bucket of the key.
func (c *GigaCache[K]) getShard(key K) *bucket[K] {
	return c.buckets[c.hash(key)]
}

// get returns nocopy value.
func (b *bucket[K]) get(idx Idx, nocopy ...bool) (any, int64, bool) {
	if idx.IsAny() {
		n := b.items[idx.start()]

		if idx.hasTTL() {
			if n.T > clock {
				return n.V, n.T, true
			}
			return nil, 0, false
		}
		return n.V, noTTL, true

	} else {
		start := idx.start()
		end := start + idx.offset()

		if idx.hasTTL() {
			ttl := parseTTL(b.bytes[end:])
			if ttl < clock {
				return nil, 0, false
			}

			// return
			if nocopy != nil && nocopy[0] {
				return b.bytes[start:end], ttl, true
			}
			return slices.Clone(b.bytes[start:end]), ttl, true
		}

		// return
		if nocopy != nil && nocopy[0] {
			return b.bytes[start:end], noTTL, true
		}
		return slices.Clone(b.bytes[start:end]), noTTL, true
	}
}

// Get returns value with expiration time by the key.
func (c *GigaCache[K]) Get(key K) (any, int64, bool) {
	b := c.getShard(key)
	b.RLock()
	defer b.RUnlock()

	if idx, ok := b.idx.Get(key); ok {
		return b.get(idx)
	}

	return nil, 0, false
}

// RandomGet returns a random unexpired key-value pair with ttl.
func (c *GigaCache[K]) RandomGet() (key K, val any, ts int64, ok bool) {
	rdm := source.Uint64()

	for i := uint64(0); i < uint64(len(c.buckets)); i++ {
		b := c.buckets[(rdm+i)&c.mask]
		b.Lock()

		for b.idx.Len() > 0 {
			key, idx, _ := b.idx.GetPos(rdm)
			val, ts, ok = b.get(idx)
			// unexpired
			if ok {
				b.Unlock()
				return key, val, ts, ok

			} else {
				b.idx.Delete(key)
			}
		}

		b.Unlock()
	}

	return
}

// set
func (b *bucket[K]) set(key K, val any, ts int64) {
	hasTTL := (ts != noTTL)

	// if bytes
	bytes, ok := val.([]byte)
	if ok {
		b.idx.Set(key, newIdx(len(b.bytes), len(bytes), hasTTL, false))
		b.bytes = append(b.bytes, bytes...)
		if hasTTL {
			b.bytes = order.AppendUint64(b.bytes, uint64(ts))
		}
		b.alloc++

	} else {
		idx, exist := b.idx.Get(key)
		// update inplace
		if exist && idx.IsAny() {
			start := idx.start()
			b.items[start].T = ts
			b.items[start].V = val
			b.idx.Set(key, newIdx(start, 0, hasTTL, true))

		} else {
			b.idx.Set(key, newIdx(len(b.items), 0, hasTTL, true))
			b.items = append(b.items, &item{V: val, T: ts})
			b.alloc++
		}
	}
}

// SetTx store key-value pair with deadline.
func (c *GigaCache[K]) SetTx(key K, val any, ts int64) {
	b := c.getShard(key)
	b.Lock()
	b.set(key, val, ts)
	b.eliminate()
	b.Unlock()
}

// Set store key-value pair.
func (c *GigaCache[K]) Set(key K, val any) {
	c.SetTx(key, val, noTTL)
}

// SetEx store key-value pair with expired duration.
func (c *GigaCache[K]) SetEx(key K, val any, dur time.Duration) {
	c.SetTx(key, val, clock+int64(dur))
}

// Delete removes the key-value pair by the key.
func (c *GigaCache[K]) Delete(key K) bool {
	b := c.getShard(key)
	b.Lock()
	_, ok := b.idx.Delete(key)
	if ok {
		b.alloc--
	}
	b.eliminate()
	b.Unlock()

	return ok
}

// Rename
func (c *GigaCache[K]) Rename(old, new K) bool {
	oldb := c.getShard(old)
	oldb.Lock()
	defer oldb.Unlock()

	// same bucket
	if oldb == c.getShard(new) {
		idx, _ := oldb.idx.Delete(old)
		oldb.idx.Set(new, idx)
		return true
	}

	// delete from old bucket.
	idx, ok := oldb.idx.Delete(old)
	if !ok {
		return false
	}
	v, ts, ok := oldb.get(idx, true)
	if !ok {
		return false
	}

	// update new bucket.
	c.SetTx(new, v, ts)

	return true
}

// scan
func (b *bucket[K]) scan(f func(K, any, int64) bool, nocopy ...bool) {
	b.idx.Scan(func(key K, idx Idx) bool {
		val, ts, ok := b.get(idx, nocopy...)
		if ok {
			return f(key, val, ts)
		}
		return true
	})
}

// Scan walk all key-value pairs.
func (c *GigaCache[K]) Scan(f func(K, any, int64) bool) {
	for _, b := range c.buckets {
		b.RLock()
		b.scan(f)
		b.RUnlock()
	}
}

// Keys returns all keys.
func (c *GigaCache[K]) Keys() (keys []K) {
	for _, b := range c.buckets {
		b.RLock()
		if keys == nil {
			keys = make([]K, 0, len(c.buckets)*b.idx.Len())
		}
		b.scan(func(key K, _ any, _ int64) bool {
			keys = append(keys, key)
			return true
		}, true)
		b.RUnlock()
	}

	return
}

func parseTTL(b []byte) int64 {
	// check bound
	_ = b[ttlBytes-1]
	return *(*int64)(unsafe.Pointer(&b[0]))
}

// eliminate the expired key-value pairs.
func (b *bucket[K]) eliminate() {
	if b.alloc%probeInterval != 0 {
		return
	}

	if b.idx.Len() == 0 {
		return
	}

	var failCont, ttl int64
	rdm := source.Uint64()

	// probing
	for i := uint64(0); i < probeCount; i++ {
		k, idx, _ := b.idx.GetPos(rdm + i*probeSpace)

		if !idx.hasTTL() {
			goto FAILED
		}

		if idx.IsAny() {
			ttl = b.items[idx.start()].T

		} else {
			end := idx.start() + idx.offset()
			ttl = parseTTL(b.bytes[end:])
		}

		// expired
		if ttl < clock {
			b.idx.Delete(k)
			failCont = 0
			continue
		}

	FAILED:
		failCont++
		if failCont > maxFailCount {
			break
		}
	}

	// on migrate threshold
	length := float64(b.idx.Len())
	alloc := float64(b.alloc)

	rate := length / alloc
	delta := alloc - length

	if rate < migrateThresRatio && delta > migrateThresDelta {
		b.migrate()
	}
}

// Migrate call migrate force.
func (c *GigaCache[K]) Migrate() {
	for _, b := range c.buckets {
		b.Lock()
		b.migrate()
		b.Unlock()
	}
}

// migrate move valid key-value pairs to the new container to save memory.
func (b *bucket[K]) migrate() {
	newBucket := &bucket[K]{
		idx:   hashmap.New[K, Idx](b.idx.Len()),
		bytes: bpool.Get(),
		items: make([]*item, 0),
	}

	b.scan(func(key K, val any, ts int64) bool {
		newBucket.set(key, val, ts)
		return true
	}, true)

	b.bytes = b.bytes[:0]
	bpool.Put(b.bytes)

	b.bytes = newBucket.bytes
	b.items = newBucket.items
	b.idx = newBucket.idx
	b.alloc = newBucket.alloc
	b.mtimes++
}

// cacheJSON
type cacheJSON[K comparable] struct {
	K []K
	V [][]byte
	T []int64
}

// MarshalBytes
func (c *GigaCache[K]) MarshalBytes() ([]byte, error) {
	return c.MarshalBytesFunc(nil)
}

// MarshalBytesFunc serializes all key-value pairs with a value of []byte,
// and calls the callback function when value is any.
func (c *GigaCache[K]) MarshalBytesFunc(callbackAny func(K, any, int64)) ([]byte, error) {
	var data cacheJSON[K]
	gob.Register(data)

	for _, b := range c.buckets {
		b.RLock()

		// init
		if data.K == nil {
			n := len(c.buckets) * b.idx.Len()
			data.K = make([]K, 0, n)
			data.V = make([][]byte, 0, n)
			data.T = make([]int64, 0, n)
		}

		b.scan(func(k K, a any, i int64) bool {
			// if bytes
			if bytes, ok := a.([]byte); ok {
				data.K = append(data.K, k)
				data.V = append(data.V, bytes)
				data.T = append(data.T, i/timeCarry) // ns -> s

			} else if callbackAny != nil {
				callbackAny(k, a, i)
			}
			return true
		}, true)

		b.RUnlock()
	}

	// encode
	buf := bytes.NewBuffer(nil)
	gob.NewEncoder(buf).Encode(data)

	return buf.Bytes(), nil
}

// UnmarshalBytes
func (c *GigaCache[K]) UnmarshalBytes(src []byte) error {
	var data cacheJSON[K]
	gob.Register(data)

	if err := gob.NewDecoder(bytes.NewBuffer(src)).Decode(&data); err != nil {
		return err
	}

	for i, k := range data.K {
		c.SetTx(k, data.V[i], data.T[i]*timeCarry)
	}

	return nil
}

// CacheStat is the runtime statistics of Gigacache.
type CacheStat struct {
	Len          uint64
	Alloc        uint64
	LenBytes     uint64
	LenAny       uint64
	MigrateTimes uint64
}

// Stat return the runtime statistics of Gigacache.
func (c *GigaCache[K]) Stat() (s CacheStat) {
	for _, b := range c.buckets {
		b.RLock()
		s.Len += uint64(b.idx.Len())
		s.Alloc += uint64(b.alloc)
		s.LenBytes += uint64(len(b.bytes))
		s.LenAny += uint64(len(b.items))
		s.MigrateTimes += uint64(b.mtimes)
		b.RUnlock()
	}
	return
}

// ExpRate
func (s CacheStat) ExpRate() float64 {
	return float64(s.Len) / float64(s.Alloc) * 100
}
