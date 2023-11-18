package cache

import "unsafe"

// s2b is string convert to bytes unsafe.
func s2b(str *string) []byte {
	strHeader := (*[2]uintptr)(unsafe.Pointer(str))
	byteSliceHeader := [3]uintptr{
		strHeader[0], strHeader[1], strHeader[1],
	}
	return *(*[]byte)(unsafe.Pointer(&byteSliceHeader))
}

// b2s is bytes convert to string unsafe.
func b2s(buf []byte) *string {
	return (*string)(unsafe.Pointer(&buf))
}
