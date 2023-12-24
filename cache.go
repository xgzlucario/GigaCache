package cache

import (
	"encoding/binary"
	"slices"
	"sync"
	"time"

	"github.com/dolthub/swiss"
	"github.com/zeebo/xxh3"
)

const (
	noTTL = 0
)

// GigaCache defination.
type GigaCache struct {
	mask    uint64
	opt     *Option
	bpool   *BufferPool
	buckets []*bucket
}

// bucket is the data container for GigaCache.
type bucket struct {
	sync.RWMutex

	opt  *Option
	root *GigaCache

	// Key & idx is Indexes, and data is where the data is actually stored.
	idx  *swiss.Map[Key, Idx]
	data []byte

	// Stat for runtime.
	alloc    uint64
	inused   uint64
	reused   uint64
	migrates uint64
	evict    uint64
	probe    uint64
}

// New returns new GigaCache instance.
func New(opt Option) *GigaCache {
	shardCount := opt.ShardCount
	cache := &GigaCache{
		mask:    uint64(shardCount - 1),
		opt:     &opt,
		buckets: make([]*bucket, shardCount),
		bpool:   NewBufferPool(),
	}
	// init buckets.
	for i := range cache.buckets {
		cache.buckets[i] = cache.newBucket(opt.DefaultIdxMapSize, opt.DefaultBufferSize)
	}
	return cache
}

// newBucket returns a new bucket.
func (c *GigaCache) newBucket(idxSize, bufferSize int) *bucket {
	return &bucket{
		opt:  c.opt,
		root: c,
		idx:  swiss.NewMap[Key, Idx](uint32(idxSize)),
		data: c.bpool.Get(bufferSize)[:0],
	}
}

// getShard returns the bucket and the real key by hash(kstr).
func (c *GigaCache) getShard(kstr []byte) (*bucket, Key) {
	hash := xxh3.Hash(kstr)
	return c.buckets[hash&c.mask], newKey(hash)
}

// find return values by given Key and Idx.
// MAKE SURE check idx valid before call this func.
func (b *bucket) find(key Key, idx Idx) (total int, kstr []byte, val []byte) {
	var index = idx.start()
	// klen
	klen, n := binary.Uvarint(b.data[index:])
	index += n
	// vlen
	vlen, n := binary.Uvarint(b.data[index:])
	index += n
	// key
	kstr = b.data[index : index+int(klen)]
	index += int(klen)
	// value
	val = b.data[index : index+int(vlen)]
	index += int(vlen)

	return index - idx.start(), kstr, val
}

// Get returns value with expiration time by the key.
func (c *GigaCache) Get(kstr []byte) ([]byte, int64, bool) {
	bucket, key := c.getShard(kstr)
	bucket.RLock()

	// find index map.
	idx, ok := bucket.idx.Get(key)
	if !ok || idx.expired() {
		bucket.RUnlock()
		return nil, 0, false
	}

	// find data.
	_, _, val := bucket.find(key, idx)
	val = slices.Clone(val)
	bucket.RUnlock()

	return val, idx.TTL(), ok
}

//	 map[Key]Idx ----+
//	                 |
//	                 v
//	               start
//			   +-----+------------+------------+------------+------------+-----+
//			   | ... |    klen    |    vlen    |    key     |    value   | ... |
//			   +-----+------------+------------+------------+------------+-----+
//		             |<- varint ->|<- varint ->|<-- klen -->|<-- vlen -->|
//
// set stores key-value pair into bucket.
func (b *bucket) set(key Key, kstr []byte, bytes []byte, ts int64) {
	// alloc new space.
	b.idx.Put(key, newIdx(len(b.data), ts))
	before := len(b.data)

	b.data = binary.AppendUvarint(b.data, uint64(len(kstr)))
	b.data = binary.AppendUvarint(b.data, uint64(len(bytes)))
	b.data = append(b.data, kstr...)
	b.data = append(b.data, bytes...)

	alloc := len(b.data) - before
	b.alloc += uint64(alloc)
	b.inused += uint64(alloc)
}

// SetTx store key-value pair with deadline.
func (c *GigaCache) SetTx(kstr, val []byte, ts int64) {
	b, key := c.getShard(kstr)
	b.Lock()
	b.eliminate()
	b.set(key, kstr, val, ts)
	b.Unlock()
}

// Set store key-value pair.
func (c *GigaCache) Set(kstr, val []byte) {
	c.SetTx(kstr, val, noTTL)
}

// SetEx store key-value pair with expired duration.
func (c *GigaCache) SetEx(kstr, val []byte, dur time.Duration) {
	c.SetTx(kstr, val, GetNanoSec()+int64(dur))
}

// Delete removes the key-value pair by the key.
func (c *GigaCache) Delete(kstr []byte) {
	b, key := c.getShard(kstr)
	b.Lock()
	if _, ok := b.idx.Get(key); ok {
		b.idx.Delete(key)
	}
	b.eliminate()
	b.Unlock()
}

// Walker is the callback function for iterator.
type Walker func(key []byte, value []byte, ttl int64) (stop bool)

// scan
func (b *bucket) scan(f Walker) {
	b.idx.Iter(func(key Key, idx Idx) bool {
		if idx.expired() {
			return false
		}
		_, kstr, val := b.find(key, idx)
		return f(kstr, val, idx.TTL())
	})
}

// Scan walk all alive key-value pairs.
func (c *GigaCache) Scan(f Walker) {
	for _, b := range c.buckets {
		b.RLock()
		b.scan(func(key, val []byte, ts int64) bool {
			return f(key, val, ts)
		})
		b.RUnlock()
	}
}

// Migrate move all data to new buckets.
func (c *GigaCache) Migrate() {
	for _, b := range c.buckets {
		b.Lock()
		b.migrate()
		b.Unlock()
	}
}

// OnEvictCallback is the callback function of evict key-value pair.
// DO NOT EDIT the input params key value.
type OnEvictCallback func(key, value []byte)

// eliminate the expired key-value pairs.
func (b *bucket) eliminate() {
	var failed, pcount int

	// probing
	b.idx.Iter(func(key Key, idx Idx) bool {
		b.probe++

		if idx.expired() {
			total, kstr, val := b.find(key, idx)
			// on evict
			if b.opt.OnEvict != nil {
				b.opt.OnEvict(kstr, val)
			}
			b.idx.Delete(key)
			b.inused -= uint64(total)
			b.evict++
			failed = 0

			return false
		}

		failed++
		if failed >= b.opt.MaxFailCount {
			return true
		}

		pcount++
		return pcount > b.opt.MaxProbeCount
	})

	// on migrate threshold
	rate := float64(b.inused) / float64(b.alloc)
	delta := b.alloc - b.inused
	if delta >= b.opt.MigrateDelta && rate <= b.opt.MigrateThresRatio {
		b.migrate()
	}
}

// migrate move valid key-value pairs to the new container to save memory.
func (b *bucket) migrate() {
	// create new bucket.
	nb := b.root.newBucket(b.idx.Count(), len(b.data))

	// migrate datas to new bucket.
	b.idx.Iter(func(key Key, idx Idx) bool {
		if idx.expired() {
			return false
		}
		_, kstr, val := b.find(key, idx)
		nb.set(key, kstr, val, idx.TTL())
		return false
	})

	// reuse buffer.
	b.root.bpool.Put(b.data)

	// replace old bucket.
	b.idx = nb.idx
	b.data = nb.data
	b.alloc = nb.alloc
	b.inused = nb.inused
	b.reused = nb.reused
	b.migrates++
}

// CacheStat is the runtime statistics of Gigacache.
type CacheStat struct {
	Len      uint64
	Alloc    uint64
	Inused   uint64
	Reused   uint64
	Migrates uint64
	Evict    uint64
	Probe    uint64
}

// Stat return the runtime statistics of Gigacache.
func (c *GigaCache) Stat() (s CacheStat) {
	for _, b := range c.buckets {
		b.RLock()
		s.Len += uint64(b.idx.Count())
		s.Alloc += b.alloc
		s.Inused += b.inused
		s.Reused += b.reused
		s.Migrates += b.migrates
		s.Evict += b.evict
		s.Probe += b.probe
		b.RUnlock()
	}
	return
}

// ExpRate
func (s CacheStat) ExpRate() float64 {
	return float64(s.Inused) / float64(s.Alloc) * 100
}

// EvictRate
func (s CacheStat) EvictRate() float64 {
	return float64(s.Evict) / float64(s.Probe) * 100
}
