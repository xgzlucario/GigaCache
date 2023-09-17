package cache

type CacheStat struct {
	Len      uint64
	Count    uint64
	BytesLen uint64
	AnyLen   uint64
	CCount   uint64
}

// Stat
func (c *GigaCache[K]) Stat() (stat CacheStat) {
	for _, b := range c.buckets {
		b.RLock()
		stat.Count += uint64(b.count)
		stat.Len += uint64(b.idx.Len())
		stat.BytesLen += uint64(b.bytes.Len())
		stat.AnyLen += uint64(len(b.anyArr))
		stat.CCount += uint64(b.ccount)
		b.RUnlock()
	}
	return
}

// ExpRate
func (s CacheStat) ExpRate() float64 {
	return float64(s.Len) / float64(s.Count) * 100
}
