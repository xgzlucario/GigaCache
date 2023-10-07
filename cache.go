package cache

import (
	"encoding/binary"
	"slices"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/exp/rand"

	"github.com/bytedance/sonic"
	"github.com/tidwall/hashmap"
	"github.com/zeebo/xxh3"
)

const (
	noTTL = 0

	// for ttl
	ttlBytes = 8

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

// GigaCache
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
	anyArr []*anyItem
	sync.RWMutex
}

// anyItem
type anyItem struct {
	V any
	T int64
}

// New returns a new GigaCache instance.
func New[K comparable](shardCount ...int) *GigaCache[K] {
	var shards = defaultShardsCount
	if len(shardCount) > 0 {
		shards = shardCount[0]
	}

	cache := &GigaCache[K]{
		mask:    uint64(shards - 1),
		buckets: make([]*bucket[K], shards),
	}

	var k K
	switch ((interface{})(k)).(type) {
	case string:
		cache.kstr = true
	default:
		cache.ksize = int(unsafe.Sizeof(k))
	}

	for i := range cache.buckets {
		cache.buckets[i] = &bucket[K]{
			idx:    hashmap.New[K, Idx](0),
			bytes:  bpool.Get(),
			anyArr: make([]*anyItem, 0),
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
		n := b.anyArr[idx.start()]

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
			b.anyArr[start].T = ts
			b.anyArr[start].V = val
			b.idx.Set(key, newIdx(start, 0, hasTTL, true))

		} else {
			b.idx.Set(key, newIdx(len(b.anyArr), 0, hasTTL, true))
			b.anyArr = append(b.anyArr, &anyItem{V: val, T: ts})
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
	b.eliminate()
	b.Unlock()

	return ok
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
			ttl = b.anyArr[idx.start()].T

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
		idx:    hashmap.New[K, Idx](b.idx.Len()),
		bytes:  bpool.Get(),
		anyArr: make([]*anyItem, 0),
		mtimes: b.mtimes + 1,
	}

	b.scan(func(key K, val any, ts int64) bool {
		newBucket.set(key, val, ts)
		return true
	}, true)

	b.bytes = b.bytes[:0]
	bpool.Put(b.bytes)

	b.bytes = newBucket.bytes
	b.anyArr = newBucket.anyArr
	b.idx = newBucket.idx
	b.mtimes = newBucket.mtimes
	b.alloc = newBucket.alloc
}

type bucketJSON[K comparable] struct {
	C    int64
	K    []K
	I, B []byte
}

// MarshalBytes only marshal bytes data ignore any data.
func (c *GigaCache[K]) MarshalBytes() ([]byte, error) {
	buckets := make([]*bucketJSON[K], 0, len(c.buckets))

	for _, b := range c.buckets {
		b.RLock()
		defer b.RUnlock()

		k := make([]K, 0, b.idx.Len())
		i := make([]byte, 0, b.idx.Len())

		b.idx.Scan(func(key K, idx Idx) bool {
			if !idx.IsAny() {
				k = append(k, key)
				i = order.AppendUint64(i, uint64(idx))
			}
			return true
		})

		buckets = append(buckets, &bucketJSON[K]{
			b.alloc, k, i, b.bytes,
		})
	}

	return sonic.Marshal(buckets)
}

// UnmarshalBytes
func (c *GigaCache[K]) UnmarshalBytes(src []byte) error {
	var buckets []*bucketJSON[K]

	if err := sonic.Unmarshal(src, &buckets); err != nil {
		return err
	}

	c.buckets = make([]*bucket[K], 0, len(buckets))
	for _, b := range buckets {
		bc := &bucket[K]{
			alloc:  b.C,
			idx:    hashmap.New[K, Idx](len(b.K)),
			bytes:  b.B,
			anyArr: make([]*anyItem, 0),
		}

		// set key
		for i, k := range b.K {
			idx := order.Uint64(b.I[i*8 : (i+1)*8])
			bc.idx.Set(k, Idx(idx))
		}

		c.buckets = append(c.buckets, bc)
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
		s.LenAny += uint64(len(b.anyArr))
		s.MigrateTimes += uint64(b.mtimes)
		b.RUnlock()
	}
	return
}

// ExpRate
func (s CacheStat) ExpRate() float64 {
	return float64(s.Len) / float64(s.Alloc) * 100
}
