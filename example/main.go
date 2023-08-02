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

	bc := cache.New[string]()

	// Test
	// for i := 1; i < 10; i++ {
	// 	bc.SetEx("xgz"+strconv.Itoa(i), []byte(strconv.Itoa(i)), time.Second*time.Duration(i))
	// }

	// for i := 0; i < 11; i++ {
	// 	bc.Scan(func(key string, val []byte, ts int64) {
	// 		fmt.Println("xgz"+strconv.Itoa(i), string(val), time.Unix(0, ts).Format(time.DateTime))
	// 	})
	// 	fmt.Println()
	// 	time.Sleep(time.Second)
	// }

	// Stat
	var maxNum int
	go func() {
		for i := 0; ; i++ {
			time.Sleep(time.Second / 10)
			runtime.ReadMemStats(&mem)

			n := bc.Len() / 1e3
			if n > maxNum {
				maxNum = n
			}

			if i%100 == 0 {
				fmt.Printf("[Cache] %.0fs\t count: %dk\t num: %dk\t maxNum: %dk\t avg: %.2f ns\n",
					time.Since(a).Seconds(), count/1e3, n, maxNum, sum/float64(stat))
			}
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

			i %= 1e9
		}
	}()

	// Set
	for i := 0; ; i++ {
		count++
		v := strconv.Itoa(i)
		bc.SetEx(v, S2B(&v), time.Second)
	}
}
