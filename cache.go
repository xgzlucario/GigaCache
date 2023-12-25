package cache

import (
	"encoding/binary"
	"slices"
	"sync"
	"time"
	"unsafe"

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
	migrates uint64
	evict    uint64
	probe    uint64
}

// New returns new GigaCache instance.
func New(opt Option) *GigaCache {
	shardCount := opt.ShardCount
	if shardCount <= 0 {
		panic("shard count must be greater than 0")
	}
	// create cache.
	cache := &GigaCache{
		mask:    uint64(shardCount - 1),
		opt:     &opt,
		buckets: make([]*bucket, shardCount),
		bpool:   NewBufferPool(),
	}
	// init buckets.
	for i := range cache.buckets {
		cache.buckets[i] = &bucket{
			opt:  cache.opt,
			root: cache,
			idx:  swiss.NewMap[Key, Idx](opt.DefaultIdxMapSize),
			data: cache.bpool.Get(opt.DefaultBufferSize)[:0],
		}
	}
	return cache
}

// getShard returns the bucket and the real key by hash(kstr).
func (c *GigaCache) getShard(kstr string) (*bucket, Key) {
	hash := xxh3.HashString(kstr)
	return c.buckets[hash&c.mask], newKey(hash)
}

// find return values by given Key and Idx.
// MAKE SURE check idx valid before call this func.
func (b *bucket) find(idx Idx) (total int, kstr []byte, val []byte) {
	var index = idx.start()
	// klen
	klen, n := binary.Uvarint(b.data[index:])
	index += n
	// vlen
	vlen, n := binary.Uvarint(b.data[index:])
	index += n
	// kstr
	kstr = b.data[index : index+int(klen)]
	index += int(klen)
	// val
	val = b.data[index : index+int(vlen)]
	index += int(vlen)

	return index - idx.start(), kstr, val
}

// findEntry
func (b *bucket) findEntry(idx Idx) (entry []byte) {
	var index = idx.start()
	// klen
	klen, n := binary.Uvarint(b.data[index:])
	index += n
	// vlen
	vlen, n := binary.Uvarint(b.data[index:])
	index += n
	// entry
	entry = b.data[idx.start() : index+int(klen)+int(vlen)]
	return
}

// Get returns value with expiration time by the key.
func (c *GigaCache) Get(kstr string) ([]byte, int64, bool) {
	b, key := c.getShard(kstr)
	b.RLock()
	// find index map.
	idx, ok := b.idx.Get(key)
	if !ok || idx.expired() {
		b.RUnlock()
		return nil, 0, false
	}
	// find data.
	_, _, val := b.find(idx)
	val = slices.Clone(val)
	b.RUnlock()
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
//				     |<--------------------- entry --------------------->|
//
// set stores key-value pair into bucket.
func (b *bucket) set(key Key, kstr []byte, val []byte, ts int64) {
	// set index.
	b.idx.Put(key, newIdx(len(b.data), ts))
	before := len(b.data)

	// append klen, vlen, key, value.
	b.data = binary.AppendUvarint(b.data, uint64(len(kstr)))
	b.data = binary.AppendUvarint(b.data, uint64(len(val)))
	b.data = append(b.data, kstr...)
	b.data = append(b.data, val...)

	// update stat.
	alloc := len(b.data) - before
	b.alloc += uint64(alloc)
	b.inused += uint64(alloc)
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
func (c *GigaCache) Delete(kstr string) bool {
	b, key := c.getShard(kstr)
	b.Lock()
	b.eliminate()

	// find index map.
	idx, ok := b.idx.Get(key)
	if !ok || idx.expired() {
		b.Unlock()
		return false
	}
	// delete.
	b.idx.Delete(key)
	entry := b.findEntry(idx)
	b.inused -= uint64(len(entry))
	b.Unlock()
	return true
}

// SetTTL
func (c *GigaCache) SetTTL(kstr string, ts int64) bool {
	b, key := c.getShard(kstr)
	b.Lock()
	b.eliminate()

	// find index map.
	idx, ok := b.idx.Get(key)
	if !ok || idx.expired() {
		b.Unlock()
		return false
	}
	// update index.
	b.idx.Put(key, newIdx(idx.start(), ts))
	b.Unlock()
	return true
}

// Walker is the callback function for iterator.
type Walker func(key []byte, value []byte, ttl int64) (stop bool)

// scan
func (b *bucket) scan(f Walker) {
	b.idx.Iter(func(_ Key, idx Idx) bool {
		if idx.expired() {
			return false
		}
		_, kstr, val := b.find(idx)
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
	var failed, pcount uint16

	// probing
	b.idx.Iter(func(key Key, idx Idx) bool {
		b.probe++

		if idx.expired() {
			// on evict
			if b.opt.OnEvict != nil {
				total, kstr, val := b.find(idx)
				b.opt.OnEvict(kstr, val)
				b.inused -= uint64(total)

			} else {
				entry := b.findEntry(idx)
				b.inused -= uint64(len(entry))
			}

			b.idx.Delete(key)
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
	// create new data.
	newData := b.root.bpool.Get(len(b.data))[:0]
	b.alloc = 0
	b.inused = 0

	// migrate datas to new bucket.
	b.idx.Iter(func(key Key, idx Idx) bool {
		if idx.expired() {
			b.idx.Delete(key)
			return false
		}

		entry := b.findEntry(idx)
		// update with new position.
		b.idx.Put(key, newIdx(len(newData), idx.TTL()))
		newData = append(newData, entry...)

		b.alloc += uint64(len(entry))
		b.inused += uint64(len(entry))

		return false
	})

	// reuse buffer.
	b.root.bpool.Put(b.data)

	// replace old data.
	b.data = newData
	b.migrates++
}

// CacheStat is the runtime statistics of Gigacache.
type CacheStat struct {
	Len      uint64
	Alloc    uint64
	Inused   uint64
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

// s2b is string convert to bytes unsafe.
func s2b(str *string) []byte {
	strHeader := (*[2]uintptr)(unsafe.Pointer(str))
	byteSliceHeader := [3]uintptr{
		strHeader[0], strHeader[1], strHeader[1],
	}
	return *(*[]byte)(unsafe.Pointer(&byteSliceHeader))
}
