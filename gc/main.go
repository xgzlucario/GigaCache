package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	cache "github.com/xgzlucario/GigaCache"
)

var previousPause time.Duration

func gcPause() time.Duration {
	runtime.GC()

	var stats debug.GCStats
	var memStats runtime.MemStats
	debug.ReadGCStats(&stats)
	runtime.ReadMemStats(&memStats)

	fmt.Printf("Heap Objects Total: %d\n", memStats.HeapObjects)

	pause := stats.PauseTotal - previousPause
	previousPause = stats.PauseTotal
	return pause
}

func main() {
	c := ""
	entries := 0
	repeat := 0
	valueSize := 0
	flag.StringVar(&c, "cache", "bigcache", "cache to bench.")
	flag.IntVar(&entries, "entries", 20000000, "number of entries to test")
	flag.IntVar(&repeat, "repeat", 50, "number of repetitions")
	flag.IntVar(&valueSize, "value-size", 100, "size of single entry value in bytes")
	flag.Parse()

	debug.SetGCPercent(10)
	fmt.Println("Cache:             ", c)
	fmt.Println("Number of entries: ", entries)
	fmt.Println("Number of repeats: ", repeat)
	fmt.Println("Value size:        ", valueSize)

	var benchFunc func(entries, valueSize int)

	switch c {
	case "gigacache":
		benchFunc = gigaCache
	case "stdmap":
		benchFunc = stdMap
	default:
		fmt.Printf("unknown cache: %s", c)
		os.Exit(1)
	}

	benchFunc(entries, valueSize)
	fmt.Println("GC pause for startup: ", gcPause())
	for i := 0; i < repeat; i++ {
		benchFunc(entries, valueSize)
	}

	fmt.Printf("GC pause for %s: %s\n", c, gcPause())
}

func stdMap(entries, valueSize int) {
	mapCache := make(map[string][]byte)
	for i := 0; i < entries; i++ {
		key, val := generateKeyValue(i, valueSize)
		mapCache[key] = val
	}
}

func gigaCache(entries, valueSize int) {
	c := cache.NewExtGigaCache[string](256)
	for i := 0; i < entries; i++ {
		key, val := generateKeyValue(i, valueSize)
		c.Set(key, val)
	}
}

func generateKeyValue(index int, valSize int) (string, []byte) {
	key := fmt.Sprintf("key-%010d", index)
	fixedNumber := []byte(fmt.Sprintf("%010d", index))
	val := append(make([]byte, valSize-10), fixedNumber...)

	return key, val
}
