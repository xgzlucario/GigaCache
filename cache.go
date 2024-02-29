package cache

import (
	"encoding/binary"
	"slices"
	"sync"
	"time"
	"unsafe"

	"github.com/dolthub/swiss"
	"github.com/sourcegraph/conc/pool"
	"github.com/zeebo/xxh3"
)

const (
	noTTL = 0
)

// GigaCache implements a key-value cache.
type GigaCache struct {
	mask    uint64
	buckets []*bucket
}

// bucket is the data container for GigaCache.
type bucket struct {
	sync.RWMutex
	options *Options
	bpool   *BufferPool

	// index is the index map for cache, mapped hash(kstr) to the position that data real stored.
	index *swiss.Map[Key, Idx]

	// data store all key-value bytes data.
	data []byte

	// some runtime stats.
	hint     bool
	alloc    uint64
	inused   uint64
	migrates uint64
	evict    uint64
	probe    uint64
}

// New returns new GigaCache instance.
func New(options Options) *GigaCache {
	if err := checkOptions(options); err != nil {
		panic(err)
	}

	// create cache.
	cache := &GigaCache{
		mask:    uint64(options.ShardCount - 1),
		buckets: make([]*bucket, options.ShardCount),
	}
	bpool := NewBufferPool()

	// init buckets.
	for i := range cache.buckets {
		cache.buckets[i] = &bucket{
			options: &options,
			bpool:   bpool,
			index:   swiss.NewMap[Key, Idx](options.IndexSize),
			data:    bpool.Get(options.BufferSize)[:0],
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

// findEntry return entire entry bytes.
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
	idx, ok := b.index.Get(key)
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
	b.index.Put(key, newIdx(len(b.data), ts))
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
	idx, ok := b.index.Get(key)
	if !ok || idx.expired() {
		b.Unlock()
		return false
	}
	// delete.
	b.index.Delete(key)
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
	idx, ok := b.index.Get(key)
	if !ok || idx.expired() {
		b.Unlock()
		return false
	}
	// update index.
	b.index.Put(key, newIdx(idx.start(), ts))
	b.Unlock()
	return true
}

// Walker is the callback function for iterator.
type Walker func(key []byte, value []byte, ttl int64) (stop bool)

// scan
func (b *bucket) scan(f Walker) {
	b.index.Iter(func(_ Key, idx Idx) bool {
		if idx.expired() {
			return false
		}
		_, kstr, val := b.find(idx)
		return f(kstr, val, idx.TTL())
	})
}

// Scan walk all alive key-value pairs with num cpu.
func (c *GigaCache) Scan(f Walker, numCPU ...int) {
	cpu := 1
	if len(numCPU) > 0 {
		cpu = numCPU[0]
	}
	pool := pool.New().WithMaxGoroutines(cpu)

	for _, b := range c.buckets {
		b := b
		pool.Go(func() {
			b.RLock()
			b.scan(func(key, val []byte, ts int64) bool {
				return f(key, val, ts)
			})
			b.RUnlock()
		})
	}

	pool.Wait()
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
type OnEvictCallback func(key, val []byte)

// eliminate the expired key-value pairs.
func (b *bucket) eliminate() {
	b.hint = !b.hint
	if b.options.HintEnabled && b.hint {
		return
	}
	var failed int

	// probing
	b.index.Iter(func(key Key, idx Idx) bool {
		b.probe++

		if idx.expired() {
			// evict
			if b.options.OnEvict != nil {
				total, kstr, val := b.find(idx)
				b.options.OnEvict(kstr, val)
				b.inused -= uint64(total)

			} else {
				entry := b.findEntry(idx)
				b.inused -= uint64(len(entry))
			}

			b.index.Delete(key)
			b.evict++
			failed = 0
			return false
		}

		failed++
		return failed >= b.options.MaxFailCount
	})

	// on migrate threshold
	rate := float64(b.inused) / float64(b.alloc)
	delta := b.alloc - b.inused
	if delta >= b.options.MigrateDelta &&
		rate <= b.options.MigrateThresRatio {
		b.migrate()
	}
}

// migrate move valid key-value pairs to the new container to save memory.
func (b *bucket) migrate() {
	// create new data.
	newData := b.bpool.Get(len(b.data))[:0]

	// migrate datas to new bucket.
	b.index.Iter(func(key Key, idx Idx) bool {
		if idx.expired() {
			b.index.Delete(key)
			return false
		}

		entry := b.findEntry(idx)
		// update with new position.
		b.index.Put(key, newIdx(len(newData), idx.TTL()))
		newData = append(newData, entry...)

		return false
	})

	// reuse buffer.
	b.bpool.Put(b.data)

	// replace old data.
	b.data = newData
	b.alloc = uint64(len(b.data))
	b.inused = uint64(len(b.data))
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
		s.Len += uint64(b.index.Count())
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
