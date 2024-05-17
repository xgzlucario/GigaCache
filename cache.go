package cache

import (
	"slices"
	"time"
)

const (
	noTTL        = 0
	KB           = 1024
	maxFailCount = 3 // maxFailCount indicates that the eviction algorithm breaks when consecutive unexpired key-value pairs are detected.
)

// GigaCache implements a key-value cache.
type GigaCache struct {
	mask      uint32
	hashFn    HashFn
	buckets   []*bucket
	reusedBuf []byte
}

// New creates a new instance of GigaCache.
func New(options Options) *GigaCache {
	if err := validateOptions(options); err != nil {
		panic(err)
	}
	cache := &GigaCache{
		mask:    options.ShardCount - 1,
		hashFn:  options.HashFn,
		buckets: make([]*bucket, options.ShardCount),
	}
	for i := range cache.buckets {
		cache.buckets[i] = newBucket(options, cache)
	}
	return cache
}

// getShard returns the appropriate bucket and key by hashing the input string.
// Different hash functions for sharding and indexing significantly reduce hash conflicts.
func (c *GigaCache) getShard(keyStr string) (*bucket, Key) {
	hash := c.hashFn(keyStr)
	hash32 := uint32(hash >> 1)
	return c.buckets[hash32&c.mask], Key(hash)
}

// Get retrieves the value and its expiration time for a given key.
func (c *GigaCache) Get(keyStr string) ([]byte, int64, bool) {
	bucket, key := c.getShard(keyStr)
	bucket.RLock()
	value, timestamp, found := bucket.get(keyStr, key)
	if found {
		value = slices.Clone(value)
	}
	bucket.RUnlock()
	return value, timestamp, found
}

// SetTx stores a key-value pair with a specific expiration timestamp.
func (c *GigaCache) SetTx(keyStr string, value []byte, expiration int64) {
	bucket, key := c.getShard(keyStr)
	bucket.Lock()
	bucket.evictExpiredItems()
	bucket.set(key, s2b(&keyStr), value, expiration)
	bucket.Unlock()
}

// Set stores a key-value pair with no expiration.
func (c *GigaCache) Set(keyStr string, value []byte) {
	c.SetTx(keyStr, value, noTTL)
}

// SetEx stores a key-value pair with a specific expiration duration.
func (c *GigaCache) SetEx(keyStr string, value []byte, duration time.Duration) {
	expiration := GetNanoSec() + int64(duration)
	c.SetTx(keyStr, value, expiration)
}

// Remove deletes a key-value pair from the cache.
func (c *GigaCache) Remove(keyStr string) bool {
	bucket, key := c.getShard(keyStr)
	bucket.Lock()
	bucket.evictExpiredItems()
	removed := bucket.remove(key, keyStr)
	bucket.Unlock()
	return removed
}

// SetTTL updates the expiration timestamp for a key.
func (c *GigaCache) SetTTL(keyStr string, expiration int64) bool {
	bucket, key := c.getShard(keyStr)
	bucket.Lock()
	success := bucket.setTTL(key, keyStr, expiration)
	bucket.evictExpiredItems()
	bucket.Unlock()
	return success
}

// Walker defines a callback function for iterating over key-value pairs.
type Walker func(key, value []byte, ttl int64) (continueIteration bool)

// Scan iterates over all alive key-value pairs without copying the data.
// DO NOT MODIFY the bytes as they are not copied.
func (c *GigaCache) Scan(callback Walker) {
	for _, bucket := range c.buckets {
		bucket.RLock()
		continueIteration := bucket.scan(callback)
		bucket.RUnlock()
		if !continueIteration {
			return
		}
	}
}

// Migrate transfers all data to new buckets.
func (c *GigaCache) Migrate() {
	for _, bucket := range c.buckets {
		bucket.Lock()
		bucket.migrate()
		bucket.Unlock()
	}
}

// Stats represents the runtime statistics of GigaCache.
type Stats struct {
	Len       int
	Conflicts int
	Alloc     uint64
	Unused    uint64
	Migrates  uint64
	Evictions uint64
	Probes    uint64
}

// GetStats returns the current runtime statistics of GigaCache.
func (c *GigaCache) GetStats() (stats Stats) {
	for _, bucket := range c.buckets {
		bucket.RLock()
		stats.Len += len(bucket.index) + len(bucket.conflictMap)
		stats.Conflicts += len(bucket.conflictMap)
		stats.Alloc += uint64(len(bucket.data))
		stats.Unused += uint64(bucket.unused)
		stats.Migrates += uint64(bucket.migrations)
		stats.Evictions += bucket.evictions
		stats.Probes += bucket.probes
		bucket.RUnlock()
	}
	return
}

// UnusedRate calculates the percentage of unused space in the cache.
func (s Stats) UnusedRate() float64 {
	return float64(s.Unused) / float64(s.Alloc) * 100
}

// EvictionRate calculates the percentage of evictions relative to probes.
func (s Stats) EvictionRate() float64 {
	return float64(s.Evictions) / float64(s.Probes) * 100
}
