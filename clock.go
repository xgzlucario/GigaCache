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

// GetUnix return now unixnano time.
func GetUnixNano() int64 {
	return clock
}
