package main

import (
	"fmt"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"time"

	"golang.org/x/exp/rand"

	"net/http"
	_ "net/http/pprof"

	cache "github.com/xgzlucario/GigaCache"
)

type Quantile struct {
	mu sync.RWMutex
	f  []float64
}

func NewQuantile(size int) *Quantile {
	return &Quantile{f: make([]float64, 0, size)}
}

func (q *Quantile) Add(v float64) {
	q.mu.Lock()
	q.f = append(q.f, v)
	q.mu.Unlock()
}

func (q *Quantile) Quantile(p float64) float64 {
	q.mu.RLock()
	r := q.f[int(float64(len(q.f))*p)]
	q.mu.RUnlock()
	return r
}

func (q *Quantile) Print() {
	q.mu.Lock()
	slices.Sort(q.f)
	fmt.Printf("50th: %.0f ns\n", q.Quantile(0.5))
	fmt.Printf("90th: %.0f ns\n", q.Quantile(0.9))
	fmt.Printf("99th: %.0f ns\n", q.Quantile(0.99))
	fmt.Printf("999th: %.0f ns\n", q.Quantile(0.999))
	q.mu.Unlock()
}

const N = 100 * 10000

func main() {
	go func() {
		_ = http.ListenAndServe("localhost:6060", nil)
	}()

	start := time.Now()
	quant := NewQuantile(N)

	var count int64
	var avgRate, avgAlloc, avgInused, avgTime float64
	var memStats runtime.MemStats

	bc := cache.New(cache.DefaultOptions)

	// Stat
	go func() {
		for i := 0; ; i++ {
			time.Sleep(time.Second / 10)

			// benchmark test
			if i > 0 && i%100 == 0 {
				stat := bc.Stat()

				avgRate += stat.ExpRate()
				avgAlloc += float64(stat.Alloc)
				avgInused += float64(stat.Inused)
				avgTime++

				// Stats
				fmt.Printf("[Cache] %.0fs | %dw | len: %dw | alloc: %v / %v (%.1f%%)\n",
					time.Since(start).Seconds(),
					count/1e4,
					stat.Len/1e4,
					formatSize(avgInused/avgTime), formatSize(avgAlloc/avgTime),
					avgRate/avgTime,
				)
				fmt.Printf("[Evict] probe: %vw / %vw (%.1f%%) | mgr: %d\n",
					stat.Evict/1e5, stat.Probe/1e5, stat.EvictRate(),
					stat.Migrates)

				// mem stats
				runtime.ReadMemStats(&memStats)
				fmt.Printf("[Mem] mem: %.0fMB | sys: %.0fMB | gc: %d | gcpause: %.0f us\n",
					float64(memStats.Alloc)/1024/1024,
					float64(memStats.Sys)/1024/1024,
					memStats.NumGC,
					float64(memStats.PauseTotalNs)/float64(memStats.NumGC)/1000)

				fmt.Println("-----------------------------------------------------")
			}
		}
	}()

	source := rand.NewSource(uint64(time.Now().UnixNano()))

	// set test
	for j := 0; ; j++ {
		k := strconv.Itoa(int(source.Uint64() >> 32))

		now := time.Now()

		bc.SetEx(k, []byte(k), time.Second*10)
		count++

		cost := float64(time.Since(now)) / float64(time.Microsecond)
		quant.Add(cost)
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
