package main

import (
	"flag"
	"fmt"
	"runtime"
	"runtime/debug"
	"time"

	cache "github.com/xgzlucario/GigaCache"
)

var previousPause time.Duration

func gcPause() time.Duration {
	runtime.GC()
	var stats debug.GCStats
	debug.ReadGCStats(&stats)
	pause := stats.PauseTotal - previousPause
	previousPause = stats.PauseTotal
	return pause
}

func genKV(id int) (string, []byte) {
	k := fmt.Sprintf("%08x", id)
	return k, []byte(k)
}

func main() {
	c := ""
	entries := 0
	flag.StringVar(&c, "cache", "gigacache", "cache to bench.")
	flag.IntVar(&entries, "entries", 2000*10000, "number of entries to test")
	flag.Parse()

	fmt.Println(c)
	fmt.Println("entries:", entries)

	start := time.Now()
	switch c {
	case "gigacache":
		cache := cache.New(cache.DefaultOption)
		for i := 0; i < entries; i++ {
			k, v := genKV(i)
			cache.Set(k, v)
		}
	case "stdmap":
		m := make(map[string][]byte)
		for i := 0; i < entries; i++ {
			k, v := genKV(i)
			m[k] = v
		}
	}
	cost := time.Since(start)

	var mem runtime.MemStats
	var stat debug.GCStats

	runtime.ReadMemStats(&mem)
	debug.ReadGCStats(&stat)

	fmt.Println("alloc:", mem.Alloc/1024/1024, "mb")
	fmt.Println("gcsys:", mem.GCSys/1024/1024, "mb")
	fmt.Println("heap inuse:", mem.HeapInuse/1024/1024, "mb")
	fmt.Println("heap object:", mem.HeapObjects/1024, "k")
	fmt.Println("gc:", stat.NumGC)
	fmt.Println("pause:", gcPause())
	fmt.Println("cost:", cost)
}
