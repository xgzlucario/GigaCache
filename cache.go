package cache

import (
	"slices"
	"time"
)

const (
	noTTL = 0
	KB    = 1024

	// maxFailCount indicates that the evict algorithm break
	// when consecutive unexpired key-value pairs are detected.
	maxFailCount = 3
)

var bpool = NewBufferPool()

// GigaCache implements a key-value cache.
type GigaCache struct {
	mask    uint32
	hashFn  HashFn
	buckets []*bucket
}

// New returns new GigaCache instance.
func New(options Options) *GigaCache {
	if err := checkOptions(options); err != nil {
		panic(err)
	}
	cache := &GigaCache{
		mask:    options.ShardCount - 1,
		hashFn:  options.HashFn,
		buckets: make([]*bucket, options.ShardCount),
	}
	for i := range cache.buckets {
		cache.buckets[i] = newBucket(options)
	}
	return cache
}

// getShard returns the bucket and the real key by hash(kstr).
// sharding and index use different hash function, can reduce the hash conflicts greatly.
func (c *GigaCache) getShard(kstr string) (*bucket, Key) {
	hash := c.hashFn(kstr)
	hash32 := uint32(hash >> 1)
	return c.buckets[hash32&c.mask], Key(hash)
}

// Get returns value with expiration time by the key.
func (c *GigaCache) Get(kstr string) ([]byte, int64, bool) {
	b, key := c.getShard(kstr)
	b.RLock()
	val, ts, ok := b.get(kstr, key)
	if ok {
		val = slices.Clone(val)
	}
	b.RUnlock()
	return val, ts, ok
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

// Remove removes the key-value pair by the key.
func (c *GigaCache) Remove(kstr string) bool {
	b, key := c.getShard(kstr)
	b.Lock()
	b.eliminate()
	ok := b.remove(key, kstr)
	b.Unlock()
	return ok
}

// SetTTL set ttl for key.
func (c *GigaCache) SetTTL(kstr string, ts int64) bool {
	b, key := c.getShard(kstr)
	b.Lock()
	ok := b.setTTL(key, kstr, ts)
	b.eliminate()
	b.Unlock()
	return ok
}

// Walker is the callback function for iterator.
type Walker func(key, val []byte, ttl int64) (next bool)

// Scan walk all alive key-value pairs.
// DO NOT EDIT the bytes as they are NO COPY.
func (c *GigaCache) Scan(f Walker) {
	var next bool
	for _, b := range c.buckets {
		b.RLock()
		if b.options.DisableEvict {
			next = b.scan2(f)
		} else {
			next = b.scan(f)
		}
		b.RUnlock()
		if !next {
			return
		}
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

// Stat is the runtime statistics of GigaCache.
type Stat struct {
	Len      int
	Conflict int
	Alloc    uint64
	Unused   uint64
	Migrates uint64
	Evict    uint64
	Probe    uint64
}

// Stat return the runtime statistics of GigaCache.
func (c *GigaCache) Stat() (s Stat) {
	for _, b := range c.buckets {
		b.RLock()
		s.Len += b.index.Len() + b.cmap.Len()
		s.Conflict += b.cmap.Len()
		s.Alloc += uint64(len(b.data))
		s.Unused += b.unused
		s.Migrates += b.migrates
		s.Evict += b.evict
		s.Probe += b.probe
		b.RUnlock()
	}
	return
}

func (s Stat) UnusedRate() float64 {
	return float64(s.Unused) / float64(s.Alloc) * 100
}

func (s Stat) EvictRate() float64 {
	return float64(s.Evict) / float64(s.Probe) * 100
}
