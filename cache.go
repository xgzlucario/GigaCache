package cache

import (
	"encoding/binary"
	"math"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/exp/rand"
	"golang.org/x/exp/slices"

	"github.com/bytedance/sonic"
	"github.com/tidwall/hashmap"
	"github.com/zeebo/xxh3"
)

const (
	noTTL   = 0
	expired = -1

	// for ttl
	ttlBytes = 8

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
	// When using LittleEndian, byte slices can be converted to uint64 unsafely.
	order = binary.LittleEndian
)

// GigaCache
type GigaCache[K comparable] struct {
	kstr    bool
	ksize   int
	mask    uint64
	buckets []*bucket[K]
}

// bucket
type bucket[K comparable] struct {
	idx     *hashmap.Map[K, Idx]
	count   int64
	ccount  int64
	byteArr []byte
	anyArr  []*anyItem
	sync.RWMutex
}

// anyItem
type anyItem struct {
	V any
	T int64
}

// New returns a new GigaCache instance.
func New[K comparable](count ...int) *GigaCache[K] {
	var shards = defaultShardsCount
	if len(count) > 0 {
		shards = count[0]
	}

	cache := &GigaCache[K]{
		mask:    uint64(shards - 1),
		buckets: make([]*bucket[K], shards),
	}

	var k K
	switch ((interface{})(k)).(type) {
	case string:
		cache.kstr = true
	default:
		cache.ksize = int(unsafe.Sizeof(k))
	}

	for i := range cache.buckets {
		cache.buckets[i] = &bucket[K]{
			idx:     hashmap.New[K, Idx](0),
			byteArr: make([]byte, 0, bufferSize),
			anyArr:  make([]*anyItem, 0, bufferSize),
		}
	}

	return cache
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

// get
func (b *bucket[K]) get(idx Idx) ([]byte, int64, bool) {
	if idx.IsAny() {
		return nil, 0, false
	}

	start := idx.start()
	end := start + idx.offset()

	if idx.hasTTL() {
		ttl := parseTTL(b.byteArr[end:])

		if ttl < clock {
			return nil, expired, false
		}
		return b.byteArr[start:end], ttl, true
	}

	return b.byteArr[start:end], noTTL, true
}

// getAny
func (b *bucket[K]) getAny(idx Idx) (any, int64, bool) {
	if !idx.IsAny() {
		return nil, 0, false
	}

	n := b.anyArr[idx.start()]

	if idx.hasTTL() {
		if n.T > clock {
			return n.V, n.T, true
		}
		return nil, expired, false
	}
	return n.V, noTTL, true
}

// Get
func (c *GigaCache[K]) Get(key K) ([]byte, int64, bool) {
	b := c.getShard(key)
	b.RLock()
	defer b.RUnlock()

	if idx, ok := b.idx.Get(key); ok {
		return b.get(idx)
	}

	return nil, 0, false
}

// GetAny get value by key.
func (c *GigaCache[K]) GetAny(key K) (any, int64, bool) {
	b := c.getShard(key)
	b.RLock()
	defer b.RUnlock()

	if idx, ok := b.idx.Get(key); ok {
		return b.getAny(idx)
	}

	return nil, 0, false
}

// SetTx
func (c *GigaCache[K]) SetTx(key K, val []byte, ts int64) {
	hasTTL := ts > noTTL
	var ttlInt int
	if hasTTL {
		ttlInt = 1
	}

	b := c.getShard(key)
	b.Lock()
	defer b.Unlock()

	b.eliminate()

	// check if existed
	idx, ok := b.idx.Get(key)
	if ok {
		start, offset := idx.start(), idx.offset()+idx.ttlInt()*ttlBytes

		// update inplace
		if len(val)+ttlInt*ttlBytes <= offset {
			b.idx.Set(key, newIdx(start, len(val), hasTTL, false))
			end := start + len(val)

			b.byteArr = slices.Replace(b.byteArr, start, end, val...)
			if hasTTL {
				order.PutUint64(b.byteArr[end:], uint64(ts))
			}
			return
		}
	}

	b.idx.Set(key, newIdx(len(b.byteArr), len(val), hasTTL, false))
	b.byteArr = append(b.byteArr, val...)
	if hasTTL {
		b.byteArr = order.AppendUint64(b.byteArr, uint64(ts))
	}

	b.count++
}

// Set
func (c *GigaCache[K]) Set(key K, val []byte) {
	c.SetTx(key, val, noTTL)
}

// SetEx
func (c *GigaCache[K]) SetEx(key K, val []byte, dur time.Duration) {
	c.SetTx(key, val, clock+int64(dur))
}

// SetAnyTx
func (c *GigaCache[K]) SetAnyTx(key K, val any, ts int64) {
	hasTTL := ts > 0

	b := c.getShard(key)
	b.Lock()
	defer b.Unlock()

	b.eliminate()

	idx, ok := b.idx.Get(key)
	// exist
	if ok {
		if idx.IsAny() {
			start := idx.start()
			b.anyArr[start].T = ts
			b.anyArr[start].V = val
			b.idx.Set(key, newIdx(start, 0, hasTTL, true))
			return
		}
		b.count--
	}

	b.idx.Set(key, newIdx(len(b.anyArr), 0, hasTTL, true))
	b.anyArr = append(b.anyArr, &anyItem{V: val, T: ts})

	b.count++
}

// SetAny
func (c *GigaCache[K]) SetAny(key K, val any) {
	c.SetAnyTx(key, val, noTTL)
}

// SetAnyEx
func (c *GigaCache[K]) SetAnyEx(key K, val any, dur time.Duration) {
	c.SetAnyTx(key, val, clock+int64(dur))
}

// Delete
func (c *GigaCache[K]) Delete(key K) bool {
	b := c.getShard(key)
	b.Lock()
	defer b.Unlock()

	_, ok := b.idx.Delete(key)
	if ok {
		b.count--
	}

	b.eliminate()

	return ok
}

type ScanType byte

const (
	TypeByte = iota + 1
	TypeAny
)

// Scan
func (c *GigaCache[K]) Scan(f func(K, any, int64) bool) {
	for _, b := range c.buckets {
		b.RLock()
		b.idx.Scan(func(key K, idx Idx) bool {
			if idx.IsAny() {
				val, ts, ok := b.getAny(idx)
				if ok {
					return f(key, val, ts)
				}
			}
			val, ts, ok := b.get(idx)
			if ok {
				return f(key, val, ts)
			}
			return true
		})
		b.RUnlock()
	}
}

func parseTTL(b []byte) int64 {
	_ = b[ttlBytes-1]
	return *(*int64)(unsafe.Pointer(&b[0]))
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
			if idx.IsAny() {
				item := b.anyArr[idx.start()]
				if item.T < clock {
					b.idx.Delete(k)
					failCont = 0
					continue
				}

			} else {
				end := idx.start() + idx.offset()
				ttl := parseTTL(b.byteArr[end:])

				// expired
				if ttl < clock {
					b.idx.Delete(k)
					failCont = 0
					continue
				}
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

// Compress
func (c *GigaCache[K]) Compress() {
	for _, b := range c.buckets {
		b.Lock()
		b.compress(float64(b.idx.Len()) / float64(b.count))
		b.Unlock()
	}
}

// Compress migrates the unexpired data and save memory.
// Trigger when the valid count (valid / total) in the cache is less than this value.
func (b *bucket[K]) compress(rate float64) {
	if math.IsNaN(rate) {
		return
	}
	b.count = 0
	b.ccount++

	newBytesArr := make([]byte, 0, int(float64(len(b.byteArr))*rate))
	newAnyArr := make([]*anyItem, 0, int(float64(len(b.anyArr))*rate))

	delKeys := make([]K, 0)

	b.idx.Scan(func(key K, idx Idx) bool {
		start, has := idx.start(), idx.hasTTL()

		if idx.IsAny() {
			item := b.anyArr[start]

			// expired
			if has && item.T < clock {
				delKeys = append(delKeys, key)
				return true
			}

			b.idx.Set(key, newIdx(len(newAnyArr), 0, has, true))
			newAnyArr = append(newAnyArr, item)

			b.count++
			return true

		} else {
			offset := idx.offset()
			end := start + offset

			// expired
			if has && parseTTL(b.byteArr[end:]) < clock {
				delKeys = append(delKeys, key)
				return true
			}

			// reset
			b.idx.Set(key, newIdx(len(newBytesArr), offset, has, false))
			if has {
				newBytesArr = append(newBytesArr, b.byteArr[start:end+ttlBytes]...)
			} else {
				newBytesArr = append(newBytesArr, b.byteArr[start:end]...)
			}

			b.count++
			return true
		}
	})

	for _, key := range delKeys {
		b.idx.Delete(key)
	}

	b.byteArr = newBytesArr
	b.anyArr = newAnyArr
}

type bucketJSON[K comparable] struct {
	C int64
	K []K
	I []Idx
	B []byte
}

// MarshalBytes
func (c *GigaCache[K]) MarshalBytes() ([]byte, error) {
	buckets := make([]*bucketJSON[K], 0, len(c.buckets))

	for _, b := range c.buckets {
		b.RLock()
		defer b.RUnlock()

		k := make([]K, 0, b.idx.Len())
		i := make([]Idx, 0, b.idx.Len())

		b.idx.Scan(func(key K, idx Idx) bool {
			if !idx.IsAny() {
				k = append(k, key)
				i = append(i, idx)
			}
			return true
		})

		buckets = append(buckets, &bucketJSON[K]{
			b.count, k, i, b.byteArr,
		})
	}

	return sonic.Marshal(buckets)
}

// UnmarshalBytes
func (c *GigaCache[K]) UnmarshalBytes(src []byte) error {
	var buckets []*bucketJSON[K]

	if err := sonic.Unmarshal(src, &buckets); err != nil {
		return err
	}

	c.buckets = make([]*bucket[K], 0, len(buckets))
	for _, b := range buckets {
		bc := &bucket[K]{
			count:   b.C,
			idx:     hashmap.New[K, Idx](len(b.K)),
			byteArr: b.B,
			anyArr:  make([]*anyItem, 0),
		}
		// set key
		for i, k := range b.K {
			bc.idx.Set(k, b.I[i])
		}

		c.buckets = append(c.buckets, bc)
	}

	return nil
}
