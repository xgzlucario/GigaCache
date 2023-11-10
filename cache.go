package cache

import (
	"slices"
	"sync"
	"time"
	"unsafe"

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
	maxProbeCount = 10000
	maxFailCount  = 10
	probemsecs    = 10

	// migrateThres defines the conditions necessary to trigger a migrate operation.
	// Ratio recommended between 0.6 and 0.7, see bench data for details.
	migrateThresRatio = 0.6
)

var (
	// Reuse buffer to reduce memory allocation.
	bpool = NewBytePoolCap(defaultShardsCount, 0, bufferSize)
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

	// index and data.
	idx   *swiss.Map[Key, V]
	bytes []byte
	items []*item

	// stat for runtime.
	alloc      uint64
	inused     uint64
	mgtimes    uint64
	evictCount uint64
	probeCount uint64
	lastEvict  time.Time

	// for reused bytes.
	roffset int
	rstart  int
}

type item struct {
	kstr string
	val  any
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
			idx:   swiss.NewMap[Key, V](8),
			bytes: bpool.Get(),
			items: make([]*item, 0),
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
func (b *bucket) find(k Key, v V, nocopy ...bool) (string, any, int64, bool) {
	if v.expired() {
		return "", nil, 0, false
	}

	if v.IsAny() {
		n := b.items[v.start()]
		return n.kstr, n.val, v.TTL, true

	} else {
		vpos := v.start() + k.klen()
		kstr := b.bytes[v.start():vpos]
		bytes := b.bytes[vpos : vpos+v.offset()]

		// nocopy
		if len(nocopy) > 0 {
			return *b2s(kstr), bytes, v.TTL, true
		}
		return string(kstr), slices.Clone(bytes), v.TTL, true
	}
}

// Has
func (c *GigaCache) Has(kstr string) bool {
	b, key := c.getShard(kstr)
	b.RLock()
	ok := b.idx.Has(key)
	b.RUnlock()
	return ok
}

// Get returns value with expiration time by the key.
func (c *GigaCache) Get(kstr string) (any, int64, bool) {
	b, key := c.getShard(kstr)
	b.RLock()
	if v, ok := b.idx.Get(key); ok {
		_, val, ts, ok := b.find(key, v)
		b.RUnlock()
		return val, ts, ok
	}
	b.RUnlock()
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
func (b *bucket) set(key Key, kstr string, val any, ts int64) {
	bytes, ok := val.([]byte)
	if ok {
		need := len(kstr) + len(bytes)
		// reuse.
		if b.roffset >= need {
			b.idx.Put(key, V{
				Idx: newIdx(b.rstart, len(bytes), false),
				TTL: ts,
			})
			copy(b.bytes[b.rstart:], kstr)
			copy(b.bytes[b.rstart+len(kstr):], bytes)

			b.inused += uint64(need)
			b.resetReused()
			return
		}

		// alloc new space.
		b.idx.Put(key, V{
			Idx: newIdx(len(b.bytes), len(bytes), false),
			TTL: ts,
		})
		b.bytes = append(b.bytes, kstr...)
		b.bytes = append(b.bytes, bytes...)

		b.alloc += uint64(need)
		b.inused += uint64(need)

	} else {
		v, ok := b.idx.Get(key)
		// update inplace.
		if ok && v.IsAny() {
			b.idx.Put(key, V{Idx: v.Idx, TTL: ts})
			b.items[v.start()].val = val

		} else {
			b.idx.Put(key, V{Idx: newIdx(len(b.items), 1, true), TTL: ts})
			b.items = append(b.items, &item{kstr, val})

			// A uintptr type can be approximated as uint64.
			need := uint64(8)
			b.alloc += need
			b.inused += need
		}
	}
}

// SetTx store key-value pair with deadline.
func (c *GigaCache) SetTx(kstr string, val any, ts int64) {
	b, key := c.getShard(kstr)
	b.Lock()
	b.eliminate()
	b.set(key, kstr, val, ts)
	b.Unlock()
}

// Set store key-value pair.
func (c *GigaCache) Set(kstr string, val any) {
	c.SetTx(kstr, val, noTTL)
}

// SetEx store key-value pair with expired duration.
func (c *GigaCache) SetEx(kstr string, val any, dur time.Duration) {
	c.SetTx(kstr, val, GetClock()+int64(dur))
}

// Delete removes the key-value pair by the key.
func (c *GigaCache) Delete(kstr string) bool {
	b, key := c.getShard(kstr)
	b.Lock()
	idx, ok := b.idx.Get(key)
	if ok {
		b.idx.Delete(key)
		b.inused -= uint64(key.klen() + idx.offset())
	}
	b.eliminate()
	b.Unlock()

	return ok
}

// scan
func (b *bucket) scan(f func(string, any, int64) bool, nocopy ...bool) {
	b.idx.Iter(func(k Key, v V) bool {
		kstr, val, ts, ok := b.find(k, v, nocopy...)
		if ok {
			return f(kstr, val, ts)
		}
		return false
	})
}

// Scan walk all key-value pairs.
func (c *GigaCache) Scan(f func(string, any, int64) bool) {
	for _, b := range c.buckets {
		b.RLock()
		b.scan(f)
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
		b.scan(func(key string, _ any, _ int64) bool {
			keys = append(keys, key)
			return false
		}, true)
		b.RUnlock()
	}

	return
}

// updateReused
func (b *bucket) updateReused(start, offset int) {
	if offset > b.roffset {
		b.roffset = offset
		b.rstart = start
	}
}

// resetReused
func (b *bucket) resetReused() {
	b.roffset = 0
}

// eliminate the expired key-value pairs.
func (b *bucket) eliminate() {
	if time.Since(b.lastEvict).Milliseconds() < probemsecs {
		return
	}
	var pcount int
	var failed byte

	// probing
	b.idx.Iter(func(k Key, v V) bool {
		b.probeCount++

		if v.expired() {
			b.idx.Delete(k)
			b.evictCount++

			// release mem space.
			used := k.klen() + v.offset()
			b.inused -= uint64(used)
			if !v.IsAny() {
				b.updateReused(v.start(), used)
			}
			failed = 0

			return false
		}

		failed++
		if failed > maxFailCount {
			return true
		}

		pcount++
		return pcount > maxProbeCount
	})

	b.lastEvict = time.Now()

	// on migrate threshold
	if rate := float64(b.inused) / float64(b.alloc); rate <= migrateThresRatio {
		b.migrate()
	}
}

// Migrate call migrate force.
func (c *GigaCache) Migrate() {
	for _, b := range c.buckets {
		b.Lock()
		b.migrate()
		b.Unlock()
	}
}

// migrate move valid key-value pairs to the new container to save memory.
func (b *bucket) migrate() {
	nb := &bucket{
		idx:   swiss.NewMap[Key, V](uint32(b.idx.Count())),
		bytes: bpool.Get(),
		items: make([]*item, 0),
	}

	b.idx.Iter(func(key Key, v V) bool {
		kstr, val, ts, ok := b.find(key, v, true)
		if ok {
			nb.set(key, kstr, val, ts)
		}
		return false
	})

	// release bytes.
	b.bytes = b.bytes[:0]
	bpool.Put(b.bytes)

	// swap buckets.
	b.bytes = nb.bytes
	b.items = nb.items
	b.idx = nb.idx
	b.alloc = nb.alloc
	b.inused = nb.inused
	b.resetReused()
	b.mgtimes++
}

// MarshalBytes
func (c *GigaCache) MarshalBytes() ([]byte, error) {
	return c.MarshalBytesFunc(nil)
}

// MarshalBytesFunc serializes all key-value pairs with a value of []byte,
// and calls the callback function when value is any.
func (c *GigaCache) MarshalBytesFunc(cb func(string, any, int64)) ([]byte, error) {
	var data bproto.Cache

	for _, b := range c.buckets {
		b.RLock()

		// init
		if data.K == nil {
			n := len(c.buckets) * b.idx.Count()
			data.K = make([]string, 0, n)
			data.V = make([][]byte, 0, n)
			data.T = make([]int64, 0, n)
		}

		b.scan(func(kstr string, val any, ts int64) bool {
			// if bytes
			if bytes, ok := val.([]byte); ok {
				data.K = append(data.K, kstr)
				data.V = append(data.V, bytes)
				data.T = append(data.T, ts)
			} else if cb != nil {
				cb(kstr, val, ts)
			}
			return false
		}, true)

		b.RUnlock()
	}

	return proto.Marshal(&data)
}

// UnmarshalBytes
func (c *GigaCache) UnmarshalBytes(src []byte) error {
	var data bproto.Cache

	if err := proto.Unmarshal(src, &data); err != nil {
		return err
	}
	for i, k := range data.K {
		c.SetTx(k, data.V[i], data.T[i])
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

// Bytes convert to string unsafe
func b2s(buf []byte) *string {
	return (*string)(unsafe.Pointer(&buf))
}
