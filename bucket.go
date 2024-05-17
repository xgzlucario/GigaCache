package cache

import (
	"bytes"
	"encoding/binary"
	"sync"
)

// bucket is the data container for GigaCache.
type bucket struct {
	sync.RWMutex
	options *Options

	// index maps hashed keys to their storage positions in data.
	index map[Key]Idx

	// conflictMap stores keys that have hash conflicts with the index.
	conflictMap map[string]Idx

	// data stores all key-value bytes data.
	data []byte

	// runtime statistics
	interval   int
	unused     uint64
	migrations uint64
	evictions  uint64
	probes     uint64
}

// newBucket initializes and returns a new bucket instance.
func newBucket(options Options) *bucket {
	return &bucket{
		options:     &options,
		index:       make(map[Key]Idx, options.IndexSize),
		conflictMap: make(map[string]Idx),
		data:        bufferPool.Get(options.BufferSize)[:0],
	}
}

// get retrieves the value and its expiration time for the given key string.
func (b *bucket) get(keyStr string, key Key) ([]byte, int64, bool) {
	// Check conflict map.
	idx, found := b.conflictMap[keyStr]
	if found && !idx.expired() {
		_, _, val := b.findEntry(idx)
		return val, idx.TTL(), found
	}

	// Check index map.
	idx, found = b.index[key]
	if found && !idx.expired() {
		_, _, val := b.findEntry(idx)
		return val, idx.TTL(), found
	}

	return nil, 0, false
}

// set stores the key-value pair into the bucket with an expiration timestamp.
func (b *bucket) set(key Key, keyStr, val []byte, ts int64) {
	// Check conflict map.
	idx, found := b.conflictMap[b2s(keyStr)]
	if found {
		entry, oldKeyStr, oldVal := b.findEntry(idx)

		// Update in-place if the lengths match.
		if len(keyStr) == len(oldKeyStr) && len(val) == len(oldVal) {
			copy(oldKeyStr, keyStr)
			copy(oldVal, val)
			b.conflictMap[string(keyStr)] = idx.setTTL(ts)
			return
		}

		// Allocate new space if lengths differ.
		b.unused += uint64(len(entry))
		b.conflictMap[string(keyStr)] = b.appendEntry(keyStr, val, ts)
		return
	}

	// Check index map.
	idx, found = b.index[key]
	if found {
		entry, oldKeyStr, oldVal := b.findEntry(idx)

		// Insert to conflictMap if hash conflict occurs.
		if !idx.expired() && !bytes.Equal(keyStr, oldKeyStr) {
			b.conflictMap[string(keyStr)] = b.appendEntry(keyStr, val, ts)
			return
		}

		// Update in-place if the lengths match.
		if len(keyStr) == len(oldKeyStr) && len(val) == len(oldVal) {
			copy(oldKeyStr, keyStr)
			copy(oldVal, val)
			b.index[key] = idx.setTTL(ts)
			return
		}

		// Allocate new space if lengths differ.
		b.unused += uint64(len(entry))
	}

	// Insert new entry.
	b.index[key] = b.appendEntry(keyStr, val, ts)
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
	idx, found := b.conflictMap[keyStr]
	if found {
		b.removeConflictEntry(keyStr, idx)
		return !idx.expired()
	}

	idx, found = b.index[key]
	if found {
		b.removeIndexEntry(key, idx)
		return !idx.expired()
	}

	return false
}

// setTTL updates the expiration timestamp for a given key.
func (b *bucket) setTTL(key Key, keyStr string, ts int64) bool {
	idx, found := b.conflictMap[keyStr]
	if found && !idx.expired() {
		b.conflictMap[keyStr] = newIdx(idx.start(), ts)
		return true
	}

	idx, found = b.index[key]
	if found && !idx.expired() {
		b.index[key] = newIdx(idx.start(), ts)
		return true
	}

	return false
}

// scan iterates over all alive key-value pairs, calling the Walker function for each.
func (b *bucket) scan(walker Walker) (continueIteration bool) {
	continueIteration = true

	for _, idx := range b.conflictMap {
		if idx.expired() {
			continue
		}
		_, keyBytes, val := b.findEntry(idx)
		continueIteration = walker(keyBytes, val, idx.TTL())
		if !continueIteration {
			return
		}
	}

	for _, idx := range b.index {
		if idx.expired() {
			continue
		}
		_, keyBytes, val := b.findEntry(idx)
		continueIteration = walker(keyBytes, val, idx.TTL())
		if !continueIteration {
			return
		}
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
	for keyStr, idx := range b.conflictMap {
		b.probes++
		if idx.expired() {
			b.removeConflictEntry(keyStr, idx)
			b.evictions++
		}
	}

	for key, idx := range b.index {
		b.probes++
		if idx.expired() {
			b.removeIndexEntry(key, idx)
			b.evictions++
			failCount = 0
		} else {
			failCount++
			if failCount > maxFailCount {
				break
			}
		}
	}

CHECK_MIGRATION:
	// Check if migration is needed.
	unusedRate := float64(b.unused) / float64(len(b.data))
	if unusedRate >= b.options.MigrateRatio {
		b.migrate()
	}
}

// migrate transfers valid key-value pairs to a new container to save memory.
func (b *bucket) migrate() {
	newData := bufferPool.Get(len(b.data))[:0]

	// Migrate data to the new bucket.
	for key, idx := range b.index {
		if idx.expired() {
			delete(b.index, key)
			continue
		}
		// Update with new position.
		b.index[key] = newIdxx(len(newData), idx)
		entry, _, _ := b.findEntry(idx)
		newData = append(newData, entry...)
	}

	for keyStr, idx := range b.conflictMap {
		if idx.expired() {
			delete(b.conflictMap, keyStr)
			continue
		}
		key := Key(b.options.HashFn(keyStr))
		// Check if conflict exists.
		if _, exists := b.index[key]; exists {
			b.conflictMap[keyStr] = newIdxx(len(newData), idx)
		} else {
			b.index[key] = newIdxx(len(newData), idx)
			delete(b.conflictMap, keyStr)
		}
		entry, _, _ := b.findEntry(idx)
		newData = append(newData, entry...)
	}

	bufferPool.Put(b.data)
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
	b.unused += uint64(len(entry))
	delete(b.conflictMap, key)
}

// removeIndexEntry removes an index entry from the index map.
func (b *bucket) removeIndexEntry(key Key, idx Idx) {
	entry, _, _ := b.findEntry(idx)
	b.unused += uint64(len(entry))
	delete(b.index, key)
}
