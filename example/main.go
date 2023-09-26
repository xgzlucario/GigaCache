package main

import (
	"fmt"
	"strconv"
	"time"

	"net/http"
	_ "net/http/pprof"

	cache "github.com/xgzlucario/GigaCache"
)

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

	pset := cache.NewPercentile()
	prate := cache.NewPercentile()
	pbytes := cache.NewPercentile()

	var count int64

	bc := cache.New[string]()

	// Stat
	go func() {
		for i := 0; ; i++ {
			time.Sleep(time.Second / 10)

			// benchmark test
			if i > 0 && i%20 == 0 {
				stat := bc.Stat()

				prate.Add(stat.ExpRate())
				pbytes.Add(float64(stat.LenBytes))

				// Stats
				fmt.Printf("Cache [%.0fs] [%dw] | len: %dw | alloc: %dw | bytes: %.0fw | rate: %.1f%% | mtime: %d\n",
					time.Since(start).Seconds(),
					count/1e4,
					stat.Len/1e4,
					stat.AllocTimes/1e4,
					pbytes.Avg()/1e4,
					prate.Avg(),
					stat.MigrateTimes)

				// latency
				fmt.Println("latency(micros)")
				pset.Print()

				fmt.Println()
			}
		}
	}()

	// Set
	for i := 0; ; i++ {
		count++
		v := strconv.Itoa(i)
		now := time.Now()

		bc.SetEx(v, []byte(v), time.Second)

		pset.Add(float64(time.Since(now)) / float64(time.Microsecond))
	}
}
