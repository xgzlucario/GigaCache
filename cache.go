package cache

import (
	"encoding/binary"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/exp/rand"
	"golang.org/x/exp/slices"

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
	idx       *hashmap.Map[K, Idx]
	byteCount uint32
	anyCount  uint32
	byteArr   []byte
	anyArr    []anyItem
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

	cc := &GigaCache[K]{
		mask:    uint64(shards - 1),
		buckets: make([]*bucket[K], shards),
	}
	cc.detectHasher()

	for i := range cc.buckets {
		cc.buckets[i] = &bucket[K]{
			idx:     hashmap.New[K, Idx](0),
			byteArr: make([]byte, 0, bufferSize),
			anyArr:  make([]anyItem, 0, bufferSize),
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

// getByIdx
func (b *bucket[K]) getByIdx(idx Idx) ([]byte, int64, bool) {
	start := idx.start()
	end := start + idx.offset()

	// has ttl
	if idx.hasTTL() {
		ttl := parseTTL(b.byteArr[end:])

		// expired
		if ttl < clock {
			return nil, expired, false

		} else {
			return b.byteArr[start:end], ttl, true
		}
	}

	return b.byteArr[start:end], noTTL, true
}

// Get
func (c *GigaCache[K]) Get(key K) ([]byte, int64, bool) {
	b := c.getShard(key)
	b.RLock()
	defer b.RUnlock()

	if idx, ok := b.idx.Get(key); ok {
		return b.getByIdx(idx)
	}

	return nil, 0, false
}

// GetAny get value by key.
func (c *GigaCache[K]) GetAny(key K) (any, int64, bool) {
	b := c.getShard(key)
	b.RLock()
	defer b.RUnlock()

	idx, ok := b.idx.Get(key)
	if !ok {
		return nil, 0, false
	}

	if idx.isAny() {
		item := b.anyArr[idx.start()]

		if idx.hasTTL() {
			// is expired
			if item.T > clock {
				return item.V, item.T, true

			} else {
				return nil, expired, false
			}

		} else {
			return item.V, noTTL, true
		}
	}

	return nil, 0, false
}

// Set set bytes value with key-value pairs.
func (c *GigaCache[K]) Set(key K, val []byte, dur ...time.Duration) {
	d := sum(dur)
	hasTTL := len(dur) > 0

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
				order.PutUint64(b.byteArr[end:], uint64(clock)+uint64(d))
			}
			return
		}
	}

	b.idx.Set(key, newIdx(len(b.byteArr), len(val), hasTTL, false))
	b.byteArr = append(b.byteArr, val...)
	if hasTTL {
		b.byteArr = order.AppendUint64(b.byteArr, uint64(clock)+uint64(d))
	}

	b.byteCount++
}

// SetAny set any value with key-value pairs.
func (c *GigaCache[K]) SetAny(key K, val any, dur ...time.Duration) {
	d := sum(dur)
	hasTTL := d > 0

	b := c.getShard(key)
	b.Lock()
	defer b.Unlock()

	b.eliminate()

	// create item
	item := anyItem{V: val, T: noTTL}
	if hasTTL {
		item.T = clock + int64(d)
	}

	// check if existed
	idx, ok := b.idx.Get(key)
	if ok {
		if idx.isAny() {
			b.anyArr[idx.start()] = item
			return

		} else {
			b.idx.Delete(key)
			b.byteCount--
		}
	}

	b.idx.Set(key, newIdx(len(b.anyArr), 0, hasTTL, true))
	b.anyArr = append(b.anyArr, item)

	b.anyCount++
}

// SetDeadline set with key-value pairs. ts should be unixnano.
func (c *GigaCache[K]) SetDeadline(key K, val []byte, ts int64) {
	c.Set(key, val, time.Duration(ts-clock))
}

// Delete
func (c *GigaCache[K]) Delete(key K) bool {
	b := c.getShard(key)
	b.Lock()
	defer b.Unlock()

	idx, ok := b.idx.Delete(key)
	if ok {
		if idx.isAny() {
			b.anyCount--
		} else {
			b.byteCount--
		}
	}

	b.eliminate()

	return ok
}

// Scan
func (c *GigaCache[K]) Scan(f func(K, any, int64) bool) {
	for _, b := range c.buckets {
		b.RLock()
		b.idx.Scan(func(key K, idx Idx) bool {
			if idx.isAny() {
				val := b.anyArr[idx.start()]
				if val.T > clock {
					return f(key, val.V, val.T)
				}

			} else {
				val, ts, ok := b.getByIdx(idx)
				if ok && ts != expired {
					return f(key, val, ts)
				}
			}
			return true
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
		r += len(b.byteArr)
		b.RUnlock()
	}
	return
}

func parseTTL(b []byte) int64 {
	_ = b[ttlBytes-1]
	return *(*int64)(unsafe.Pointer(&b[0]))
}

// eliminate the expired key-value pairs.
func (b *bucket[K]) eliminate() {
	if b.byteCount%probeInterval != 0 {
		return
	}

	var failCont int
	rdm := rand.Uint64()

	// probing
	for i := uint64(0); i < probeCount; i++ {
		k, idx, ok := b.idx.GetPos(rdm + i*probeSpace)

		if ok && idx.hasTTL() {
			end := idx.start() + idx.offset()
			ttl := parseTTL(b.byteArr[end:])

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
	if rate := float64(b.idx.Len()) / float64(b.byteCount); rate < compressThreshold {
		b.compress(rate)
	}
}

// Compress migrates the unexpired data and save memory.
// Trigger when the valid count (valid / total) in the cache is less than this value.
func (b *bucket[K]) compress(rate float64) {
	b.byteCount = 0

	newCap := float64(len(b.byteArr)) * rate
	nbuf := make([]byte, 0, int(newCap))

	delKeys := make([]K, 0)

	b.idx.Scan(func(key K, idx Idx) bool {
		// offset only contains value, except ttl
		start, offset, has := idx.start(), idx.offset(), idx.hasTTL()
		end := start + offset

		if has {
			ttl := parseTTL(b.byteArr[end:])

			// expired
			if ttl < clock {
				delKeys = append(delKeys, key)
				return true
			}
		}

		// reset
		b.idx.Set(key, newIdx(len(nbuf), offset, has, false))
		if has {
			nbuf = append(nbuf, b.byteArr[start:end+ttlBytes]...)
		} else {
			nbuf = append(nbuf, b.byteArr[start:end]...)
		}

		b.byteCount++
		return true
	})

	for _, key := range delKeys {
		b.idx.Delete(key)
	}

	b.byteArr = nbuf
}
