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

func testBytes() {
	bc := cache.New[string]()

	// Test
	for i := 1; i < 20; i++ {
		bc.SetEx("xgz"+strconv.Itoa(i), []byte(strconv.Itoa(i)), time.Second/10*time.Duration(i))
	}

	for i := 0; i < 25; i++ {
		bc.Scan(func(key string, val any, ts int64) bool {
			fmt.Println(key, string(val.([]byte)), time.Unix(0, ts).Format(time.DateTime))
			return true
		})
		fmt.Println()
		time.Sleep(time.Second / 10)
	}
}

func testAny() {
	bc := cache.New[string]()

	// Test
	for i := 1; i < 20; i++ {
		bc.SetAnyEx("xgz-any"+strconv.Itoa(i), i, time.Second/10*time.Duration(i))
	}

	for i := 0; i < 25; i++ {
		bc.Scan(func(key string, val any, ts int64) bool {
			fmt.Println(key, val, time.Unix(0, ts).Format(time.DateTime))
			return true
		})
		fmt.Println()
		time.Sleep(time.Second / 10)
	}
}

func main() {
	go http.ListenAndServe("localhost:6060", nil)

	testBytes()
	testAny()

	start := time.Now()

	var sum float64
	var n1, count int64

	bc := cache.New[string]()

	// Stat
	go func() {
		for i := 0; ; i++ {
			time.Sleep(time.Second / 10)

			// benchmark test
			if i > 0 && i%10 == 0 {
				stat := bc.Stat()
				fmt.Printf("[Cache] %.0fs | count: %dw | len: %dw | alloc: %dw | bytes: %dw | any: %dw | rate: %.1f%% | ccount: %d | avg: %.2f ns\n",
					time.Since(start).Seconds(),
					count/1e4,
					stat.Len/1e4,
					stat.AllocLen/1e4,
					stat.BytesLen/1e4,
					stat.AnyLen/1e4,
					stat.ExpRate(),
					stat.CCount,
					sum/float64(n1))
			}

			// marshal test
			if i > 0 && i%100 == 0 {
				stat := bc.Stat()
				before := time.Now()
				data, err := bc.MarshalBytes()
				if err != nil {
					panic(err)
				}
				fmt.Printf("[Marshal] len: %vw | cost: %v | len: %.2fM\n",
					stat.Len/1e4,
					time.Since(before),
					float64(len(data))/1024/1024/8)
			}
		}
	}()

	// Get
	go func() {
		for i := 0; ; i++ {
			now := time.Now()
			key := strconv.Itoa(i)

			if i%2 == 0 {
				val, _, ok := bc.Get(key)
				if ok && !bytes.Equal(S2B(&key), val) {
					panic("key and value not equal")
				}

			} else {
				val, _, ok := bc.GetAny(key)
				if ok && !bytes.Equal(S2B(&key), val.([]byte)) {
					panic("key and value not equal")
				}
			}

			c := time.Since(now).Microseconds()
			sum += float64(c)
			n1++

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
