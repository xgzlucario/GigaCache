package cache

import (
	"sync/atomic"
	"time"
)

var (
	zeroUnix     int64
	zeroUnixNano int64

	// now secs since zeroUnix
	clock uint32
)

func init() {
	zt, _ := time.Parse(time.DateOnly, "2023-08-01")
	zeroUnix = zt.Unix()
	zeroUnixNano = zt.UnixNano()
	clock = uint32(time.Now().Unix() - zeroUnix)

	go func() {
		ticker := time.NewTicker(time.Millisecond)
		for t := range ticker.C {
			atomic.StoreUint32(&clock, uint32(t.Unix()-zeroUnix))
		}
	}()
}

// GetUnix return now unix time.
func GetUnix() int64 {
	return zeroUnix
}

// GetUnixNano return now unixnano time.
func GetUnixNano() int64 {
	return zeroUnixNano
}

// SetZeroTime
func SetZeroTime(secs int64) {
	zt := time.Unix(0, secs)
	zeroUnix = zt.Unix()
	zeroUnixNano = zt.UnixNano()
}
