package cache

import (
	"sync/atomic"
	"time"
)

var (
	// now unixnano time
	nanosec atomic.Int64
	sec     atomic.Uint32
)

func init() {
	now := time.Now()
	nanosec.Store(now.UnixNano())
	sec.Store(uint32(now.Unix()))

	go func() {
		ticker := time.NewTicker(time.Millisecond)
		for t := range ticker.C {
			nanosec.Store(t.UnixNano())
			sec.Store(uint32(t.Unix()))
		}
	}()
}

// GetNanoSec returns the current unixnano time.
func GetNanoSec() int64 {
	return nanosec.Load()
}

// GetSec returns the current unix time.
func GetSec() uint32 {
	return sec.Load()
}
