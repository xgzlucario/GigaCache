package cache

import "errors"

// Options is the configuration of GigaCache.
type Options struct {
	// ShardCount is shard numbers of GigaCache.
	ShardCount uint32

	// Default size of the bucket initial.
	IndexSize  uint32
	BufferSize int

	// Configuration of evict strategy.
	MaxProbeCount uint16
	MaxFailCount  uint16

	// Migrate threshold for a bucket to trigger a migration.
	MigrateThresRatio float64
	MigrateDelta      uint64

	// OnEvict is evict callback function.
	OnEvict OnEvictCallback
}

// DefaultOptions
var DefaultOptions = Options{
	ShardCount:        1024,
	IndexSize:         128,
	BufferSize:        64 * 1024, // 64 KB
	MaxProbeCount:     1000,
	MaxFailCount:      3,
	MigrateThresRatio: 0.6,
	MigrateDelta:      4 * 1 << 10, // 4 KB
}

func checkOptions(options Options) error {
	if options.ShardCount == 0 {
		return errors.New("cache/options: invalid shard count")
	}
	if options.MaxProbeCount == 0 {
		return errors.New("cache/options: max probe count should greater than 0")
	}
	return nil
}
