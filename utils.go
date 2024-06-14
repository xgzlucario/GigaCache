package cache

import (
	"math/bits"
	"unsafe"
)

//go:linkname FastRand runtime.fastrand
func FastRand() uint32

//go:linkname FastRand64 runtime.fastrand64
func FastRand64() uint64

func s2b(str *string) []byte {
	strHeader := (*[2]uintptr)(unsafe.Pointer(str))
	byteSliceHeader := [3]uintptr{
		strHeader[0], strHeader[1], strHeader[1],
	}
	return *(*[]byte)(unsafe.Pointer(&byteSliceHeader))
}

// SizeUvarint
// See https://go-review.googlesource.com/c/go/+/572196/1/src/encoding/binary/varint.go#174
func SizeUvarint(x uint64) int {
	return int(9*uint32(bits.Len64(x))+64) / 64
}
