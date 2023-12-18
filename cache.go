package cache

import (
	"slices"
	"sync"
	"time"
	"unsafe"

	"github.com/dolthub/swiss"
	"github.com/zeebo/xxh3"
)

const (
	noTTL              = 0
	defaultShardsCount = 4096
	bufferSize         = 1024

	// eliminate probing.
	maxProbeCount = 1000
	maxFailCount  = 3

	// reuse space count.
	reuseSpace = 8

	// migrateThres defines the conditions necessary to trigger a migrate operation.
	// Ratio recommended is 0.6, see bench data for details.
	migrateThresRatio = 0.6
	migrateDelta      = 1024
)

// bpool is a global buffer pool.
var bpool = NewBufferPool(bufferSize)

// GigaCache return a new GigaCache instance.
type GigaCache struct {
	mask    uint64
	buckets []*bucket
}

// bucket is the container for key-value pairs.
type bucket struct {
	sync.RWMutex

	// Key & idx is Indexes, and data is where the data is actually stored.
	idx  *swiss.Map[Key, Idx]
	data []byte

	// Stat for runtime.
	alloc      uint64
	inused     uint64
	mgtimes    uint64
	evictCount uint64
	probeCount uint64

	// Reuse sharded space to save memory.
	reuseSlice reuseSlice
}

// New returns new GigaCache instance.
func New(shard ...int) *GigaCache {
	num := defaultShardsCount
	if len(shard) > 0 {
		num = shard[0]
	}
	cache := &GigaCache{
		mask:    uint64(num - 1),
		buckets: make([]*bucket, num),
	}
	for i := range cache.buckets {
		cache.buckets[i] = newBucket(64, bufferSize)
	}
	return cache
}

// newBucket returns a new bucket.
func newBucket(idxSize, bufferSize int) *bucket {
	return &bucket{
		idx:        swiss.NewMap[Key, Idx](uint32(idxSize)),
		data:       bpool.Get(bufferSize)[:0],
		reuseSlice: newReuseSlice(reuseSpace),
	}
}

// getShard returns the bucket and the real key by hash(kstr).
func (c *GigaCache) getShard(kstr string) (*bucket, Key) {
	hash := xxh3.HashString(kstr)
	return c.buckets[hash&c.mask], newKey(hash, len(kstr))
}

// find return values by given Key and Idx.
func (b *bucket) find(key Key, idx Idx) ([]byte, []byte, bool) {
	if idx.expired() {
		return nil, nil, false
	}

	pos := idx.start() + key.klen()
	kstr := b.data[idx.start():pos]
	data := b.data[pos : pos+idx.offset()]

	return kstr, data, true
}

// Get returns value with expiration time by the key.
func (c *GigaCache) Get(kstr string) ([]byte, int64, bool) {
	bucket, key := c.getShard(kstr)
	bucket.RLock()
	defer bucket.RUnlock()
	// find index map.
	idx, ok := bucket.idx.Get(key)
	if !ok {
		return nil, 0, false
	}
	// find data.
	_, val, ok := bucket.find(key, idx)
	if !ok {
		return nil, 0, false
	}
	return slices.Clone(val), idx.TTL(), ok
}

// set store key-value pair into bucket.
//
//	 map[Key]Idx ----+
//	                 |
//	                 v
//	               start
//			   +-----+------------------+-------------------+-----+
//			   | ... |    key bytes     |    value bytes    | ... |
//			   +-----+------------------+-------------------+-----+
//		             |<---  keylen  --->|<---   offset  --->|
//
// set stores key and value in an array of bytes and returns their index positions.
func (b *bucket) set(key Key, kstr []byte, bytes []byte, ts int64) {
	need := len(kstr) + len(bytes)
	// reuse empty space.
	start, ok := b.reuseSlice.pop(need)
	if ok {
		b.idx.Put(key, newIdx(start, len(bytes), ts))
		copy(b.data[start:], kstr)
		copy(b.data[start+len(kstr):], bytes)

		b.inused += uint64(need)
		return
	}

	// alloc new space.
	b.idx.Put(key, newIdx(len(b.data), len(bytes), ts))
	b.data = append(b.data, kstr...)
	b.data = append(b.data, bytes...)

	b.alloc += uint64(need)
	b.inused += uint64(need)
}

// SetTx store key-value pair with deadline.
func (c *GigaCache) SetTx(kstr string, val []byte, ts int64) {
	b, key := c.getShard(kstr)
	b.Lock()
	b.eliminate()
	b.set(key, s2b(&kstr), val, ts)
	b.Unlock()
}

// Set store key-value pair.
func (c *GigaCache) Set(kstr string, val []byte) {
	c.SetTx(kstr, val, noTTL)
}

// SetEx store key-value pair with expired duration.
func (c *GigaCache) SetEx(kstr string, val []byte, dur time.Duration) {
	c.SetTx(kstr, val, GetNanoSec()+int64(dur))
}

// Delete removes the key-value pair by the key.
func (c *GigaCache) Delete(kstr string) {
	b, key := c.getShard(kstr)
	b.Lock()
	if idx, ok := b.idx.Get(key); ok {
		b.idx.Delete(key)
		b.updateEvict(key, idx)
	}
	b.eliminate()
	b.Unlock()
}

// Walker is the callback function for iterator.
type Walker func(key []byte, value []byte, ttl int64) (stop bool)

// scan
func (b *bucket) scan(f Walker) {
	b.idx.Iter(func(key Key, idx Idx) bool {
		kstr, val, ok := b.find(key, idx)
		if ok {
			return f(kstr, val, idx.TTL())
		}
		return false
	})
}

// Scan walk all alive key-value pairs.
func (c *GigaCache) Scan(f Walker) {
	for _, b := range c.buckets {
		b.RLock()
		b.scan(func(s []byte, b []byte, i int64) bool {
			return f(s, b, i)
		})
		b.RUnlock()
	}
}

// updateEvict
func (b *bucket) updateEvict(key Key, idx Idx) {
	used := key.klen() + idx.offset()
	b.inused -= uint64(used)
	b.reuseSlice.push(used, idx.start())
}

// eliminate the expired key-value pairs.
func (b *bucket) eliminate() {
	var pcount int
	var failed byte

	// probing
	b.idx.Iter(func(key Key, idx Idx) bool {
		b.probeCount++

		if idx.expired() {
			b.idx.Delete(key)
			b.evictCount++
			b.updateEvict(key, idx)
			failed = 0

			return false
		}

		failed++
		if failed >= maxFailCount {
			return true
		}

		pcount++
		return pcount > maxProbeCount
	})

	// on migrate threshold
	rate := float64(b.inused) / float64(b.alloc)
	delta := b.alloc - b.inused
	if delta >= migrateDelta && rate <= migrateThresRatio {
		b.migrate()
	}
}

// migrate move valid key-value pairs to the new container to save memory.
func (b *bucket) migrate() {
	// create new bucket.
	nb := newBucket(b.idx.Count(), len(b.data))

	// migrate datas to new bucket.
	b.idx.Iter(func(key Key, idx Idx) bool {
		kstr, val, ok := b.find(key, idx)
		if ok {
			nb.set(key, kstr, val, idx.TTL())
		}
		return false
	})

	// reuse buffer.
	bpool.Put(b.data)

	// replace old bucket.
	b.idx = nb.idx
	b.data = nb.data
	b.alloc = nb.alloc
	b.inused = nb.inused
	b.reuseSlice = nb.reuseSlice
	b.mgtimes++
}

// CacheStat is the runtime statistics of Gigacache.
type CacheStat struct {
	Len          uint64
	BytesAlloc   uint64
	BytesInused  uint64
	MigrateTimes uint64
	EvictCount   uint64
	ProbeCount   uint64
}

// Stat return the runtime statistics of Gigacache.
func (c *GigaCache) Stat() (s CacheStat) {
	for _, b := range c.buckets {
		b.RLock()
		s.Len += uint64(b.idx.Count())
		s.BytesAlloc += b.alloc
		s.BytesInused += b.inused
		s.MigrateTimes += b.mgtimes
		s.EvictCount += b.evictCount
		s.ProbeCount += b.probeCount
		b.RUnlock()
	}
	return
}

// ExpRate
func (s CacheStat) ExpRate() float64 {
	return float64(s.BytesInused) / float64(s.BytesAlloc) * 100
}

// EvictRate
func (s CacheStat) EvictRate() float64 {
	return float64(s.EvictCount) / float64(s.ProbeCount) * 100
}

// s2b is string convert to bytes unsafe.
func s2b(str *string) []byte {
	strHeader := (*[2]uintptr)(unsafe.Pointer(str))
	byteSliceHeader := [3]uintptr{
		strHeader[0], strHeader[1], strHeader[1],
	}
	return *(*[]byte)(unsafe.Pointer(&byteSliceHeader))
}
