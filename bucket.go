package cache

import (
	"bytes"
	"encoding/binary"
	"sync"

	"github.com/cockroachdb/swiss"
)

// bucket is the data container for GigaCache.
type bucket struct {
	sync.RWMutex
	options *Options

	// index is the index map for cache, mapped hash(kstr) to the position that data real stored.
	index *swiss.Map[Key, Idx]

	// conflict map store keys that hash conflict with index.
	cmap *cmap

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
		index:   swiss.New[Key, Idx](options.IndexSize),
		cmap:    newCMap(),
		data:    bpool.Get(options.BufferSize)[:0],
	}
}

func (b *bucket) get(kstr string, key Key) ([]byte, int64, bool) {
	// find conflict map.
	idx, ok := b.cmap.Get(kstr)
	if ok && !idx.expired() {
		_, _, val := b.find(idx)
		return val, idx.TTL(), ok
	}

	// find index map.
	idx, ok = b.index.Get(key)
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
	idx, ok := b.cmap.Get(b2s(kstr))
	if ok {
		entry, _, _ := b.find(idx)
		b.unused += uint64(len(entry))
		b.cmap.Put(b2s(kstr), newIdx(len(b.data), ts))
		goto ADD
	}

	// check index map.
	idx, ok = b.index.Get(key)
	if ok {
		entry, oldKstr, _ := b.find(idx)
		b.unused += uint64(len(entry))
		// hash conflict
		if !idx.expired() && !bytes.Equal(oldKstr, kstr) {
			b.cmap.Put(string(kstr), newIdx(len(b.data), ts))
			goto ADD
		}
	}

	// update index.
	b.index.Put(key, newIdx(len(b.data), ts))

ADD:
	// append klen, vlen, key, val.
	b.data = binary.AppendUvarint(b.data, uint64(len(kstr)))
	b.data = binary.AppendUvarint(b.data, uint64(len(val)))
	b.data = append(b.data, kstr...)
	b.data = append(b.data, val...)
}

func (b *bucket) remove(key Key, kstr string) bool {
	idx, ok := b.cmap.Get(kstr)
	if ok {
		b.removeConflict(kstr, idx)
		return !idx.expired()
	}

	idx, ok = b.index.Get(key)
	if ok {
		b.removeIndex(key, idx)
		return !idx.expired()
	}

	return false
}

func (b *bucket) setTTL(key Key, kstr string, ts int64) bool {
	idx, ok := b.cmap.Get(kstr)
	if ok && !idx.expired() {
		b.cmap.Put(kstr, newIdx(idx.start(), ts))
		return true
	}

	idx, ok = b.index.Get(key)
	if ok && !idx.expired() {
		b.index.Put(key, newIdx(idx.start(), ts))
		return true
	}

	return false
}

// When evict enabled, scan key-values by iter index smap.
func (b *bucket) scan(f Walker) (next bool) {
	next = true
	scanf := func(idx Idx) bool {
		if idx.expired() {
			return true
		}
		_, kstr, val := b.find(idx)
		next = f(kstr, val, idx.TTL())
		return next
	}
	if next {
		b.cmap.All(func(_ string, idx Idx) bool {
			return scanf(idx)
		})
	}
	if next {
		b.index.All(func(_ Key, idx Idx) bool {
			return scanf(idx)
		})
	}
	return
}

// When evict disabled, scan key-values by reading bytes data sequentially.
func (b *bucket) scan2(f Walker) (next bool) {
	next = true
	for index := 0; index < len(b.data) && next; {
		// klen
		klen, n := binary.Uvarint(b.data[index:])
		index += n
		// vlen
		vlen, n := binary.Uvarint(b.data[index:])
		index += n
		// kstr
		kstr := b.data[index : index+int(klen)]
		index += int(klen)
		// val
		val := b.data[index : index+int(vlen)]
		index += int(vlen)

		next = f(kstr, val, 0)
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
	b.cmap.All(func(key string, idx Idx) bool {
		b.probe++
		if idx.expired() {
			b.removeConflict(key, idx)
			b.evict++
		}
		return true
	})

	b.index.All(func(key Key, idx Idx) bool {
		b.probe++
		if idx.expired() {
			b.removeIndex(key, idx)
			b.evict++
			failed = 0
			return true
		}
		failed++
		return failed <= maxFailCount
	})

MIG:
	// check need to migrate.
	rate := float64(b.unused) / float64(len(b.data))
	if b.unused >= b.options.MigrateDelta && rate >= b.options.MigrateRatio {
		b.migrate()
	}
}

// migrate move valid key-value pairs to the new container to save memory.
func (b *bucket) migrate() {
	newData := bpool.Get(len(b.data))[:0]

	// migrate data to new bucket.
	b.index.All(func(key Key, idx Idx) bool {
		if idx.expired() {
			b.index.Delete(key)
			return true
		}
		// update with new position.
		b.index.Put(key, newIdxx(len(newData), idx))
		entry, _, _ := b.find(idx)
		newData = append(newData, entry...)
		return true
	})

	b.cmap.All(func(kstr string, idx Idx) bool {
		if idx.expired() {
			b.cmap.Delete(kstr)
			return true
		}
		key := Key(b.options.HashFn(kstr))
		// check if conflict.
		_, ok := b.index.Get(key)
		if ok {
			b.cmap.Put(kstr, newIdxx(len(newData), idx))
		} else {
			b.index.Put(key, newIdxx(len(newData), idx))
			b.cmap.Delete(kstr)
		}
		entry, _, _ := b.find(idx)
		newData = append(newData, entry...)

		return true
	})

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
	b.cmap.Delete(key)
}

func (b *bucket) removeIndex(key Key, idx Idx) {
	entry, _, _ := b.find(idx)
	b.unused += uint64(len(entry))
	b.index.Delete(key)
}
