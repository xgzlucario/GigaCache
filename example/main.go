package main

import (
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
		bc.SetEx("xgz-any"+strconv.Itoa(i), i, time.Second/10*time.Duration(i))
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

	// testBytes()
	// testAny()

	start := time.Now()

	delays := make([]float64, 0, 1e5)
	var count int64

	bc := cache.New[string]()

	var c float64
	var sumRate, sumBytesLen float64

	// Stat
	go func() {
		for i := 0; ; i++ {
			time.Sleep(time.Second / 10)

			// benchmark test
			if i > 0 && i%100 == 0 {
				stat := bc.Stat()

				c++
				sumRate += stat.ExpRate()
				sumBytesLen += float64(stat.BytesLen)

				// Stats
				fmt.Printf("[Cache] %.0fs | count: %dw | len: %dw | alloc: %dw | bytes: %.0fw | rate: %.1f%% | ccount: %d\n",
					time.Since(start).Seconds(),
					count/1e4,
					stat.Len/1e4,
					stat.Count/1e4,
					sumBytesLen/c/1e4,
					sumRate/c,
					stat.CCount)

				// P99
				cache.Sort(delays)
				fmt.Printf("[P99 SET] avg: %v | min: %v | p50: %v | p95: %v | p99: %v | max: %v\n",
					time.Duration(cache.Avg(delays)),
					time.Duration(cache.Min(delays)),
					time.Duration(cache.CalculatePercentile(delays, 50)),
					time.Duration(cache.CalculatePercentile(delays, 95)),
					time.Duration(cache.CalculatePercentile(delays, 99)),
					time.Duration(cache.Max(delays)))

				fmt.Println()
			}
		}
	}()

	// Set
	for i := 0; ; i++ {
		count++
		v := strconv.Itoa(i)
		now := time.Now()

		bc.SetEx(v, S2B(&v), time.Second)

		if len(delays) >= 1e5 {
			delays[i%1e5] = float64(time.Since(now))
		} else {
			delays = append(delays, float64(time.Since(now)))
		}
	}
}
