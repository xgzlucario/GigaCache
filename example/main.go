package main

import (
	"fmt"
	"runtime"
	"strconv"
	"time"

	"golang.org/x/exp/rand"

	"net/http"
	_ "net/http/pprof"

	cache "github.com/xgzlucario/GigaCache"
)

/*
	1024
	[Cache] 101s / 41927w | len: 535w | alloc: 691w | bytes: 12490w | rate: 77.9% | mtime: 84438
	[Mem] mem: 925MB | sys: 1251MB | object: 1662w | gc: 109 | gcpause: 238 us
	[Latency]
	avg: 1.04 | min: 0.08 | p50: 0.38 | p95: 1.85 | p99: 3.47 | max: 2351.58

	2048
	[Cache] 101s / 43006w | len: 569w | alloc: 730w | bytes: 12847w | rate: 78.0% | mtime: 169595
	[Mem] mem: 778MB | sys: 1173MB | object: 1304w | gc: 114 | gcpause: 257 us
	[Latency]
	avg: 0.85 | min: 0.08 | p50: 0.37 | p95: 1.65 | p99: 2.99 | max: 638.79

	4096
	[Cache] 101s / 46147w | len: 616w | alloc: 794w | bytes: 13859w | rate: 77.9% | mtime: 343968
	[Mem] mem: 1060MB | sys: 1261MB | object: 2204w | gc: 112 | gcpause: 248 us
	[Latency]
	avg: 0.77 | min: 0.08 | p50: 0.40 | p95: 1.65 | p99: 2.90 | max: 397.24
*/

func main() {
	go http.ListenAndServe("localhost:6060", nil)

	start := time.Now()

	pset := cache.NewPercentile()

	var count int64
	var avgRate, avgBytes, avgTime float64
	var memStats runtime.MemStats

	bc := cache.New[string](4096)

	// Stat
	go func() {
		for i := 0; ; i++ {
			time.Sleep(time.Second / 10)

			// benchmark test
			if i > 0 && i%100 == 0 {
				stat := bc.Stat()

				avgRate += stat.ExpRate()
				avgBytes += float64(stat.LenBytes)
				avgTime++

				// Stats
				fmt.Printf("[Cache] %.0fs / %dw | len: %dw | alloc: %dw | bytes: %.0fw | rate: %.1f%% | mtime: %d\n",
					time.Since(start).Seconds(),
					count/1e4,
					stat.Len/1e4,
					stat.Alloc/1e4,
					avgBytes/avgTime/1e4,
					avgRate/avgTime,
					stat.MigrateTimes)

				// mem stats
				runtime.ReadMemStats(&memStats)

				fmt.Printf("[Mem] mem: %.0fMB | sys: %.0fMB | object: %.0fw | gc: %d | gcpause: %.0f us\n",
					float64(memStats.Alloc)/1024/1024,
					float64(memStats.Sys)/1024/1024,
					float64(memStats.HeapObjects)/1e4,
					memStats.NumGC,
					float64(memStats.PauseTotalNs)/float64(memStats.NumGC)/1000)

				// latency
				fmt.Println("[Latency]")
				pset.Print()

				fmt.Println("-----------------------------------------------------")
			}
		}
	}()

	// 8 clients set concurrent
	for i := 0; i < 8; i++ {
		go func() {
			for {
				k := strconv.Itoa(int(rand.Uint32()))
				now := time.Now()

				bc.SetEx(k, []byte(k), time.Second)
				count++

				pset.Add(float64(time.Since(now)) / float64(time.Microsecond))
			}
		}()
	}

	select {}
}
