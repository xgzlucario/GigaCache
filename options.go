package cache

import "errors"

// Options is the configuration of GigaCache.
type Options struct {
	// ShardCount is shard numbers of cache.
	ShardCount uint32

	// Default size of the bucket initial.
	IndexSize  uint32
	BufferSize int

	// Migrate threshold for a bucket to trigger a migration.
	MigrateThresRatio float64
	MigrateDelta      uint64

	// OnRemove called when a key-value pair is evicted.
	OnRemove OnRemove
}

// DefaultOptions
var DefaultOptions = Options{
	ShardCount:        1024,
	IndexSize:         1024,
	BufferSize:        64 * 1024, // 64 KB
	MigrateThresRatio: 0.6,
	MigrateDelta:      4 * 1024, // 4 * KB
	OnRemove:          nil,
}

func checkOptions(options Options) error {
	if options.ShardCount == 0 {
		return errors.New("cache/options: invalid shard count")
	}
	return nil
}
