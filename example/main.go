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
				fmt.Printf("90th = %.2f us\n", td.Quantile(0.9))
				fmt.Printf("99th = %.2f us\n", td.Quantile(0.99))
				fmt.Printf("100th = %.2f us\n", td.Quantile(0.9999))
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

		bc.SetEx(k, []byte(k), time.Second)
		count++

		cost := float64(time.Since(now)) / float64(time.Microsecond)
		tdlock.Lock()
		td.Add(cost, 1)
		tdlock.Unlock()
	}
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
