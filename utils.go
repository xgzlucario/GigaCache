package cache

import (
	"sync/atomic"
	"time"
	"unsafe"
)

var (
	// now nano time
	nanosec atomic.Int64
	sec     atomic.Uint32
)

//go:linkname FastRand runtime.fastrand
func FastRand() uint32

//go:linkname FastRand64 runtime.fastrand64
func FastRand64() uint64

type stringStruct struct {
	str unsafe.Pointer
	len int
}

//go:noescape
//go:linkname memhash runtime.memhash
func memhash(p unsafe.Pointer, h, s uintptr) uintptr

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

// GetNanoSec returns the current nano time.
func GetNanoSec() int64 {
	return nanosec.Load()
}

// GetSec returns the current unix time.
func GetSec() uint32 {
	return sec.Load()
}

type HashFn func(string) uint64

// MemHash is the hash function used by go map, it utilizes available hardware instructions
// (behaves as aes hash if aes instruction is available).
// NOTE: The hash seed changes for every process. So, this cannot be used as a persistent hash.
func MemHash(str string) uint64 {
	ss := (*stringStruct)(unsafe.Pointer(&str))
	return uint64(memhash(ss.str, 0, uintptr(ss.len)))
}

func s2b(str *string) []byte {
	strHeader := (*[2]uintptr)(unsafe.Pointer(str))
	byteSliceHeader := [3]uintptr{
		strHeader[0], strHeader[1], strHeader[1],
	}
	return *(*[]byte)(unsafe.Pointer(&byteSliceHeader))
}

func b2s(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}
