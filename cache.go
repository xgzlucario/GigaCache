package cache

import (
	"encoding/binary"
	"math"
	"math/rand"
	"sync"
	"time"
	"unsafe"

	"github.com/bytedance/sonic"
	"github.com/klauspost/compress/s2"
	"github.com/zeebo/xxh3"
)

const (
	startBits  = 32
	offsetBits = 31
	ttlBits    = 8
	offsetMask = math.MaxUint32

	noTTL             = 0
	probeCount        = 100
	compressThreshold = 0.5

	bufferSize         = 1024
	defaultShardsCount = 1024
)

var (
	// When using LittleEndian, byte slices can be converted to uint64 unsafely.
	order = binary.LittleEndian

	// Global clock
	globalClock = time.Now().UnixNano()
)

func init() {
	go func() {
		ticker := time.NewTicker(time.Millisecond)
		for t := range ticker.C {
			globalClock = t.UnixNano()
		}
	}()
}

// Idx is the index of BigCahce.
// start(32)|offset(31)|hasTTL(1)
type Idx uint64

func (i Idx) start() int {
	return int(i >> startBits)
}

func (i Idx) offset() int {
	return int((i & offsetMask) >> 1)
}

func (i Idx) hasTTL() bool {
	return i&1 == 1
}

func newIdx(start, offset int, hasTTL bool) Idx {
	// bound check
	if start > math.MaxUint32 || offset > math.MaxUint32>>1 {
		panic("index overflow")
	}

	idx := Idx(start<<startBits | offset<<1)
	if hasTTL {
		idx |= 1
	}
	return idx
}

// GigaCache
type GigaCache[K comparable] struct {
	kstr    bool
	ksize   int
	mask    uint64
	buckets []*bucket[K]
}

// bucket
type bucket[K comparable] struct {
	count    float64
	expCount float64
	buf      []byte
	idx      *Map[K, Idx]
	sync.RWMutex
}

// NewGigaCache returns a new GigaCache.
func NewGigaCache[K comparable]() *GigaCache[K] {
	return newCache[K](defaultShardsCount)
}

// NewExtGigaCache returns a new GigaCache with shards specified.
func NewExtGigaCache[K comparable](shards int) *GigaCache[K] {
	return newCache[K](shards)
}

func newCache[K comparable](shards int) *GigaCache[K] {
	cc := &GigaCache[K]{
		mask:    uint64(shards - 1),
		buckets: make([]*bucket[K], shards),
	}
	cc.detectHasher()

	for i := range cc.buckets {
		cc.buckets[i] = &bucket[K]{
			idx: New[K, Idx](0),
			buf: make([]byte, 0, bufferSize),
		}
	}

	return cc
}

// detectHasher Detect the key type.
func (c *GigaCache[K]) detectHasher() {
	var k K
	switch ((interface{})(k)).(type) {
	case string:
		c.kstr = true
	default:
		c.ksize = int(unsafe.Sizeof(k))
	}
}

// Hash is the real hash function.
func (c *GigaCache[K]) hash(key K) uint64 {
	var strKey string
	if c.kstr {
		strKey = *(*string)(unsafe.Pointer(&key))
	} else {
		strKey = *(*string)(unsafe.Pointer(&struct {
			data unsafe.Pointer
			len  int
		}{unsafe.Pointer(&key), c.ksize}))
	}

	return xxh3.HashString(strKey) & c.mask
}

// getShard returns the bucket of the key.
func (c *GigaCache[K]) getShard(key K) *bucket[K] {
	return c.buckets[c.hash(key)]
}

// Set set key-value pairs.
func (c *GigaCache[K]) Set(key K, val []byte) {
	b := c.getShard(key)
	b.Lock()
	defer b.Unlock()

	b.eliminate()

	b.idx.Set(key, newIdx(len(b.buf), len(val), false))
	b.buf = append(b.buf, val...)

	b.count++
}

// SetEx set expiry time with key-value pairs.
func (c *GigaCache[K]) SetEx(key K, val []byte, dur time.Duration) {
	c.SetTx(key, val, globalClock+int64(dur))
}

// SetTx set deadline with key-value pairs.
func (c *GigaCache[K]) SetTx(key K, val []byte, ts int64) {
	b := c.getShard(key)
	b.Lock()
	defer b.Unlock()

	b.eliminate()

	b.idx.Set(key, newIdx(len(b.buf), len(val), true))
	b.buf = append(b.buf, val...)
	b.buf = order.AppendUint64(b.buf, uint64(ts))

	b.count++
	b.expCount++
}

// Get
func (c *GigaCache[K]) Get(key K) ([]byte, bool) {
	val, _, ok := c.GetTx(key)
	return val, ok
}

// GetTx
func (c *GigaCache[K]) GetTx(key K) ([]byte, int64, bool) {
	b := c.getShard(key)
	b.RLock()

	if idx, ok := b.idx.Get(key); ok {
		start := idx.start()
		end := start + idx.offset()

		// has ttl
		if idx.hasTTL() {
			ttl := int64(*(*uint64)(unsafe.Pointer(&b.buf[end])))

			// not expired
			if b.timeAlive(ttl) {
				b.RUnlock()
				return b.buf[start:end], ttl, true

			} else {
				// delete
				b.RUnlock()
				b.Lock()
				b.idx.Delete(key)
				b.Unlock()
				return nil, -1, false
			}

		} else {
			b.RUnlock()
			return b.buf[start:end], noTTL, true
		}
	}

	b.RUnlock()
	return nil, -1, false
}

// Delete
func (c *GigaCache[K]) Delete(key K) bool {
	b := c.getShard(key)
	b.Lock()
	defer b.Unlock()

	b.eliminate()

	_, ok := b.idx.Delete(key)
	return ok
}

// Len returns keys length. It returns not an exact value, it may contain expired keys.
func (c *GigaCache[K]) Len() (r int) {
	for _, b := range c.buckets {
		b.RLock()
		r += b.idx.Len()
		b.RUnlock()
	}
	return
}

func (b *bucket[K]) timeAlive(ttl int64) bool {
	return ttl > globalClock || ttl == noTTL
}

// eliminate the expired key-value pairs.
func (b *bucket[K]) eliminate() {
	if b.expCount == 0 {
		return
	}

	var failCont int
	rdm := rand.Uint64()

	// probing
	for i := uint64(0); i < probeCount; i++ {
		k, idx, ok := b.idx.GetPos(rdm + i*3)

		if ok && idx.hasTTL() {
			end := idx.start() + idx.offset()
			ttl := int64(*(*uint64)(unsafe.Pointer(&b.buf[end])))

			// expired
			if !b.timeAlive(ttl) {
				b.idx.Delete(k)
				b.expCount--
				failCont = 0
				continue
			}
		}

		failCont++
		if failCont > 2 {
			break
		}
	}

	// on compress threshold
	if float64(b.idx.Len())/b.count < compressThreshold {
		b.compress()
	}
}

// Compress migrates the unexpired data and save memory.
// Trigger when the valid count (valid / total) in the cache is less than this value
func (b *bucket[K]) compress() {
	b.count = 0
	b.expCount = 0

	length := float64(len(b.buf)) * compressThreshold
	nbuf := make([]byte, 0, int(length))

	delKeys := make([]K, 0)

	b.idx.Scan(func(key K, idx Idx) {
		// offset only contains value, except ttl
		start, offset, has := idx.start(), idx.offset(), idx.hasTTL()

		if has {
			ttl := int64(*(*uint64)(unsafe.Pointer(&b.buf[start+offset])))
			if !b.timeAlive(ttl) {
				delKeys = append(delKeys, key)
				return
			}
		}

		// reset
		b.idx.Set(key, newIdx(len(nbuf), offset, has))
		if has {
			nbuf = append(nbuf, b.buf[start:start+offset+ttlBits]...)
			b.expCount++

		} else {
			nbuf = append(nbuf, b.buf[start:start+offset]...)
		}

		b.count++
	})

	for _, key := range delKeys {
		b.idx.Delete(key)
	}

	b.buf = nbuf
}

// MarshalJSON
func (c *GigaCache[K]) MarshalJSON() ([]byte, error) {
	plen := len(c.buckets[0].buf) * len(c.buckets)

	buf := make([]byte, 0, plen)

	buf = append(buf, '[')

	for i, b := range c.buckets {
		buf = append(buf, '"')

		b.RLock()
		src, _ := b.MarshalJSON()
		buf = append(buf, src...)
		b.RUnlock()

		buf = append(buf, '"')
		if i != len(c.buckets)-1 {
			buf = append(buf, ',')
		}
	}

	buf = append(buf, ']')

	return s2.EncodeSnappy(nil, buf), nil
}

type bucketJSON[K comparable] struct {
	K []K
	I []Idx
	B []byte
}

func (b *bucket[K]) MarshalJSON() ([]byte, error) {
	k := make([]K, 0, b.idx.Len())
	i := make([]Idx, 0, b.idx.Len())

	b.idx.Scan(func(key K, idx Idx) {
		k = append(k, key)
		i = append(i, idx)
	})

	return sonic.Marshal(bucketJSON[K]{k, i, b.buf})
}
