package cache

import (
	"encoding/binary"
	"sync"
	"time"

	"github.com/cockroachdb/swiss"
	"github.com/zeebo/xxh3"
)

// bucket is the data container for GigaCache.
type bucket struct {
	rwlocker
	options *Options

	// index maps hashed keys to their storage positions in data.
	index *swiss.Map[Key, Idx]

	// data stores all key-value bytes data.
	data []byte

	// runtime statistics
	interval   byte
	unused     uint32
	migrations uint32
	evictions  uint64
	probes     uint64
}

type rwlocker interface {
	Lock()
	Unlock()
	RLock()
	RUnlock()
}

type emptyLocker struct{}

func (emptyLocker) Lock() {}

func (emptyLocker) Unlock() {}

func (emptyLocker) RLock() {}

func (emptyLocker) RUnlock() {}

// newBucket initializes and returns a new bucket instance.
func newBucket(options Options) *bucket {
	bucket := &bucket{
		rwlocker: &emptyLocker{},
		options:  &options,
		index:    swiss.New[Key, Idx](options.IndexSize),
		data:     make([]byte, 0, options.BufferSize),
	}
	if options.ConcurrencySafe {
		bucket.rwlocker = &sync.RWMutex{}
	}
	return bucket
}

func hashFn(kstr string) Key {
	return xxh3.HashString128(kstr)
}

// get retrieves the value and its expiration time for the given key string.
func (b *bucket) get(key Key) ([]byte, int64, bool) {
	idx, found := b.index.Get(key)
	if found && !idx.expired() {
		_, _, val := b.findEntry(idx)
		return val, idx.lo, found
	}

	return nil, 0, false
}

// set stores the key-value pair into the bucket with an expiration timestamp.
func (b *bucket) set(key Key, keyStr, val []byte, ts int64) (newField bool) {
	idx, found := b.index.Get(key)
	if found {
		entry, oldKeyStr, oldVal := b.findEntry(idx)

		// Update in-place if the lengths match.
		if len(keyStr) == len(oldKeyStr) && len(val) == len(oldVal) {
			copy(oldKeyStr, keyStr)
			copy(oldVal, val)
			b.index.Put(key, idx.setTTL(ts))
			return false
		}

		// Allocate new space if lengths differ.
		b.unused += uint32(len(entry))
	}

	// Insert new entry.
	b.index.Put(key, b.appendEntry(keyStr, val, ts))
	return true
}

// appendEntry appends a key-value entry to the data slice and returns the index.
func (b *bucket) appendEntry(keyStr, val []byte, ts int64) Idx {
	idx := newIdx(len(b.data), ts)
	// Append key length, value length, key, and value.
	b.data = binary.AppendUvarint(b.data, uint64(len(keyStr)))
	b.data = binary.AppendUvarint(b.data, uint64(len(val)))
	b.data = append(b.data, keyStr...)
	b.data = append(b.data, val...)
	return idx
}

// remove deletes the key-value pair from the bucket.
func (b *bucket) remove(key Key) bool {
	idx, found := b.index.Get(key)
	if found {
		b.removeEntry(key, idx)
		return !idx.expired()
	}

	return false
}

// setTTL updates the expiration timestamp for a given key.
func (b *bucket) setTTL(key Key, ts int64) bool {
	idx, found := b.index.Get(key)
	if found && !idx.expired() {
		b.index.Put(key, newIdx(idx.start(), ts))
		return true
	}

	return false
}

// scan iterates over all alive key-value pairs, calling the Walker function for each.
func (b *bucket) scan(walker Walker) (next bool) {
	next = true

	b.index.All(func(_ Key, idx Idx) bool {
		if idx.expired() {
			return true
		}
		_, kstr, val := b.findEntry(idx)
		next = walker(kstr, val, idx.lo)
		return next
	})
	return
}

func (b *bucket) evictExpiredKeys(force ...bool) {
	flag := len(force) > 0 && force[0]
	if !flag {
		if b.options.DisableEvict {
			return
		}

		b.interval++
		if b.interval < b.options.EvictInterval {
			return
		}
		b.interval = 0
	}

	var failed int
	nanosec := time.Now().UnixNano()

	// Probing
	b.index.All(func(key Key, idx Idx) bool {
		b.probes++
		if idx.expiredWith(nanosec) {
			b.removeEntry(key, idx)
			b.evictions++
			failed = 0
		} else {
			failed++
		}
		return failed <= maxFailed
	})

	// Check if migration is needed.
	unusedRate := float64(b.unused) / float64(len(b.data))
	if unusedRate >= b.options.MigrateRatio {
		b.migrate()
	}
}

// migrate transfers valid key-value pairs to a new container to save memory.
func (b *bucket) migrate() {
	newData := make([]byte, 0, len(b.data))

	// Migrate data to the new bucket.
	nanosec := time.Now().UnixNano()
	b.index.All(func(key Key, idx Idx) bool {
		if idx.expiredWith(nanosec) {
			b.index.Delete(key)
			return true
		}
		// Update with new position.
		b.index.Put(key, newIdxx(len(newData), idx))
		entry, _, _ := b.findEntry(idx)
		newData = append(newData, entry...)
		return true
	})

	b.data = newData
	b.unused = 0
	b.migrations++
}

// findEntry retrieves the full entry, key, and value bytes for the given index.
func (b *bucket) findEntry(idx Idx) (entry, kstr, val []byte) {
	pos := idx.start()
	// read keyLen
	klen, n := binary.Uvarint(b.data[pos:])
	pos += n
	// read valLen
	vlen, n := binary.Uvarint(b.data[pos:])
	pos += n
	// read kstr
	kstr = b.data[pos : pos+int(klen)]
	pos += int(klen)
	// read value
	val = b.data[pos : pos+int(vlen)]
	pos += int(vlen)

	return b.data[idx.start():pos], kstr, val
}

func (b *bucket) removeEntry(key Key, idx Idx) {
	entry, _, _ := b.findEntry(idx)
	b.unused += uint32(len(entry))
	b.index.Delete(key)
}
