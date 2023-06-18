package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/allegro/bigcache/v3"
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
	case "bigcache":
		benchFunc = bigCache
	case "gigacache":
		benchFunc = gigaCache
	case "stdmap":
		benchFunc = stdMap
	case "hashmap":
		benchFunc = hashMap
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

func hashMap(entries, valueSize int) {
	mapCache := cache.NewMap[string, []byte]()
	for i := 0; i < entries; i++ {
		key, val := generateKeyValue(i, valueSize)
		mapCache.Set(key, val)
	}
}

func bigCache(entries, valueSize int) {
	config := bigcache.Config{
		Shards:             256,
		LifeWindow:         100 * time.Minute,
		MaxEntriesInWindow: entries,
		MaxEntrySize:       200,
		Verbose:            true,
	}

	bigcache, _ := bigcache.New(context.Background(), config)
	for i := 0; i < entries; i++ {
		key, val := generateKeyValue(i, valueSize)
		bigcache.Set(key, val)
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
