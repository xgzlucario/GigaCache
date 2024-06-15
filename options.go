package cache

import "errors"

// Options is the configuration of GigaCache.
type Options struct {
	// ShardCount is shard numbers of cache.
	ShardCount uint32

	// Default size of the bucket initial.
	IndexSize  int
	BufferSize int

	// EvictInterval indicates the frequency of execution of the evict algorithm.
	// if n >= 0, evict algorithm auto perform every `n` times write.
	// if n < 0, evict is disabled.
	EvictInterval int

	// Migrate threshold for a bucket to trigger a migration.
	MigrateRatio float64

	// ConcurrencySafe specifies whether RWLocker are required for multithreading safety.
	ConcurrencySafe bool
}

var DefaultOptions = Options{
	ShardCount:      1024,
	IndexSize:       1024,
	BufferSize:      64 * KB,
	EvictInterval:   5,
	MigrateRatio:    0.4,
	ConcurrencySafe: true,
}

func validateOptions(options Options) error {
	if options.ShardCount == 0 {
		return errors.New("cache/options: invalid shard count")
	}
	return nil
}
