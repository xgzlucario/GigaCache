package cache

import "errors"

// Options is the configuration of GigaCache.
type Options struct {
	// ShardCount is shard numbers of cache.
	ShardCount uint32

	// Default size of the bucket initial.
	IndexSize  int
	BufferSize int

	// EvictInterval indicates the frequency of execution of the eliminate algorithm.
	// the higher the frequency, the more expired key-value pairs will be evicted,
	// but accordingly it will slow down the overall performance,
	// because the system needs to spend more time on probing and evicting.
	EvictInterval int

	// DisableEvict
	// Set `true` when you don't need any expiration times.
	DisableEvict bool

	// Migrate threshold for a bucket to trigger a migration.
	MigrateRatio float64
	MigrateDelta uint64

	// HashFn is custom hash function, default is runtime.memhash.
	HashFn HashFn

	// OnRemove called when key-value pair is evicted.
	OnRemove Callback

	// OnHashConflict called when hash conflict occurred.
	OnHashConflict Callback
}

var DefaultOptions = Options{
	ShardCount:    1024,
	IndexSize:     1024,
	BufferSize:    64 * 1024, // 64 KB
	EvictInterval: 3,
	DisableEvict:  false,
	MigrateRatio:  0.4,
	MigrateDelta:  4 * 1024, // 4 * KB
	HashFn:        MemHash,
}

func checkOptions(options Options) error {
	if options.ShardCount == 0 {
		return errors.New("cache/options: invalid shard count")
	}
	if options.EvictInterval < 0 {
		return errors.New("cache/options: invalid evict interval")
	}
	return nil
}
