package cache

import (
	"encoding/binary"
	"math"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/exp/rand"

	"github.com/bytedance/sonic"
	"github.com/klauspost/compress/s2"
	"github.com/zeebo/xxh3"
)

const (
	startBits  = 32
	offsetBits = 31

	// for ttl
	ttlBytes  = 4
	timeCarry = time.Second

	offsetMask = math.MaxUint32

	bufferSize         = 1024
	defaultShardsCount = 1024

	// eliminate probing
	probeInterval     = 2
	probeCount        = 100
	probeSpace        = 3
	compressThreshold = 0.5
	maxFailCount      = 3
)

var (
	zeroTime, _ = time.Parse(time.DateOnly, "2023-07-01")

	// When using LittleEndian, byte slices can be converted to uint64 unsafely.
	order = binary.LittleEndian

	// now timer and offset clock since zeroTime
	now   = time.Now()
	clock = uint32(time.Since(zeroTime).Seconds())
)

func init() {
	go func() {
		ticker := time.NewTicker(time.Millisecond)
		for t := range ticker.C {
			now = t
			clock = uint32(t.Sub(zeroTime).Seconds())
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
	count uint32
	buf   []byte
	idx   *Map[K, Idx]
	sync.RWMutex
}

// NewGigaCache returns a new GigaCache.
func NewGigaCache[K comparable](count ...int) *GigaCache[K] {
	var shards = defaultShardsCount
	if len(count) > 0 {
		shards = count[0]
	}

	cc := &GigaCache[K]{
		mask:    uint64(shards - 1),
		buckets: make([]*bucket[K], shards),
	}
	cc.detectHasher()

	for i := range cc.buckets {
		cc.buckets[i] = &bucket[K]{
			idx: newMap[K, Idx](0),
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
	b.eliminate()

	b.idx.Set(key, newIdx(len(b.buf), len(val), false))
	b.buf = append(b.buf, val...)

	b.count++
	b.Unlock()
}

// SetEx set expiry time with key-value pairs.
func (c *GigaCache[K]) SetEx(key K, val []byte, dur time.Duration) {
	if dur < timeCarry {
		panic("ttl must be greater than 1s")
	}

	b := c.getShard(key)
	b.Lock()
	b.eliminate()

	b.idx.Set(key, newIdx(len(b.buf), len(val), true))
	b.buf = append(b.buf, val...)
	// ttl
	b.buf = order.AppendUint32(b.buf, clock+uint32(dur/timeCarry))

	b.count++
	b.Unlock()
}

// SetTx set deadline with key-value pairs.
func (c *GigaCache[K]) SetTx(key K, val []byte, ts time.Time) {
	c.SetEx(key, val, ts.Sub(now))
}

// Get
func (c *GigaCache[K]) Get(key K) ([]byte, bool) {
	val, _, ok := c.GetTx(key)
	return val, ok
}

// GetTx
func (c *GigaCache[K]) GetTx(key K) ([]byte, time.Time, bool) {
	b := c.getShard(key)
	b.RLock()
	defer b.RUnlock()

	if idx, ok := b.idx.Get(key); ok {
		start := idx.start()
		end := start + idx.offset()

		// has ttl
		if idx.hasTTL() {
			ttl := parseTTL(b.buf[end:])

			// expired
			if ttl < clock {
				return nil, time.Time{}, false

			} else {
				return b.buf[start:end], zeroTime.Add(timeCarry * time.Duration(ttl)), true
			}

		} else {
			return b.buf[start:end], time.Time{}, true
		}
	}

	return nil, time.Time{}, false
}

// Delete
func (c *GigaCache[K]) Delete(key K) (ok bool) {
	b := c.getShard(key)
	b.Lock()
	_, ok = b.idx.Delete(key)
	b.eliminate()
	b.Unlock()
	return
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

func parseTTL(b []byte) uint32 {
	_ = b[ttlBytes-1]
	return *(*uint32)(unsafe.Pointer(&b[0]))
}

// eliminate the expired key-value pairs.
func (b *bucket[K]) eliminate() {
	if b.count%probeInterval != 0 {
		return
	}

	var failCont int
	rdm := rand.Uint64()

	// probing
	for i := uint64(0); i < probeCount; i++ {
		k, idx, ok := b.idx.GetPos(rdm + i*probeSpace)

		if ok && idx.hasTTL() {
			end := idx.start() + idx.offset()
			ttl := parseTTL(b.buf[end:])

			// expired
			if ttl < clock {
				b.idx.Delete(k)
				failCont = 0
				continue
			}
		}

		failCont++
		if failCont > maxFailCount {
			break
		}
	}

	// on compress threshold
	if rate := float64(b.idx.Len()) / float64(b.count); rate < compressThreshold {
		b.compress(rate)
	}
}

// Compress migrates the unexpired data and save memory.
// Trigger when the valid count (valid / total) in the cache is less than this value
func (b *bucket[K]) compress(rate float64) {
	b.count = 0

	newCap := float64(len(b.buf)) * rate
	nbuf := make([]byte, 0, int(newCap))

	delKeys := make([]K, 0)

	b.idx.Scan(func(key K, idx Idx) {
		// offset only contains value, except ttl
		start, offset, has := idx.start(), idx.offset(), idx.hasTTL()
		end := start + offset

		if has {
			ttl := parseTTL(b.buf[end:])

			// expired
			if ttl < clock {
				delKeys = append(delKeys, key)
				return
			}
		}

		// reset
		b.idx.Set(key, newIdx(len(nbuf), offset, has))
		if has {
			nbuf = append(nbuf, b.buf[start:end+ttlBytes]...)
		} else {
			nbuf = append(nbuf, b.buf[start:end]...)
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
