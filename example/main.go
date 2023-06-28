package main

import (
	"bytes"
	"fmt"
	"strconv"
	"time"
	"unsafe"

	"net/http"
	_ "net/http/pprof"

	cache "github.com/xgzlucario/GigaCache"
)

// String convert to bytes unsafe
func S2B(str *string) []byte {
	strHeader := (*[2]uintptr)(unsafe.Pointer(str))
	byteSliceHeader := [3]uintptr{
		strHeader[0], strHeader[1], strHeader[1],
	}
	return *(*[]byte)(unsafe.Pointer(&byteSliceHeader))
}

// Bytes convert to string unsafe
func B2S(buf []byte) *string {
	return (*string)(unsafe.Pointer(&buf))
}

func main() {
	// tt := time.Now().Add(time.Hour * 24 * 365 * 85)
	// as := tt.UnixMilli()
	// buf := binary.AppendUvarint(nil, uint64(as))
	// fmt.Println(tt, as, buf)
	// fmt.Println(binary.Uvarint(buf))
	// fmt.Println()

	// fmt.Println("==========================")

	go http.ListenAndServe("localhost:6060", nil)

	a := time.Now()

	var sum float64
	var stat, count int64

	bc := cache.NewGigaCache[string]()

	// Stat
	go func() {
		for {
			time.Sleep(time.Second)
			fmt.Printf("[Cache] %.1fs\t count: %dk\t num: %dk\t avg: %.2f ns\n",
				time.Since(a).Seconds(), count/1000, bc.Len()/1000, sum/float64(stat))
		}
	}()

	// Get
	go func() {
		for i := 0; ; i++ {
			a := time.Now()
			ph := strconv.Itoa(i)

			val, ok := bc.Get(ph)
			if ok && !bytes.Equal(S2B(&ph), val) {
				panic("key and value not equal")
			}

			c := time.Since(a).Microseconds()
			sum += float64(c)
			stat++

			time.Sleep(time.Microsecond)

			i %= 99999
		}
	}()

	// Set
	for i := 0; ; i++ {
		count++
		v := strconv.Itoa(i)
		bc.SetEx(v, S2B(&v), time.Second*10)
	}
}
