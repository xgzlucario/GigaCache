package cache

import (
	"bytes"
	"encoding/binary"
	"runtime"
	"slices"
	"sync"
	"time"
	"unsafe"

	"github.com/sourcegraph/conc/pool"
	"github.com/zeebo/xxh3"
)

const (
	noTTL = 0

	// maxFailCount indicates that the evict algorithm break
	// when consecutive unexpired key-value pairs are detected.
	maxFailCount = 3
)

// GigaCache implements a key-value cache.
type GigaCache struct {
	mask    uint32
	buckets []*bucket
}

// bucket is the data container for GigaCache.
type bucket struct {
	sync.RWMutex
	options *Options
	bpool   *BufferPool

	// index is the index map for cache, mapped hash(kstr) to the position that data real stored.
	index map[Key]Idx

	// data store all key-value bytes data.
	data []byte

	// runtime stats.
	interval int
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
		mask:    uint32(options.ShardCount - 1),
		buckets: make([]*bucket, options.ShardCount),
	}
	bpool := NewBufferPool()

	// init buckets.
	for i := range cache.buckets {
		cache.buckets[i] = &bucket{
			options: &options,
			bpool:   bpool,
			index:   make(map[Key]Idx, options.IndexSize),
			data:    bpool.Get(options.BufferSize)[:0],
		}
	}
	return cache
}

// getShard returns the bucket and the real key by hash(kstr).
// sharding and index use different hash function, can reduce the hash conflicts greatly.
func (c *GigaCache) getShard(kstr string) (*bucket, Key) {
	hashShard := MemHashString(kstr)
	hashKey := xxh3.HashString(kstr)
	return c.buckets[hashShard&c.mask], newKey(hashKey)
}

func (b *bucket) find(idx Idx) (total int, kstr, val []byte) {
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
	idx, ok := b.index[key]
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
func (b *bucket) set(key Key, kstr, val []byte, ts int64) {
	// check key if already exists.
	if idx, ok := b.index[key]; ok {
		total, key, _ := b.find(idx)
		if bytes.Equal(key, kstr) {
			// update stat.
			b.alloc -= uint64(total)
			b.inused -= uint64(total)
		}
	}

	// set index.
	b.index[key] = newIdx(len(b.data), ts)
	before := len(b.data)

	// append klen, vlen, key, val.
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

// Remove removes the key-value pair by the key.
func (c *GigaCache) Remove(kstr string) bool {
	b, key := c.getShard(kstr)
	b.Lock()
	b.eliminate()

	// find index map.
	idx, ok := b.index[key]
	if !ok || idx.expired() {
		b.Unlock()
		return false
	}
	b.remove(key, idx)
	b.Unlock()
	return true
}

func (b *bucket) remove(key Key, idx Idx) {
	if b.options.OnRemove != nil {
		total, kstr, val := b.find(idx)
		b.options.OnRemove(kstr, val)
		b.inused -= uint64(total)

	} else {
		entry := b.findEntry(idx)
		b.inused -= uint64(len(entry))
	}
	delete(b.index, key)
}

// SetTTL
func (c *GigaCache) SetTTL(kstr string, ts int64) bool {
	b, key := c.getShard(kstr)
	b.Lock()
	b.eliminate()

	// find index map.
	idx, ok := b.index[key]
	if !ok || idx.expired() {
		b.Unlock()
		return false
	}
	// update index.
	b.index[key] = newIdx(idx.start(), ts)
	b.Unlock()
	return true
}

// Walker is the callback function for iterator.
type Walker func(key, val []byte, ttl int64) (stop bool)

func (b *bucket) scan(f Walker) (stop bool) {
	for _, idx := range b.index {
		if idx.expired() {
			continue
		}
		_, kstr, val := b.find(idx)
		if f(kstr, val, idx.TTL()) {
			return true
		}
	}
	return false
}

// Scan walk all alive key-value pairs.
// DO NOT EDIT the bytes as they are NO COPY.
func (c *GigaCache) Scan(f Walker) {
	for _, b := range c.buckets {
		b.RLock()
		stop := b.scan(f)
		b.RUnlock()
		if stop {
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
		pool := pool.New().WithMaxGoroutines(cpu)
		for _, b := range c.buckets {
			b := b
			pool.Go(func() {
				b.Lock()
				b.migrate()
				b.Unlock()
			})
		}
		pool.Wait()
	}
}

// OnRemove called when a key-value pair is evicted.
// DO NOT EDIT the input params.
type OnRemove func(key, val []byte)

// eliminate the expired key-value pairs.
func (b *bucket) eliminate() {
	if b.options.DisableEvict {
		b.migrate()
		return
	}

	b.interval++
	if b.interval < b.options.EvictInterval {
		return
	}
	b.interval = 0
	var failed int

	// probing
	for key, idx := range b.index {
		b.probe++
		if idx.expired() {
			b.remove(key, idx)
			b.evict++
			failed = 0
			continue
		}
		failed++
		if failed >= maxFailCount {
			break
		}
	}

	b.migrate()
}

// migrate move valid key-value pairs to the new container to save memory.
func (b *bucket) migrate() {
	// check if need to migrate.
	rate := float64(b.inused) / float64(b.alloc)
	delta := b.alloc - b.inused
	if delta >= b.options.MigrateDelta &&
		rate <= b.options.MigrateThresRatio {
	} else {
		return
	}

	// create new data.
	newData := b.bpool.Get(len(b.data))[:0]

	// migrate datas to new bucket.
	for key, idx := range b.index {
		if idx.expired() {
			delete(b.index, key)
			continue
		}
		entry := b.findEntry(idx)
		// update with new position.
		b.index[key] = newIdx(len(newData), idx.TTL())
		newData = append(newData, entry...)
	}

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
		s.Len += uint64(len(b.index))
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
