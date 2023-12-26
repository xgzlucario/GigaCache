package cache

// Option is the configuration of GigaCache.
type Option struct {
	// ShardCount is shard numbers of GigaCache.
	ShardCount uint32

	// Initial size of the bucket.
	DefaultIdxMapSize uint32
	DefaultBufferSize int

	// Configuration of evict strategy.
	MaxProbeCount uint16
	MaxFailCount  uint16

	// Migrate threshold for a bucket to trigger a migration.
	MigrateThresRatio float64
	MigrateDelta      uint64

	// OnEvict is evict callback function.
	OnEvict OnEvictCallback
}

// DefaultOption
var DefaultOption = Option{
	ShardCount:        1024,
	DefaultIdxMapSize: 64,
	DefaultBufferSize: 64 * 1024, // 64 KB
	MaxProbeCount:     1000,
	MaxFailCount:      3,
	MigrateThresRatio: 0.6,
	MigrateDelta:      4 * 1 << 10, // 4 KB
}
