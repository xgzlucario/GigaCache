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

func main() {
	go http.ListenAndServe("localhost:6060", nil)

	a := time.Now()

	var sum float64
	var stat, count int64
	var mem runtime.MemStats

	bc := cache.NewGigaCache[string]()

	// Test
	// for i := 0; i < 10; i++ {
	// 	bc.SetEx("xgz"+strconv.Itoa(i), []byte("1"), time.Second*time.Duration(i))
	// }

	// for i := 0; i < 10; i++ {
	// 	fmt.Println()
	// 	for i := 0; i < 10; i++ {
	// 		c, ts, ok := bc.GetTx("xgz" + strconv.Itoa(i))
	// 		fmt.Println(string(c), time.Unix(0, ts), ok)
	// 	}
	// 	time.Sleep(time.Second)
	// }

	// Stat
	go func() {
		for {
			time.Sleep(time.Second)
			runtime.ReadMemStats(&mem)

			fmt.Printf("[Cache] %.1fs\t count: %dk\t num: %dk\t mem: %d MB\t avg: %.2f ns\n",
				time.Since(a).Seconds(), count/1e3, bc.Len()/1e3, mem.HeapAlloc/1e6, sum/float64(stat))
		}
	}()

	// Get
	go func() {
		for i := 0; ; i++ {
			a := time.Now()
			ph := strconv.Itoa(i)

			val, _, ok := bc.GetTx(ph)
			if ok && !bytes.Equal(S2B(&ph), val) {
				panic("key and value not equal")

			}

			c := time.Since(a).Microseconds()
			sum += float64(c)
			stat++

			time.Sleep(time.Microsecond)

			i %= 1e6
		}
	}()

	// Set
	for i := 0; ; i++ {
		count++
		v := strconv.Itoa(i)
		bc.SetEx(v, S2B(&v), time.Second*10)
	}
}
