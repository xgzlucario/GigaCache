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
	EvictInterval uint8

	// DisableEvict
	// Set `true` when you don't need any expiration times.
	DisableEvict bool

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
	DisableEvict:    false,
	MigrateRatio:    0.4,
	ConcurrencySafe: true,
}

func validateOptions(options Options) error {
	if options.ShardCount == 0 {
		return errors.New("cache/options: invalid shard count")
	}
	return nil
}
