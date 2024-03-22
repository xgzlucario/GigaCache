package cache

import (
	"runtime"
	"slices"
	"time"

	"github.com/sourcegraph/conc/pool"
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
func (c *GigaCache) Remove(kstr string) {
	b, key := c.getShard(kstr)
	b.Lock()
	b.eliminate()
	b.remove(key, kstr)
	b.Unlock()
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
	for _, b := range c.buckets {
		b.RLock()
		next := b.scan(f)
		b.RUnlock()
		if !next {
			return
		}
	}
}

// Migrate move all data to new buckets with num cpu.
func (c *GigaCache) Migrate(numCPU ...int) {
	cpu := runtime.NumCPU()
	if len(numCPU) > 0 {
		cpu = numCPU[0]
	}

	if cpu == 1 {
		for _, b := range c.buckets {
			b.Lock()
			b.migrate()
			b.Unlock()
		}

	} else {
		p := pool.New().WithMaxGoroutines(cpu)
		for _, b := range c.buckets {
			b := b
			p.Go(func() {
				b.Lock()
				b.migrate()
				b.Unlock()
			})
		}
		p.Wait()
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
		s.Len += b.index.Len()
		s.Conflict += b.conflict.Len()
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
