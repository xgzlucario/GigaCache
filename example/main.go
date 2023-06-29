package main

import (
	"bytes"
	"fmt"
	"runtime"
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
	// for {
	// 	ww := time.Now().UnixMilli()
	// 	fmt.Println("====================================")

	// 	// millisecond(32 x1)(16 x2)
	// 	x1, x2 := uint32(ww>>16), uint16(ww&math.MaxUint16)

	// 	fmt.Println(ww, x1, x2)
	// 	fmt.Println(ww, uint64(x1)<<16|uint64(x2))

	// 	time.Sleep(time.Second)
	// }

	go http.ListenAndServe("localhost:6060", nil)

	a := time.Now()

	var sum float64
	var stat, count int64
	var mem runtime.MemStats

	bc := cache.NewGigaCache[string]()

	// Test
	bc.SetEx("xgz", []byte("1ds"), time.Hour*77)
	c, ts, ok := bc.GetTx("xgz")
	fmt.Println(string(c), time.Unix(0, ts), time.Now().Add(time.Hour*77), ok)

	bc.SetEx("xgz1", []byte("1ds"), time.Second)
	time.Sleep(time.Second + time.Millisecond)
	c, ts, ok = bc.GetTx("xgz1")
	fmt.Println(string(c), time.Unix(0, ts), ok)

	// Stat
	go func() {
		for {
			time.Sleep(time.Second)
			runtime.ReadMemStats(&mem)

			fmt.Printf("[Cache] %.1fs\t count: %dk\t num: %dk\t mem: %d MB\t avg: %.2f ns\n",
				time.Since(a).Seconds(),
				count/1000,
				bc.Len()/1000,
				mem.HeapAlloc/1e6,
				sum/float64(stat))
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
