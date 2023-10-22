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
	5secs 1024
	[Cache] 201s / 99236w | len: 3126w | alloc: 4273w | bytes: 68195w | rate: 87.0% | mtime: 39936
	[Mem] mem: 3257MB | sys: 5941MB | gc: 97 | gcpause: 238 us
	[Latency]
	avg: 0.44 | min: 0.14 | p50: 0.36 | p95: 0.70 | p99: 1.02 | max: 656.02

	5secs 2048
	[Cache] 201s / 103751w | len: 3228w | alloc: 4551w | bytes: 72911w | rate: 85.6% | mtime: 79872
	[Mem] mem: 4078MB | sys: 6067MB | gc: 98 | gcpause: 273 us
	[Latency]
	avg: 0.46 | min: 0.14 | p50: 0.38 | p95: 0.74 | p99: 1.05 | max: 328.07

	5secs 4096
	[Cache] 201s / 104891w | len: 3330w | alloc: 4755w | bytes: 74831w | rate: 85.0% | mtime: 159745
	[Mem] mem: 4289MB | sys: 6110MB | gc: 98 | gcpause: 272 us
	[Latency]
	avg: 0.41 | min: 0.11 | p50: 0.36 | p95: 0.66 | p99: 0.90 | max: 133.64
*/

func main() {
	go http.ListenAndServe("localhost:6060", nil)

	start := time.Now()
	pset := cache.NewPercentile()

	var count int64
	var avgRate, avgBytes, avgTime float64
	var memStats runtime.MemStats

	bc := cache.New()

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
				fmt.Printf("[Mem] mem: %.0fMB | sys: %.0fMB | gc: %d | gcpause: %.0f us\n",
					float64(memStats.Alloc)/1024/1024,
					float64(memStats.Sys)/1024/1024,
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

				bc.SetEx(k, []byte(k), time.Second*5)
				count++

				pset.Add(float64(time.Since(now)) / float64(time.Microsecond))
			}
		}()
	}

	// Marshal test
	// go func() {
	// 	for {
	// 		time.Sleep(time.Second * 8)
	// 		a := time.Now()
	// 		bc.MarshalBytes()
	// 		fmt.Println("Marshal cost:", time.Since(a))
	// 	}
	// }()

	select {}
}
