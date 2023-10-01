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

	bufferSize         = 1024
	defaultShardsCount = 1024

	// eliminate probing
	probeInterval = 3
	probeCount    = 100
	probeSpace    = 3

	// config for rehash
	rehashCount = 100

	// migrateThreshold Indicates how many effective bytes trigger the migrate operation.
	// Recommended between 0.6 and 0.7, see bench data for details.
	migrateThreshold = 0.6

	maxFailCount = 5
)

var (
	// When using LittleEndian, byte slices can be converted to uint64 unsafely.
	order = binary.LittleEndian

	// Reuse buffer to reduce memory allocation.
	bpool = NewBytePoolCap(defaultShardsCount, 0, bufferSize)
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
	rehashing  bool
	id         uint32
	allocTimes int64
	mtimes     int64
	idx        *hashmap.Map[K, Idx]
	bytes      []byte
	anyArr     []*anyItem
	nb         *bucket[K] // new bucket for rehash.
	sync.RWMutex
}

// anyItem
type anyItem struct {
	V any
	T int64
}

// New returns a new GigaCache instance.
func New[K comparable](count ...int) *GigaCache[K] {
	var shards = defaultShardsCount
	if len(count) > 0 {
		shards = count[0]
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
			id:     uint32(i),
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

// getNoCopy returns NoCopy value.
func (b *bucket[K]) getNoCopy(idx Idx) (any, int64, bool) {
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
			return b.bytes[start:end], ttl, true
		}

		return b.bytes[start:end], noTTL, true
	}
}

// get returns value.
func (b *bucket[K]) get(idx Idx) (any, int64, bool) {
	val, ts, ok := b.getNoCopy(idx)
	if ok {
		if idx.IsAny() {
			return val, ts, true
		}
		return slices.Clone(val.([]byte)), ts, true
	}
	return nil, 0, false
}

// Get return bytes value by key.
func (c *GigaCache[K]) Get(key K) (any, int64, bool) {
	b := c.getShard(key)
	b.RLock()
	defer b.RUnlock()

	if idx, ok := b.idx.Get(key); ok {
		return b.get(idx)
	}

	return nil, 0, false
}

// set
func (b *bucket[K]) set(key K, val any, ts int64) {
	// rehashing
	if b.rehashing {
		b.nb.set(key, val, ts)
		return
	}

	hasTTL := (ts != noTTL)

	// if bytes
	bytes, ok := val.([]byte)
	if ok {
		b.idx.Set(key, newIdx(len(b.bytes), len(bytes), hasTTL, false))
		b.bytes = append(b.bytes, bytes...)
		if hasTTL {
			b.bytes = order.AppendUint64(b.bytes, uint64(ts))
		}
		b.allocTimes++

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
			b.allocTimes++
		}
	}
}

// SetTx
func (c *GigaCache[K]) SetTx(key K, val any, ts int64) {
	b := c.getShard(key)
	b.Lock()
	b.set(key, val, ts)
	b.eliminate()
	b.Unlock()
}

// Set
func (c *GigaCache[K]) Set(key K, val any) {
	c.SetTx(key, val, noTTL)
}

// SetEx
func (c *GigaCache[K]) SetEx(key K, val any, dur time.Duration) {
	c.SetTx(key, val, clock+int64(dur))
}

// Delete
func (c *GigaCache[K]) Delete(key K) (ok bool) {
	b := c.getShard(key)
	b.Lock()
	if b.rehashing {
		_, ok = b.nb.idx.Delete(key)
		if !ok {
			_, ok = b.idx.Delete(key)
		}
	} else {
		_, ok = b.idx.Delete(key)
	}
	b.Unlock()

	return
}

// Scan
func (c *GigaCache[K]) Scan(f func(K, any, int64) bool) {
	for _, b := range c.buckets {
		b.RLock()
		b.idx.Scan(func(key K, idx Idx) bool {
			val, ts, ok := b.get(idx)
			if ok {
				return f(key, val, ts)
			}
			return true
		})
		b.RUnlock()
	}
}

func parseTTL(b []byte) int64 {
	// check bound
	_ = b[ttlBytes-1]
	return *(*int64)(unsafe.Pointer(&b[0]))
}

// eliminate the expired key-value pairs.
func (b *bucket[K]) eliminate() {
	// if rehashing
	if b.rehashing {
		b.migrate()
		return
	}

	if b.allocTimes%probeInterval != 0 {
		return
	}

	// bucket is empty
	if b.idx.Len() == 0 {
		return
	}

	var failCont, ttl int64
	rdm := rand.Uint64()

	// probe expired entries
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

		// delete expired
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

	b.migrate()
}

// Migrate
func (c *GigaCache[K]) Migrate() {
	for _, b := range c.buckets {
		b.Lock()
		b.migrate()
		b.Unlock()
	}
}

// migrate put valid key-value pairs to the new bucket.
func (b *bucket[K]) migrate() {
	if !b.rehashing {
		if rate := float64(b.idx.Len()) / float64(b.allocTimes); rate > migrateThreshold {
			return
		}

		// start rehash
		b.rehashing = true
		b.nb = &bucket[K]{
			idx:    hashmap.New[K, Idx](b.idx.Len()),
			bytes:  bpool.Get(),
			anyArr: make([]*anyItem, 0),
		}
	}

	// rehash
	keys := make([]K, 0, rehashCount)
	b.idx.Scan(func(key K, idx Idx) bool {
		v, ts, ok := b.getNoCopy(idx)
		if ok {
			b.nb.set(key, v, ts)
		}
		keys = append(keys, key)

		return len(keys) < rehashCount
	})

	for _, key := range keys {
		b.idx.Delete(key)
	}

	// rehash finished
	if b.idx.Len() == 0 {
		b.bytes = b.bytes[:0]
		bpool.Put(b.bytes)

		b.bytes = b.nb.bytes
		b.anyArr = b.nb.anyArr
		b.idx = b.nb.idx
		b.mtimes++
		b.allocTimes = b.nb.allocTimes

		b.rehashing = false
		b.nb = nil
	}
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
			b.allocTimes, k, i, slices.Clone(b.bytes),
		})

		b.RUnlock()
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
			allocTimes: b.C,
			idx:        hashmap.New[K, Idx](len(b.K)),
			bytes:      b.B,
			anyArr:     make([]*anyItem, 0),
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
	AllocTimes   uint64
	LenBytes     uint64
	LenAny       uint64
	MigrateTimes uint64
}

// Stat
func (c *GigaCache[K]) Stat() (s CacheStat) {
	for _, b := range c.buckets {
		b.RLock()
		s.Len += uint64(b.idx.Len())
		s.AllocTimes += uint64(b.allocTimes)
		s.LenBytes += uint64(len(b.bytes))
		s.LenAny += uint64(len(b.anyArr))
		s.MigrateTimes += uint64(b.mtimes)
		b.RUnlock()
	}
	return
}

// ExpRate
func (s CacheStat) ExpRate() float64 {
	return float64(s.Len) / float64(s.AllocTimes) * 100
}
