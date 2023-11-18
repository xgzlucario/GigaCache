package cache

import (
	"sync/atomic"
	"time"
)

var (
	// now unixnano time
	clock int64
)

func init() {
	clock = time.Now().UnixNano()

	go func() {
		ticker := time.NewTicker(time.Microsecond)
		for t := range ticker.C {
			atomic.StoreInt64(&clock, t.UnixNano())
		}
	}()
}

// GetClock return now unixnano time.
func GetClock() int64 {
	return atomic.LoadInt64(&clock)
}
