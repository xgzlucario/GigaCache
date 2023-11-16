package main

import (
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"time"

	"golang.org/x/exp/rand"

	"net/http"
	_ "net/http/pprof"

	"github.com/influxdata/tdigest"
	cache "github.com/xgzlucario/GigaCache"
)

/*
	(lastEvict 20ms)
	[Cache] 201s | 74094w | len: 2403w | alloc: 485.4MB / 627.6MB (78.7%)
	[Evict] probe: 4862w / 13071w (37.2%) | mtime: 25754
	[Mem] mem: 2850MB | sys: 5619MB | gc: 72 | gcpause: 386 us
	[Latency]
	avg: 1.07 | min: 0.20 | p50: 0.62 | p95: 0.99 | p99: 1.88 | max: 2934.09

	(default)
	[Cache] 201s | 71051w | len: 2037w | alloc: 455.9MB / 591.0MB (79.7%)
	[Evict] probe: 4841w / 15215w (31.8%) | mtime: 27650
	[Mem] mem: 4205MB | sys: 5410MB | gc: 73 | gcpause: 369 us
	[Latency]
	avg: 1.57 | min: 0.20 | p50: 0.69 | p95: 1.27 | p99: 3.01 | max: 2089.43
*/

var tdlock sync.Mutex

func main() {
	go http.ListenAndServe("localhost:6060", nil)

	start := time.Now()
	td := tdigest.NewWithCompression(1000)

	var count int64
	var avgRate, avgAlloc, avgInused, avgTime float64
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
				avgAlloc += float64(stat.BytesAlloc)
				avgInused += float64(stat.BytesInused)
				avgTime++

				// Stats
				fmt.Printf("[Cache] %.0fs | %dw | len: %dw | alloc: %v / %v (%.1f%%)\n",
					time.Since(start).Seconds(),
					count/1e4,
					stat.Len/1e4,
					formatSize(avgInused/avgTime), formatSize(avgAlloc/avgTime),
					avgRate/avgTime,
				)
				fmt.Printf("[Evict] probe: %vw / %vw (%.1f%%) | mtime: %d\n",
					stat.EvictCount/1e5, stat.ProbeCount/1e5, stat.EvictRate(),
					stat.MigrateTimes)

				// mem stats
				runtime.ReadMemStats(&memStats)
				fmt.Printf("[Mem] mem: %.0fMB | sys: %.0fMB | gc: %d | gcpause: %.0f us\n",
					float64(memStats.Alloc)/1024/1024,
					float64(memStats.Sys)/1024/1024,
					memStats.NumGC,
					float64(memStats.PauseTotalNs)/float64(memStats.NumGC)/1000)

				// compute quantiles
				tdlock.Lock()
				fmt.Printf("90th = %.2f ms\n", td.Quantile(0.9))
				fmt.Printf("99th = %.2f ms\n", td.Quantile(0.99))
				fmt.Printf("100th = %.2f ms\n", td.Quantile(0.9999))
				tdlock.Unlock()

				fmt.Println("-----------------------------------------------------")
			}
		}
	}()

	source := rand.NewSource(uint64(time.Now().UnixNano()))

	// set test
	for j := 0; ; j++ {
		k := strconv.Itoa(int(source.Uint64() >> 32))
		now := time.Now()

		bc.SetEx(k, []byte(k), time.Second*5)
		count++

		cost := float64(time.Since(now)) / float64(time.Microsecond)
		tdlock.Lock()
		td.Add(cost, 1)
		tdlock.Unlock()
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

const (
	KB = 1024
	MB = 1024 * KB
)

// formatSize
func formatSize(size float64) string {
	switch {
	case size < KB:
		return fmt.Sprintf("%.0fB", size)
	case size < MB:
		return fmt.Sprintf("%.1fKB", float64(size)/KB)
	default:
		return fmt.Sprintf("%.1fMB", float64(size)/MB)
	}
}
