package cache

import "errors"

// Options is the configuration of GigaCache.
type Options struct {
	// ShardCount is shard numbers of cache.
	ShardCount uint32

	// Default size of the bucket initial.
	IndexSize  uint32
	BufferSize int

	// MaxFailCount indicates that the algorithm exits
	// when `n` consecutive unexpired key-value pairs are detected.
	MaxFailCount int

	// HintEnabled seeks a balance between performance and memory usage.
	// When `true`, performance is prioritized.
	// When `false`, reduce memory usage is prioritized.
	// If you prefer to reduce memory usage over performance, set it to `false`.
	HintEnabled bool

	// Migrate threshold for a bucket to trigger a migration.
	MigrateThresRatio float64
	MigrateDelta      uint64

	// OnEvict is callback function that is called when a key-value pair is evicted.
	OnEvict OnEvictCallback
}

// DefaultOptions
var DefaultOptions = Options{
	ShardCount:        1024,
	IndexSize:         128,
	BufferSize:        64 * 1024, // 64 KB
	MaxFailCount:      3,
	HintEnabled:       true,
	MigrateThresRatio: 0.6,
	MigrateDelta:      4 * 1 << 10, // 4 KB
}

func checkOptions(options Options) error {
	if options.ShardCount == 0 {
		return errors.New("cache/options: invalid shard count")
	}
	if options.MaxFailCount < 0 {
		return errors.New("cache/options: maxFailCount should not less than 0")
	}
	return nil
}
