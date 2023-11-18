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

	// eliminate probing
	maxProbeCount = 1000
	maxFailCount  = 3

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

type V struct {
	Idx
	TTL int64
}

func (v V) expired() bool {
	return v.TTL != noTTL && v.TTL < GetClock()
}

// bucket
type bucket struct {
	sync.RWMutex

	// idx is Indexes, and data is where the data is actually stored.
	idx  *swiss.Map[Key, V]
	data []byte

	// stat for runtime.
	alloc      uint64
	inused     uint64
	mgtimes    uint64
	evictCount uint64
	probeCount uint64

	// for reused bytes.
	roffset int
	rstart  int

	// for rehash.
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
			idx:  swiss.NewMap[Key, V](8),
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
func (b *bucket) find(k Key, v V) ([]byte, []byte, bool) {
	if v.expired() {
		return nil, nil, false
	}

	pos := v.start() + k.klen()
	kstr := b.data[v.start():pos]
	data := b.data[pos : pos+v.offset()]

	return kstr, data, true
}

// Has
func (c *GigaCache) Has(kstr string) bool {
	b, key := c.getShard(kstr)
	b.RLock()
	defer b.RUnlock()

	if b.rehash && b.nb.idx.Has(key) {
		return true
	}
	return b.idx.Has(key)
}

// Get returns value with expiration time by the key.
func (c *GigaCache) Get(kstr string) ([]byte, int64, bool) {
	b, key := c.getShard(kstr)
	b.RLock()
	defer b.RUnlock()

	if b.rehash && b.nb.idx.Has(key) {
		return b.nb.get(key)
	}
	return b.get(key)
}

// get
func (b *bucket) get(k Key) ([]byte, int64, bool) {
	v, ok := b.idx.Get(k)
	if ok {
		_, val, ok := b.find(k, v)
		return slices.Clone(val), v.TTL, ok
	}
	return nil, 0, false
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
	if b.roffset >= need {
		b.idx.Put(key, V{
			Idx: newIdx(b.rstart, len(bytes)),
			TTL: ts,
		})
		copy(b.data[b.rstart:], kstr)
		copy(b.data[b.rstart+len(kstr):], bytes)

		b.inused += uint64(need)
		b.resetReused()
		return
	}

	// alloc new space.
	b.idx.Put(key, V{
		Idx: newIdx(len(b.data), len(bytes)),
		TTL: ts,
	})
	b.data = append(b.data, kstr...)
	b.data = append(b.data, bytes...)

	b.alloc += uint64(need)
	b.inused += uint64(need)
}

// SetTx store key-value pair with deadline.
func (c *GigaCache) SetTx(kstr string, val []byte, ts int64) {
	b, key := c.getShard(kstr)
	b.Lock()
	b.set(key, s2b(&kstr), val, ts)
	b.eliminate()
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
	defer b.Unlock()

	if b.rehash && b.nb.delete(k) {
		return true
	}
	return b.delete(k)
}

// delete
func (b *bucket) delete(k Key) bool {
	v, ok := b.idx.Get(k)
	if ok {
		b.updateEvict(k, v)
		b.idx.Delete(k)
	}
	return ok
}

// scan
func (b *bucket) scan(f func(string, []byte, int64) bool) {
	b.idx.Iter(func(k Key, v V) bool {
		kstr, val, ok := b.find(k, v)
		if ok {
			return f(string(kstr), val, v.TTL)
		}
		return false
	})
}

// Scan walk all key-value pairs.
func (c *GigaCache) Scan(f func(string, []byte, int64) bool) {
	for _, b := range c.buckets {
		b.RLock()
		b.scan(func(s string, b []byte, i int64) bool {
			return f(s, slices.Clone(b), i)
		})
		b.RUnlock()
	}
}

// Keys returns all keys.
func (c *GigaCache) Keys() (keys []string) {
	for _, b := range c.buckets {
		b.RLock()
		if keys == nil {
			keys = make([]string, 0, len(c.buckets)*b.idx.Count())
		}
		b.scan(func(key string, _ []byte, _ int64) bool {
			keys = append(keys, key)
			return false
		})
		b.RUnlock()
	}

	return
}

// updateEvict
func (b *bucket) updateEvict(k Key, v V) {
	used := k.klen() + v.offset()
	b.inused -= uint64(used)

	if used > b.roffset {
		b.roffset = used
		b.rstart = v.start()
	}
}

// resetReused
func (b *bucket) resetReused() {
	b.roffset = 0
}

// eliminate the expired key-value pairs.
func (b *bucket) eliminate() {
	var pcount int
	var failed byte

	// probing
	b.idx.Iter(func(k Key, v V) bool {
		b.probeCount++

		if v.expired() {
			b.idx.Delete(k)
			b.evictCount++
			b.updateEvict(k, v)
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
		idx:  swiss.NewMap[Key, V](uint32(float64(b.idx.Count()) * 1.5)),
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
	b.idx.Iter(func(key Key, v V) bool {
		kstr, val, ok := b.find(key, v)
		if ok {
			b.nb.set(key, kstr, val, v.TTL)
		}
		b.idx.Delete(key)

		count++
		return count >= 100
	})

	// not finish yet.
	if b.idx.Count() > 0 {
		return
	}

	// swap buckets.
	b.data = b.nb.data
	b.alloc = b.nb.alloc
	b.inused = b.nb.inused
	b.nb = nil
	b.rehash = false
	b.resetReused()
	b.mgtimes++
}

// MarshalBinary
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

		b.idx.Iter(func(k Key, v V) bool {
			kstr, val, ok := b.find(k, v)
			if ok {
				data.K = append(data.K, kstr)
				data.V = append(data.V, val)
				data.T = append(data.T, v.TTL)
			}
			return false
		})

		b.RUnlock()
	}

	return proto.Marshal(&data)
}

// UnmarshalBinary
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
