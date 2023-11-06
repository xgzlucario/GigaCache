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
	[Cache] 201s | 96826w | len: 2817w | alloc: 566.8MB / 751.4MB | rate: 77.0% | mtime: 24697
	[Mem] mem: 2558MB | sys: 6459MB | gc: 75 | gcpause: 255 us
	[Latency]
	avg: 0.69 | min: 0.16 | p50: 0.41 | p95: 0.77 | p99: 1.30 | max: 3007.34

	5secs 2048
	[Cache] 201s | 100085w | len: 3247w | alloc: 586.5MB / 777.0MB | rate: 77.3% | mtime: 49209
	[Mem] mem: 3549MB | sys: 6506MB | gc: 74 | gcpause: 268 us
	[Latency]
	avg: 0.53 | min: 0.13 | p50: 0.42 | p95: 0.78 | p99: 1.10 | max: 1238.77

	5secs 4096
	[Cache] 201s | 96791w | len: 3074w | alloc: 568.2MB / 752.4MB | rate: 77.1% | mtime: 98946
	[Mem] mem: 4497MB | sys: 5961MB | gc: 74 | gcpause: 297 us
	[Latency]
	avg: 0.55 | min: 0.11 | p50: 0.39 | p95: 0.74 | p99: 1.03 | max: 1019.75
*/

func main() {
	go http.ListenAndServe("localhost:6060", nil)

	start := time.Now()
	pset := cache.NewPercentile()

	var count int64
	var avgRate, avgAlloc, avgInused, avgTime float64
	var memStats runtime.MemStats

	bc := cache.New(4096)

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
				fmt.Printf("[Cache] %.0fs | %dw | len: %dw | alloc: %v / %v | rate: %.1f%% | mtime: %d\n",
					time.Since(start).Seconds(),
					count/1e4,
					stat.Len/1e4,
					formatSize(avgInused/avgTime),
					formatSize(avgAlloc/avgTime),
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
