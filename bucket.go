package cache

import (
	"bytes"
	"encoding/binary"
	"sync"

	"github.com/cockroachdb/swiss"
)

// bucket is the data container for GigaCache.
type bucket struct {
	rwlocker
	root    *GigaCache
	options *Options

	// index maps hashed keys to their storage positions in data.
	index *swiss.Map[Key, Idx]

	// conflictMap stores keys that have hash conflicts with the index.
	conflictMap *swiss.Map[string, Idx]

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
func newBucket(options Options, root *GigaCache) *bucket {
	bucket := &bucket{
		rwlocker:    &emptyLocker{},
		root:        root,
		options:     &options,
		index:       swiss.New[Key, Idx](options.IndexSize),
		conflictMap: swiss.New[string, Idx](8),
		data:        make([]byte, 0, options.BufferSize),
	}
	if options.ConcurrencySafe {
		bucket.rwlocker = &sync.RWMutex{}
	}
	return bucket
}

// get retrieves the value and its expiration time for the given key string.
func (b *bucket) get(keyStr string, key Key) ([]byte, int64, bool) {
	// Check conflict map.
	idx, found := b.conflictMap.Get(keyStr)
	if found && !idx.expired() {
		_, _, val := b.findEntry(idx)
		return val, idx.TTL(), found
	}

	// Check index map.
	idx, found = b.index.Get(key)
	if found && !idx.expired() {
		_, _, val := b.findEntry(idx)
		return val, idx.TTL(), found
	}

	return nil, 0, false
}

// set stores the key-value pair into the bucket with an expiration timestamp.
func (b *bucket) set(key Key, keyStr, val []byte, ts int64) (newField bool) {
	// Check conflict map.
	idx, found := b.conflictMap.Get(b2s(keyStr))
	if found {
		entry, oldKeyStr, oldVal := b.findEntry(idx)

		// Update in-place if the lengths match.
		if len(keyStr) == len(oldKeyStr) && len(val) == len(oldVal) {
			copy(oldKeyStr, keyStr)
			copy(oldVal, val)
			b.conflictMap.Put(string(keyStr), idx.setTTL(ts))
			return false
		}

		// Allocate new space if lengths differ.
		b.unused += uint32(len(entry))
		b.conflictMap.Put(string(keyStr), b.appendEntry(keyStr, val, ts))
		return false
	}

	// Check index map.
	idx, found = b.index.Get(key)
	if found {
		entry, oldKeyStr, oldVal := b.findEntry(idx)

		// Insert to conflictMap if hash conflict occurs.
		if !idx.expired() && !bytes.Equal(keyStr, oldKeyStr) {
			b.conflictMap.Put(string(keyStr), b.appendEntry(keyStr, val, ts))
			return false
		}

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
func (b *bucket) remove(key Key, keyStr string) bool {
	idx, found := b.conflictMap.Get(keyStr)
	if found {
		b.removeConflictEntry(keyStr, idx)
		return !idx.expired()
	}

	idx, found = b.index.Get(key)
	if found {
		b.removeIndexEntry(key, idx)
		return !idx.expired()
	}

	return false
}

// setTTL updates the expiration timestamp for a given key.
func (b *bucket) setTTL(key Key, keyStr string, ts int64) bool {
	idx, found := b.conflictMap.Get(keyStr)
	if found && !idx.expired() {
		b.conflictMap.Put(keyStr, newIdx(idx.start(), ts))
		return true
	}

	idx, found = b.index.Get(key)
	if found && !idx.expired() {
		b.index.Put(key, newIdx(idx.start(), ts))
		return true
	}

	return false
}

// scan iterates over all alive key-value pairs, calling the Walker function for each.
func (b *bucket) scan(walker Walker) (continueIteration bool) {
	continueIteration = true

	b.conflictMap.All(func(_ string, idx Idx) bool {
		if idx.expired() {
			return true
		}
		_, keyBytes, val := b.findEntry(idx)
		continueIteration = walker(keyBytes, val, idx.TTL())
		return continueIteration
	})

	if continueIteration {
		b.index.All(func(_ Key, idx Idx) bool {
			if idx.expired() {
				return true
			}
			_, keyBytes, val := b.findEntry(idx)
			continueIteration = walker(keyBytes, val, idx.TTL())
			return continueIteration
		})
	}
	return
}

// eliminate removes expired key-value pairs and triggers migration if necessary.
func (b *bucket) evictExpiredItems() {
	var failCount int
	if b.options.DisableEvict {
		goto CHECK_MIGRATION
	}

	b.interval++
	if b.interval < b.options.EvictInterval {
		return
	}
	b.interval = 0

	// Probing
	b.conflictMap.All(func(keyStr string, idx Idx) bool {
		b.probes++
		if idx.expired() {
			b.removeConflictEntry(keyStr, idx)
			b.evictions++
		}
		return true
	})

	b.index.All(func(key Key, idx Idx) bool {
		b.probes++
		if idx.expired() {
			b.removeIndexEntry(key, idx)
			b.evictions++
			failCount = 0
		} else {
			failCount++
			if failCount > maxFailCount {
				return false
			}
		}
		return true
	})

CHECK_MIGRATION:
	// Check if migration is needed.
	unusedRate := float64(b.unused) / float64(len(b.data))
	if unusedRate >= b.options.MigrateRatio {
		b.migrate()
	}
}

// migrate transfers valid key-value pairs to a new container to save memory.
func (b *bucket) migrate() {
	var newData []byte
	if b.root != nil {
		newData = b.root.reusedBuf[:0]
	} else {
		newData = make([]byte, 0, len(b.data))
	}

	// Migrate data to the new bucket.
	b.index.All(func(key Key, idx Idx) bool {
		if idx.expired() {
			b.index.Delete(key)
			return true
		}
		// Update with new position.
		b.index.Put(key, newIdxx(len(newData), idx))
		entry, _, _ := b.findEntry(idx)
		newData = append(newData, entry...)
		return true
	})

	b.conflictMap.All(func(keyStr string, idx Idx) bool {
		if idx.expired() {
			b.conflictMap.Delete(keyStr)
			return true
		}
		key := Key(b.options.HashFn(keyStr))
		// Check if conflict exists.
		if _, exists := b.index.Get(key); exists {
			b.conflictMap.Put(keyStr, newIdxx(len(newData), idx))
		} else {
			b.index.Put(key, newIdxx(len(newData), idx))
			b.conflictMap.Delete(keyStr)
		}
		entry, _, _ := b.findEntry(idx)
		newData = append(newData, entry...)
		return true
	})

	if b.root != nil {
		b.root.reusedBuf = b.data
	}
	b.data = newData
	b.unused = 0
	b.migrations++
}

// findEntry retrieves the full entry, key, and value bytes for the given index.
func (b *bucket) findEntry(idx Idx) (entry, key, val []byte) {
	position := idx.start()
	// Key length
	keyLen, bytesRead := binary.Uvarint(b.data[position:])
	position += bytesRead
	// Value length
	valLen, bytesRead := binary.Uvarint(b.data[position:])
	position += bytesRead
	// Key
	key = b.data[position : position+int(keyLen)]
	position += int(keyLen)
	// Value
	val = b.data[position : position+int(valLen)]
	position += int(valLen)

	return b.data[idx.start():position], key, val
}

// removeConflictEntry removes a conflict entry from the conflict map.
func (b *bucket) removeConflictEntry(key string, idx Idx) {
	entry, _, _ := b.findEntry(idx)
	b.unused += uint32(len(entry))
	b.conflictMap.Delete(key)
}

// removeIndexEntry removes an index entry from the index map.
func (b *bucket) removeIndexEntry(key Key, idx Idx) {
	entry, _, _ := b.findEntry(idx)
	b.unused += uint32(len(entry))
	b.index.Delete(key)
}
