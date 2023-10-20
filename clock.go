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
		ticker := time.NewTicker(time.Millisecond)
		for t := range ticker.C {
			atomic.StoreInt64(&clock, t.UnixNano())
		}
	}()
}

// getClock return now unixnano time.
func getClock() int64 {
	return atomic.LoadInt64(&clock)
}
