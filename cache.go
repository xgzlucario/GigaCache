package cache

import (
	"encoding/binary"
	"math"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/exp/rand"
	"golang.org/x/exp/slices"

	"github.com/bytedance/sonic"
	"github.com/zeebo/xxh3"
)

const (
	startBits  = 32
	offsetBits = 31

	// for ttl
	ttlBytes  = 4
	timeCarry = 1e9 // Second

	offsetMask = math.MaxUint32

	bufferSize         = 1024
	defaultShardsCount = 1024

	// eliminate probing
	probeInterval     = 3
	probeCount        = 100
	probeSpace        = 3
	compressThreshold = 0.5
	maxFailCount      = 5
)

var (
	zeroUnix     int64
	zeroUnixNano int64

	// When using LittleEndian, byte slices can be converted to uint64 unsafely.
	order = binary.LittleEndian

	// now timer and offset clock since zeroTime
	clock uint32
)

func init() {
	zt, _ := time.Parse(time.DateOnly, "2023-07-01")
	zeroUnix = zt.Unix()
	zeroUnixNano = zt.UnixNano()
	clock = uint32(time.Now().Unix() - zeroUnix)

	go func() {
		ticker := time.NewTicker(time.Millisecond)
		for t := range ticker.C {
			atomic.StoreUint32(&clock, uint32(t.Unix()-zeroUnix))
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

func (i Idx) hasTTLInt() int {
	return int(i & 1)
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

// New returns a new GigaCache instance.
func New[K comparable](count ...int) *GigaCache[K] {
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
	defer b.Unlock()

	b.eliminate()

	// check if existed
	idx, ok := b.idx.Get(key)
	if ok {
		start := idx.start()
		offset := idx.offset() + idx.hasTTLInt()*ttlBytes

		if len(val) <= offset {
			b.idx.Set(key, newIdx(start, len(val), false))
			b.buf = slices.Replace(b.buf, start, start+len(val), val...)
			return
		}
	}

	b.idx.Set(key, newIdx(len(b.buf), len(val), false))
	b.buf = append(b.buf, val...)
	b.count++
}

// SetEx set expiry time with key-value pairs.
// dur should be unixnano.
func (c *GigaCache[K]) SetEx(key K, val []byte, dur time.Duration) {
	if dur < timeCarry {
		panic("dur must be greater than 1s")
	}

	b := c.getShard(key)
	b.Lock()
	defer b.Unlock()
	defer b.eliminate()

	// check if existed
	idx, ok := b.idx.Get(key)
	if ok {
		start := idx.start()
		offset := idx.offset() + idx.hasTTLInt()*ttlBytes

		// update inplace
		if len(val)+ttlBytes <= offset {
			b.idx.Set(key, newIdx(start, len(val), true))
			end := start + len(val)

			b.buf = slices.Replace(b.buf, start, end, val...)
			order.PutUint32(b.buf[end:], clock+uint32(dur/timeCarry))
			return
		}
	}

	b.idx.Set(key, newIdx(len(b.buf), len(val), true))
	b.buf = append(b.buf, val...)
	b.buf = order.AppendUint32(b.buf, clock+uint32(dur/timeCarry))

	b.count++
}

// SetTx set deadline with key-value pairs.
// ts should be unixnano.
func (c *GigaCache[K]) SetTx(key K, val []byte, ts int64) {
	c.SetEx(key, val, time.Duration(ts-zeroUnixNano))
}

// Get
func (c *GigaCache[K]) Get(key K) ([]byte, bool) {
	val, _, ok := c.GetTx(key)
	return val, ok
}

// getByIdx
func (b *bucket[K]) getByIdx(idx Idx) ([]byte, int64, bool) {
	start := idx.start()
	end := start + idx.offset()

	// has ttl
	if idx.hasTTL() {
		ttl := parseTTL(b.buf[end:])

		// expired
		if ttl < clock {
			return nil, 0, false

		} else {
			return b.buf[start:end], (zeroUnix + int64(ttl)) * timeCarry, true
		}
	}

	return b.buf[start:end], -1, true
}

// GetTx
func (c *GigaCache[K]) GetTx(key K) ([]byte, int64, bool) {
	b := c.getShard(key)
	b.RLock()
	defer b.RUnlock()

	if idx, ok := b.idx.Get(key); ok {
		return b.getByIdx(idx)
	}

	return nil, 0, false
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

// Scan
func (c *GigaCache[K]) Scan(f func(K, []byte, int64)) {
	for _, b := range c.buckets {
		b.RLock()
		b.idx.Scan(func(key K, idx Idx) {
			val, ts, ok := b.getByIdx(idx)
			if ok {
				f(key, val, ts)
			}
		})
		b.RUnlock()
	}
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

func (c *GigaCache[K]) bytesLen() (r int) {
	for _, b := range c.buckets {
		b.RLock()
		r += len(b.buf)
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
// Trigger when the valid count (valid / total) in the cache is less than this value.
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

	for i, b := range c.buckets {
		b.RLock()
		src, _ := b.MarshalJSON()
		buf = append(buf, src...)
		b.RUnlock()

		if i < len(c.buckets)-1 {
			buf = append(buf, '\n')
		}
	}

	return buf, nil
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
