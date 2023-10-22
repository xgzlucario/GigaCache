package cache

import (
	"bytes"
	"encoding/gob"
	"slices"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/exp/rand"

	"github.com/dolthub/swiss"
	"github.com/zeebo/xxh3"
)

const (
	noTTL = 0

	// for ttl
	timeCarry = 1e9

	defaultShardsCount = 1024
	bufferSize         = 1024

	// eliminate probing
	probeInterval = 3
	probeCount    = 100

	// migrateThres defines the conditions necessary to trigger a migrate operation.
	// Ratio recommended between 0.6 and 0.7, see bench data for details.
	migrateThresRatio = 0.6

	maxFailCount = 5
)

var (
	// Reuse buffer to reduce memory allocation.
	bpool = NewBytePoolCap(defaultShardsCount, 0, bufferSize)

	// rand source
	source = rand.NewSource(uint64(time.Now().UnixNano()))
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
	idx     *swiss.Map[Key, V]
	alloc   int64
	mtimes  int64
	eltimes byte
	bytes   []byte
	items   []*item
	sync.RWMutex
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
		kstr := getSlice(b.bytes, v.start(), k.klen())
		bytes := getSlice(b.bytes, v.start()+k.klen(), v.offset())

		// nocopy
		if nocopy != nil && nocopy[0] {
			return *b2s(kstr), bytes, v.TTL, true
		}
		return string(kstr), slices.Clone(bytes), v.TTL, true
	}
}

// deleteGet
func (b *bucket) deleteGet(key Key) (V, bool) {
	v, ok := b.idx.Get(key)
	if ok {
		b.idx.Delete(key)
	}
	return v, ok
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
	defer b.RUnlock()
	if v, ok := b.idx.Get(key); ok {
		_, val, ts, ok := b.find(key, v)
		return val, ts, ok
	}
	return nil, 0, false
}

// RandomGet returns a random unexpired key-value pair with ttl.
func (c *GigaCache) RandomGet() (kstr string, val any, ts int64, ok bool) {
	rdm := source.Uint64()

	for i := uint64(0); i < uint64(len(c.buckets)); i++ {
		b := c.buckets[(rdm+i)&c.mask]
		b.Lock()
		b.idx.Iter(func(k Key, v V) bool {
			kstr, val, ts, ok = b.find(k, v)
			// unexpired
			if ok {
				return true

			} else {
				b.idx.Delete(k)
				return false
			}
		})
		b.Unlock()

		if ok {
			return
		}
	}

	return
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
		b.idx.Put(key, V{
			Idx: newIdx(len(b.bytes), len(bytes), false),
			TTL: ts,
		})
		b.bytes = append(b.bytes, kstr...)
		b.bytes = append(b.bytes, bytes...)
		b.alloc++

	} else {
		v, ok := b.idx.Get(key)
		// update inplace
		if ok && v.IsAny() {
			b.idx.Put(key, V{
				Idx: v.Idx,
				TTL: ts,
			})
			b.items[v.start()].val = val

		} else {
			b.idx.Put(key, V{
				Idx: newIdx(len(b.items), 1, true),
				TTL: ts,
			})
			b.items = append(b.items, &item{kstr, val})
			b.alloc++
		}
	}
}

// SetTx store key-value pair with deadline.
func (c *GigaCache) SetTx(kstr string, val any, ts int64) {
	b, key := c.getShard(kstr)
	b.Lock()
	b.set(key, kstr, val, ts)
	b.eliminate()
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
	ok := b.idx.Delete(key)
	b.eliminate()
	b.Unlock()

	return ok
}

// Rename
func (c *GigaCache) Rename(old, new string) bool {
	oldb, oldKey := c.getShard(old)
	oldb.Lock()
	defer oldb.Unlock()

	// same bucket
	newb, newKey := c.getShard(new)
	if oldb == newb {
		idx, ok := oldb.deleteGet(oldKey)
		if !ok {
			return false
		}
		oldb.idx.Put(newKey, idx)
		return true
	}

	// delete from old bucket.
	v, ok := oldb.deleteGet(oldKey)
	if !ok {
		return false
	}
	_, val, ts, ok := oldb.find(oldKey, v, true)
	if !ok {
		return false
	}

	// update new bucket.
	c.SetTx(new, val, ts)

	return true
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

// eliminate the expired key-value pairs.
func (b *bucket) eliminate() {
	b.eltimes = (b.eltimes + 1) % probeInterval
	if b.eltimes > 0 {
		return
	}

	var failCont, pcount int64

	// probing
	b.idx.Iter(func(k Key, v V) bool {
		if v.expired() {
			b.idx.Delete(k)
			failCont = 0
			return false
		}

		failCont++
		// break
		if failCont > maxFailCount {
			return true
		}

		pcount++
		return pcount > probeCount
	})

	// on migrate threshold
	if rate := float64(b.idx.Count()) / float64(b.alloc); rate < migrateThresRatio {
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

	b.bytes = b.bytes[:0]
	bpool.Put(b.bytes)

	b.bytes = nb.bytes
	b.items = nb.items
	b.idx = nb.idx
	b.alloc = nb.alloc
	b.mtimes++
}

// cacheJSON
type cacheJSON struct {
	K []string
	V [][]byte
	T []int64
}

// MarshalBytes
func (c *GigaCache) MarshalBytes() ([]byte, error) {
	return c.MarshalBytesFunc(nil)
}

// MarshalBytesFunc serializes all key-value pairs with a value of []byte,
// and calls the callback function when value is any.
func (c *GigaCache) MarshalBytesFunc(cb func(string, any, int64)) ([]byte, error) {
	var data cacheJSON
	gob.Register(data)

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
				data.T = append(data.T, ts/timeCarry) // ns -> s

			} else if cb != nil {
				cb(kstr, val, ts)
			}
			return false
		}, true)

		b.RUnlock()
	}

	// encode
	buf := bytes.NewBuffer(nil)
	gob.NewEncoder(buf).Encode(data)

	return buf.Bytes(), nil
}

// UnmarshalBytes
func (c *GigaCache) UnmarshalBytes(src []byte) error {
	var data cacheJSON
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
func (c *GigaCache) Stat() (s CacheStat) {
	for _, b := range c.buckets {
		b.RLock()
		s.Len += uint64(b.idx.Count())
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

func getSlice(b []byte, start int, offset int) []byte {
	return b[start : start+offset]
}

// Bytes convert to string unsafe
func b2s(buf []byte) *string {
	return (*string)(unsafe.Pointer(&buf))
}
