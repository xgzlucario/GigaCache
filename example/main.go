package main

import (
	"bytes"
	"fmt"
	"time"
	"unsafe"

	"net/http"
	_ "net/http/pprof"

	"github.com/brianvoe/gofakeit/v6"
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
	go http.ListenAndServe("localhost:6060", nil)

	a := time.Now()

	var sum float64
	var stat, count int64

	bc := cache.NewGigaCache[string]()

	// Stat
	go func() {
		for {
			time.Sleep(time.Second * 10)
			fmt.Printf("[Cache] %.1fs\t count: %dk\t num: %dk\t avg: %.2f ns\n",
				time.Since(a).Seconds(), count/1000, bc.Len()/1000, sum/float64(stat))
		}
	}()

	// Get
	go func() {
		for {
			a := time.Now()
			ph := gofakeit.Phone()

			val, ok := bc.Get(ph)
			if ok && !bytes.Equal(S2B(&ph), val) {
				panic("key and value not equal")
			}

			c := time.Since(a).Microseconds()
			sum += float64(c)
			stat++

			time.Sleep(time.Microsecond)
		}
	}()

	// Set
	for {
		count++
		v := gofakeit.Phone()
		bc.SetEx(v, S2B(&v), time.Second*10)
	}
}
