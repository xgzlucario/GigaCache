package cache

// Option is the option of GigaCache.
type Option struct {
	// ShardCount is shard numbers of GigaCache.
	ShardCount int

	// Initial size of the bucket.
	DefaultIdxMapSize int
	DefaultBufferSize int

	// Configuration of evict strategy.
	MaxProbeCount int
	MaxFailCount  int

	// SCacheSize is the number of bytes of space reused in the scache.
	SCacheSize int

	// Migrate threshold for a bucket to trigger a migration.
	MigrateThresRatio float64
	MigrateDelta      uint64

	// OnEvict is evict callback function.
	OnEvict OnEvictCallback
}

// DefaultOption
var DefaultOption = Option{
	ShardCount:        4096,
	DefaultIdxMapSize: 32,
	DefaultBufferSize: 1024,
	MaxProbeCount:     1000,
	MaxFailCount:      3,
	SCacheSize:        8,
	MigrateThresRatio: 0.6,
	MigrateDelta:      4 * 1 << 10, // 4 KB
}
