package main

import (
	"fmt"
	"hash/fnv"
	"time"

	"github.com/zeebo/xxh3"
)

const (
	SHARD_COUNT = 32
	N           = 1000 * 10000
)

type hashFunc func(string) uint64

type cache struct {
	buckets []map[uint32]struct{}
}

func newCache() *cache {
	buckets := make([]map[uint32]struct{}, SHARD_COUNT)
	for i := range buckets {
		buckets[i] = make(map[uint32]struct{}, N/SHARD_COUNT)
	}
	return &cache{buckets: buckets}
}

func (c *cache) Set(key string, shardFunc, setFunc hashFunc) {
	shard := shardFunc(key) % uint64(len(c.buckets))
	h := setFunc(key)
	c.buckets[shard][uint32(h)] = struct{}{}
}

func (c *cache) Len() int {
	n := 0
	for _, b := range c.buckets {
		n += len(b)
	}
	return n
}

func main() {
	dataset := make([]string, 0, N)
	for i := 0; i < N; i++ {
		dataset = append(dataset, fmt.Sprintf("%010x", i))
	}

	// xxh3
	m := newCache()
	start := time.Now()
	for _, k := range dataset {
		m.Set(k, xxhash, xxhash)
	}
	fmt.Println("xxh3(xxh3):", N-m.Len(), time.Since(start))

	// xxh3 with shard fnv64
	m = newCache()
	start = time.Now()
	for _, k := range dataset {
		m.Set(k, fnv64, xxhash)
	}
	fmt.Println("xxh3(fnv64):", N-m.Len(), time.Since(start))
}

func xxhash(text string) uint64 {
	return xxh3.HashString(text)
}

func fnv64(text string) uint64 {
	algorithm := fnv.New64()
	algorithm.Write([]byte(text))
	return algorithm.Sum64()
}
