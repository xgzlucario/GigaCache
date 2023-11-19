package cache

import (
	"slices"
	"sync"
	"time"

	"github.com/dolthub/swiss"
	bproto "github.com/xgzlucario/GigaCache/proto"
	"github.com/zeebo/xxh3"
	"google.golang.org/protobuf/proto"
)

const (
	noTTL              = 0
	defaultShardsCount = 1024
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
	reuseDataLength [reuseSpace]int
	reuseDataPos    [reuseSpace]int

	// For rehash migrate.
	rehash bool
	nb     *bucket
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
		cache.buckets[i] = &bucket{
			idx:  swiss.NewMap[Key, Idx](8),
			data: make([]byte, 0, bufferSize),
		}
	}
	return cache
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

// Has return the key exists or not.
func (c *GigaCache) Has(kstr string) bool {
	b, key := c.getShard(kstr)
	b.RLock()
	defer b.RUnlock()

	if b.rehash {
		if b.nb.has(key) {
			return true
		}
	}
	return b.has(key)
}

// has
func (b *bucket) has(k Key) bool {
	v, ok := b.idx.Get(k)
	return ok && !v.expired()
}

// Get returns value with expiration time by the key.
func (c *GigaCache) Get(kstr string) ([]byte, int64, bool) {
	b, key := c.getShard(kstr)
	b.RLock()
	defer b.RUnlock()

	if b.rehash {
		v, ts, ok := b.nb.get(key)
		if ok {
			return v, ts, ok
		}
	}
	return b.get(key)
}

// get
func (b *bucket) get(key Key) ([]byte, int64, bool) {
	idx, ok := b.idx.Get(key)
	if !ok || idx.expired() {
		return nil, 0, false
	}
	_, val, ok := b.find(key, idx)
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
	if b.rehash {
		b.nb.set(key, kstr, bytes, ts)
		return
	}

	need := len(kstr) + len(bytes)
	// reuse space.
	for i, offset := range b.reuseDataLength {
		if offset >= need {
			start := b.reuseDataPos[i]

			b.idx.Put(key, newIdx(start, len(bytes), ts))
			copy(b.data[start:], kstr)
			copy(b.data[start+len(kstr):], bytes)

			b.inused += uint64(need)
			b.resetReused(i)
			return
		}
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
	c.SetTx(kstr, val, GetClock()+int64(dur))
}

// Delete removes the key-value pair by the key.
func (c *GigaCache) Delete(kstr string) bool {
	b, k := c.getShard(kstr)
	b.Lock()
	b.eliminate()
	defer b.Unlock()

	if b.rehash {
		v, ok := b.nb.idx.Get(k)
		if ok {
			b.nb.idx.Delete(k)
			b.nb.updateEvict(k, v)
			return !v.expired()
		}
	}

	v, ok := b.idx.Get(k)
	if ok {
		b.idx.Delete(k)
		b.updateEvict(k, v)
		return !v.expired()
	}

	return false
}

// Walker is the callback function for iterator.
type Walker func(key []byte, value []byte, ttl int64) bool

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

// Keys returns all alive keys in cache.
func (c *GigaCache) Keys() (keys []string) {
	for _, b := range c.buckets {
		b.RLock()
		if keys == nil {
			keys = make([]string, 0, len(c.buckets)*b.idx.Count())
		}
		b.scan(func(key []byte, _ []byte, _ int64) bool {
			keys = append(keys, string(key))
			return false
		})
		b.RUnlock()
	}

	return
}

// updateEvict
func (b *bucket) updateEvict(key Key, idx Idx) {
	used := key.klen() + idx.offset()
	b.inused -= uint64(used)

	for i, length := range b.reuseDataLength {
		if used > length {
			b.reuseDataLength[i] = used
			b.reuseDataPos[i] = idx.start()
			return
		}
	}
}

// resetReused
func (b *bucket) resetReused(i int) {
	b.reuseDataLength[i] = 0
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

	if b.rehash {
		b.migrate()
		return
	}

	// on migrate threshold
	rate := float64(b.inused) / float64(b.alloc)
	delta := b.alloc - b.inused
	if delta >= migrateDelta && rate <= migrateThresRatio {
		b.migrate()
	}
}

// initRehashBucket
func (b *bucket) initRehashBucket() {
	b.nb = &bucket{
		idx:  swiss.NewMap[Key, Idx](uint32(float64(b.idx.Count()) * 1.5)),
		data: make([]byte, 0, len(b.data)*2),
	}
}

// migrate move valid key-value pairs to the new container to save memory.
func (b *bucket) migrate() {
	if !b.rehash {
		b.rehash = true
		b.initRehashBucket()
		return
	}

	var count int
	// fewer key-value pairs moved in a single migrate operation.
	b.idx.Iter(func(key Key, idx Idx) bool {
		kstr, val, ok := b.find(key, idx)
		if ok {
			b.nb.set(key, kstr, val, idx.TTL())
		}
		b.idx.Delete(key)

		count++
		return count >= 100
	})

	// finish rehash.
	if b.idx.Count() == 0 {
		b.idx = b.nb.idx
		b.data = b.nb.data
		b.alloc = b.nb.alloc
		b.inused = b.nb.inused
		b.nb = nil
		b.rehash = false
		b.reuseDataLength = [reuseSpace]int{}
		b.reuseDataPos = [reuseSpace]int{}
		b.mgtimes++
	}
}

// MarshalBinary serialize the cache to binary data.
func (c *GigaCache) MarshalBinary() ([]byte, error) {
	var data bproto.Cache
	for _, b := range c.buckets {
		b.RLock()
		// init
		if data.K == nil {
			n := len(c.buckets) * b.idx.Count()
			data.K = make([][]byte, 0, n)
			data.V = make([][]byte, 0, n)
			data.T = make([]int64, 0, n)
		}

		b.idx.Iter(func(key Key, idx Idx) bool {
			kstr, val, ok := b.find(key, idx)
			if ok {
				data.K = append(data.K, kstr)
				data.V = append(data.V, val)
				data.T = append(data.T, idx.TTL())
			}
			return false
		})

		b.RUnlock()
	}

	return proto.Marshal(&data)
}

// UnmarshalBinary deserialize the cache from binary data.
func (c *GigaCache) UnmarshalBinary(src []byte) error {
	var data bproto.Cache

	if err := proto.Unmarshal(src, &data); err != nil {
		return err
	}
	for i, k := range data.K {
		c.SetTx(*b2s(k), data.V[i], data.T[i])
	}

	return nil
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
		if b.rehash {
			s.Len += uint64(b.nb.idx.Count())
			s.BytesAlloc += b.nb.alloc
			s.BytesInused += b.nb.inused
		}
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
