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
	5sec 1024
	[Cache] 201s / 70665w | len: 2284w | alloc: 2894w | bytes: 52827w | rate: 77.6% | mtime: 33098
	[Mem] mem: 3659MB | sys: 6103MB | gc: 59 | gcpause: 236 us
	[Latency]
	avg: 1.50 | min: 0.09 | p50: 0.40 | p95: 1.87 | p99: 3.34 | max: 2992.52

	5sec 2048
	[Cache] 201s / 74424w | len: 2384w | alloc: 3056w | bytes: 55520w | rate: 77.4% | mtime: 66862
	[Mem] mem: 4367MB | sys: 5080MB | gc: 60 | gcpause: 262 us
	[Latency]
	avg: 1.23 | min: 0.08 | p50: 0.44 | p95: 1.99 | p99: 3.56 | max: 2692.78

	5sec 4096
	[Cache] 201s / 77253w | len: 2477w | alloc: 3096w | bytes: 56733w | rate: 78.2% | mtime: 134800
	[Mem] mem: 2637MB | sys: 5206MB | gc: 61 | gcpause: 258 us
	[Latency]
	avg: 1.03 | min: 0.09 | p50: 0.49 | p95: 2.05 | p99: 3.72 | max: 2133.35
*/

func main() {
	go http.ListenAndServe("localhost:6060", nil)

	start := time.Now()
	pset := cache.NewPercentile()

	var count int64
	var avgRate, avgBytes, avgTime float64
	var memStats runtime.MemStats

	bc := cache.New[string]()

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
