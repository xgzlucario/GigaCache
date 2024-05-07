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

	// index is the index map for cache, mapped hash(kstr) to the position that data real stored.
	index map[Key]Idx

	// conflict map store keys that hash conflict with index.
	cmap map[string]Idx

	// data store all key-value bytes data.
	data []byte

	// runtime stats.
	interval int
	unused   uint64
	migrates uint64
	evict    uint64
	probe    uint64
}

func newBucket(options Options) *bucket {
	return &bucket{
		options: &options,
		index:   make(map[Key]Idx, options.IndexSize),
		cmap:    map[string]Idx{},
		data:    bpool.Get(options.BufferSize)[:0],
	}
}

func (b *bucket) get(kstr string, key Key) ([]byte, int64, bool) {
	// find conflict map.
	idx, ok := b.cmap[kstr]
	if ok && !idx.expired() {
		_, _, val := b.find(idx)
		return val, idx.TTL(), ok
	}

	// find index map.
	idx, ok = b.index[key]
	if ok && !idx.expired() {
		_, _, val := b.find(idx)
		return val, idx.TTL(), ok
	}

	return nil, 0, false
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
	// check conflict map.
	idx, ok := b.cmap[b2s(kstr)]
	if ok {
		entry, kstrOld, valOld := b.find(idx)

		// update inplaced.
		if len(kstr) == len(kstrOld) && len(val) == len(valOld) {
			copy(kstrOld, kstr)
			copy(valOld, val)
			b.cmap[string(kstr)] = idx.setTTL(ts)
			return
		}

		// alloc new space.
		b.unused += uint64(len(entry))
		b.cmap[string(kstr)] = b.appendEntry(kstr, val, ts)
		return
	}

	// check index map.
	idx, ok = b.index[key]
	if ok {
		entry, kstrOld, valOld := b.find(idx)

		// if hash conflict, insert to cmap.
		if !idx.expired() && !bytes.Equal(kstr, kstrOld) {
			b.cmap[string(kstr)] = b.appendEntry(kstr, val, ts)
			return
		}

		// update inplaced.
		if len(kstr) == len(kstrOld) && len(val) == len(valOld) {
			copy(kstrOld, kstr)
			copy(valOld, val)
			b.index[key] = idx.setTTL(ts)
			return
		}

		// alloc new space.
		b.unused += uint64(len(entry))
	}

	// insert.
	b.index[key] = b.appendEntry(kstr, val, ts)
}

func (b *bucket) appendEntry(kstr, val []byte, ts int64) Idx {
	idx := newIdx(len(b.data), ts)
	// append klen, vlen, key, val.
	b.data = binary.AppendUvarint(b.data, uint64(len(kstr)))
	b.data = binary.AppendUvarint(b.data, uint64(len(val)))
	b.data = append(b.data, kstr...)
	b.data = append(b.data, val...)
	return idx
}

func (b *bucket) remove(key Key, kstr string) bool {
	idx, ok := b.cmap[kstr]
	if ok {
		b.removeConflict(kstr, idx)
		return !idx.expired()
	}

	idx, ok = b.index[key]
	if ok {
		b.removeIndex(key, idx)
		return !idx.expired()
	}

	return false
}

func (b *bucket) setTTL(key Key, kstr string, ts int64) bool {
	idx, ok := b.cmap[kstr]
	if ok && !idx.expired() {
		b.cmap[kstr] = newIdx(idx.start(), ts)
		return true
	}

	idx, ok = b.index[key]
	if ok && !idx.expired() {
		b.index[key] = newIdx(idx.start(), ts)
		return true
	}

	return false
}

func (b *bucket) scan(f Walker) (next bool) {
	next = true

	for _, idx := range b.cmap {
		if idx.expired() {
			continue
		}
		_, kstr, val := b.find(idx)
		next = f(kstr, val, idx.TTL())
		if !next {
			return
		}
	}

	for _, idx := range b.index {
		if idx.expired() {
			continue
		}
		_, kstr, val := b.find(idx)
		next = f(kstr, val, idx.TTL())
		if !next {
			return
		}
	}
	return
}

// eliminate the expired key-value pairs.
func (b *bucket) eliminate() {
	var failed int
	if b.options.DisableEvict {
		goto MIG
	}

	b.interval++
	if b.interval < b.options.EvictInterval {
		return
	}
	b.interval = 0

	// probing
	for key, idx := range b.cmap {
		b.probe++
		if idx.expired() {
			b.removeConflict(key, idx)
			b.evict++
		}
	}

	for key, idx := range b.index {
		b.probe++
		if idx.expired() {
			b.removeIndex(key, idx)
			b.evict++
			failed = 0

		} else {
			failed++
			if failed > maxFailCount {
				break
			}
		}
	}

MIG:
	// check need to migrate.
	rate := float64(b.unused) / float64(len(b.data))
	if rate >= b.options.MigrateRatio {
		b.migrate()
	}
}

// migrate move valid key-value pairs to the new container to save memory.
func (b *bucket) migrate() {
	newData := bpool.Get(len(b.data))[:0]

	// migrate data to new bucket.
	for key, idx := range b.index {
		if idx.expired() {
			delete(b.index, key)
			continue
		}
		// update with new position.
		b.index[key] = newIdxx(len(newData), idx)
		entry, _, _ := b.find(idx)
		newData = append(newData, entry...)
	}

	for kstr, idx := range b.cmap {
		if idx.expired() {
			delete(b.cmap, kstr)
			continue
		}
		key := Key(b.options.HashFn(kstr))
		// check if conflict.
		_, ok := b.index[key]
		if ok {
			b.cmap[kstr] = newIdxx(len(newData), idx)
		} else {
			b.index[key] = newIdxx(len(newData), idx)
			delete(b.cmap, kstr)
		}
		entry, _, _ := b.find(idx)
		newData = append(newData, entry...)
	}

	bpool.Put(b.data)
	b.data = newData
	b.unused = 0
	b.migrates++
}

func (b *bucket) find(idx Idx) (entry, kstr, val []byte) {
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

	return b.data[idx.start():index], kstr, val
}

func (b *bucket) removeConflict(key string, idx Idx) {
	entry, _, _ := b.find(idx)
	b.unused += uint64(len(entry))
	delete(b.cmap, key)
}

func (b *bucket) removeIndex(key Key, idx Idx) {
	entry, _, _ := b.find(idx)
	b.unused += uint64(len(entry))
	delete(b.index, key)
}
