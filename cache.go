package cache

import (
	"bytes"
	"encoding/binary"
	"github.com/cockroachdb/swiss"
	"runtime"
	"slices"
	"sync"
	"time"
	"unsafe"

	"github.com/sourcegraph/conc/pool"
)

const (
	noTTL = 0

	maxKeySize = 1024

	// maxFailCount indicates that the evict algorithm break
	// when consecutive unexpired key-value pairs are detected.
	maxFailCount = 3
)

// GigaCache implements a key-value cache.
type GigaCache struct {
	mask    uint32
	hashFn  HashFn
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

	// runtime stats.
	interval int
	unused   uint64
	migrates uint64
	evict    uint64
	probe    uint64
}

type item struct {
	val []byte
	ttl uint32
}

// New returns new GigaCache instance.
func New(options Options) *GigaCache {
	if err := checkOptions(options); err != nil {
		panic(err)
	}

	// create cache.
	cache := &GigaCache{
		mask:    options.ShardCount - 1,
		hashFn:  options.HashFn,
		buckets: make([]*bucket, options.ShardCount),
	}
	bpool := NewBufferPool()

	// init buckets.
	for i := range cache.buckets {
		cache.buckets[i] = &bucket{
			options: &options,
			bpool:   bpool,
			index:   swiss.New[Key, Idx](options.IndexSize),
			data:    bpool.Get(options.BufferSize)[:0],
		}
	}
	return cache
}

// getShard returns the bucket and the real key by hash(kstr).
// sharding and index use different hash function, can reduce the hash conflicts greatly.
func (c *GigaCache) getShard(kstr string) (*bucket, Key) {
	hash := c.hashFn(kstr)
	hash32 := uint32(hash >> 1)
	return c.buckets[hash32&c.mask], newKey(hash)
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
func (b *bucket) set(key Key, kstr, val []byte, ts int64) {
	if len(kstr) > maxKeySize {
		panic("key size is too large")
	}

	// check key exist in index.
	if idx, ok := b.index.Get(key); ok {
		total, key, _ := b.find(idx)
		// same as index
		if bytes.Equal(key, kstr) {
			b.unused += uint64(total)

		} else {
			if b.options.OnHashConflict != nil {
				// key is origin string, kstr is conflict key.
				b.options.OnHashConflict(key, kstr)
			}
			return
		}
	}

	// set index.
	b.index.Put(key, newIdx(len(b.data), ts))

	// append klen, vlen, key, val.
	b.data = binary.AppendUvarint(b.data, uint64(len(kstr)))
	b.data = binary.AppendUvarint(b.data, uint64(len(val)))
	b.data = append(b.data, kstr...)
	b.data = append(b.data, val...)
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
	idx, ok := b.index.Get(key)
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
		b.unused += uint64(total)

	} else {
		entry := b.findEntry(idx)
		b.unused += uint64(len(entry))
	}
	b.index.Delete(key)
}

// SetTTL set ttl for key.
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
type Walker func(key, val []byte, ttl int64) (next bool)

func (b *bucket) scan(f Walker) (next bool) {
	b.index.All(func(_ Key, idx Idx) bool {
		if idx.expired() {
			return true
		}
		_, kstr, val := b.find(idx)
		next = f(kstr, val, idx.TTL())
		return next
	})
	return
}

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

// Callback is the callback function.
// DO NOT EDIT the input params.
type Callback func(key, val []byte)

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
	b.index.All(func(key Key, idx Idx) bool {
		b.probe++
		if idx.expired() {
			b.remove(key, idx)
			b.evict++
			failed = 0
			return true
		}

		failed++
		return failed <= maxFailCount
	})

	b.migrate()
}

// migrate move valid key-value pairs to the new container to save memory.
func (b *bucket) migrate() {
	// check need to migrate.
	rate := float64(b.unused) / float64(len(b.data))
	if b.unused >= b.options.MigrateDelta && rate >= b.options.MigrateRatio {
	} else {
		return
	}

	// create new data.
	newData := b.bpool.Get(len(b.data))[:0]

	// migrate data to new bucket.
	b.index.All(func(key Key, idx Idx) bool {
		if idx.expired() {
			b.index.Delete(key)
			return true
		}
		entry := b.findEntry(idx)
		// update with new position.
		b.index.Put(key, newIdx(len(newData), idx.TTL()))
		newData = append(newData, entry...)
		return true
	})

	b.bpool.Put(b.data)
	b.data = newData
	b.unused = 0
	b.migrates++
}

// Stat is the runtime statistics of GigaCache.
type Stat struct {
	Len      int
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

func s2b(str *string) []byte {
	strHeader := (*[2]uintptr)(unsafe.Pointer(str))
	byteSliceHeader := [3]uintptr{
		strHeader[0], strHeader[1], strHeader[1],
	}
	return *(*[]byte)(unsafe.Pointer(&byteSliceHeader))
}
