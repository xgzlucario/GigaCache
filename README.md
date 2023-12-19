# GigaCache

[![Go Report Card](https://goreportcard.com/badge/github.com/xgzlucario/GigaCache)](https://goreportcard.com/report/github.com/xgzlucario/GigaCache) [![Go Reference](https://pkg.go.dev/badge/github.com/xgzlucario/GigaCache.svg)](https://pkg.go.dev/github.com/xgzlucario/GigaCache) ![](https://img.shields.io/badge/go-1.21.0-orange.svg) ![](https://img.shields.io/github/languages/code-size/xgzlucario/GigaCache.svg) [![codecov](https://codecov.io/gh/xgzlucario/GigaCache/graph/badge.svg?token=yC1xELYaM2)](https://codecov.io/gh/xgzlucario/GigaCache) [![Test and coverage](https://github.com/xgzlucario/GigaCache/actions/workflows/rotom.yml/badge.svg)](https://github.com/xgzlucario/GigaCache/actions/workflows/rotom.yml)

GigaCache is a Golang cache built on `swissmap`, designed to manage GB-level caches with better performance, and higher memory efficiency than `built-in map`, multi-threaded support, 0 GC overhead.

[See doc here](https://www.yuque.com/1ucario/devdoc/ntyyeekkxu8apngd?singleDoc)

# 🚗Usage

**Install**

```bash
go get github.com/xgzlucario/GigaCache
```

**Example**

```go
package main

import (
    "fmt"
    cache "github.com/xgzlucario/GigaCache"
)

func main() {
    m := cache.New()

    m.Set("foo", []byte("bar"))
    // Set with expired time.
    m.SetEx("foo1", []byte("bar1"), time.Minute)
     // Set with deadline.
    m.SetTx("foo2", []byte("bar2"), time.Now().Add(time.Minute).UnixNano())

    val, ts, ok := m.Get("foo")
    fmt.Println(string(val), ok) // bar, (nanosecs), true

    ok := m.Has("foo1") // true
    if ok { 
        // ...
    }

    ok := m.Delete("foo1") // true
    if ok { 
        // ...
    }

    // or Range cache
    m.Scan(func(key []byte, val []byte, ts int64) bool {
        // ...
        return true
    })

    m.Keys() // ["foo", "foo2"]
}
```

# 🚀Benchmark

**Environment**

```
goos: linux
goarch: amd64
pkg: github.com/xgzlucario/GigaCache
cpu: 13th Gen Intel(R) Core(TM) i5-13600KF
```

**Set**

Gigache Set operation has better performance than stdmap.

| Benchmark        | Iter    | time/op     | bytes/op | alloc/op    |
| ---------------- | ------- | ----------- | -------- | ----------- |
| Set/stdmap-20    | 4457132 | 296.9 ns/op | 183 B/op | 1 allocs/op |
| Set/GigaCache-20 | 6852141 | 216.9 ns/op | 146 B/op | 1 allocs/op |
| Set/swissmap-20  | 6700950 | 215.5 ns/op | 110 B/op | 0 allocs/op |

**Get** from 100k entries.

| Benchmark        | Iter     | time/op     | bytes/op | alloc/op    |
| ---------------- | -------- | ----------- | -------- | ----------- |
| Get/stdmap-20    | 22750813 | 52.25 ns/op | 7 B/op   | 0 allocs/op |
| Get/GigaCache-20 | 20830256 | 52.62 ns/op | 8 B/op   | 1 allocs/op |
| Get/swissmap-20  | 33340096 | 34.66 ns/op | 7 B/op   | 0 allocs/op |

**Delete**

| Benchmark              | Iter     | time/op     | bytes/op | alloc/op    |
| ---------------------- | -------- | ----------- | -------- | ----------- |
| Delete/stdmap-20       | 87499602 | 14.53 ns/op |	7 B/op	 | 0 allocs/op |
| Delete/GigaCache-20    | 22143832 | 49.78 ns/op |	8 B/op	 | 1 allocs/op |
| Delete/swissmap-20     | 50007508	| 24.14 ns/op |	7 B/op	 | 0 allocs/op |

**Iter** from 100k entries.

| Benchmark                   | Iter     | time/op       | bytes/op | alloc/op    |
| --------------------------- | -------- | ------------- | -------- | ----------- |
| BenchmarkIter/stdmap-20     |      496 | 2451833 ns/op |	 0 B/op	| 0 allocs/op |
| BenchmarkIter/GigaCache-20  |     1998 |  579076 ns/op |	 0 B/op | 0 allocs/op |
| BenchmarkIter/swissmap-20   | 	5544 |  201880 ns/op |	 0 B/op | 0 allocs/op |

**Latency**

Insert 49010*10000 pieces of data in 200s, p90 is 0.36us, p99 is 0.69us, p9999 is 51.12us.

```sh
[Cache] 200s | 49010w | len: 3200w | alloc: 577.3MB / 772.9MB (76.0%)
[Evict] probe: 3549w / 20431w (17.4%) | mtime: 9824
[Mem] mem: 5859MB | sys: 8348MB | gc: 38 | gcpause: 86 us
50th = 0.22 us
75th = 0.27 us
90th = 0.36 us
99th = 0.69 us
9999th = 51.12 us
```

**GC pause time**（Reference to [allegro/bigcache-bench](https://github.com/allegro/bigcache-bench)）

```go
Cache:              stdmap
Number of entries:  20000000
Number of repeats:  50
Value size:         100
Heap Objects Total: 446
GC pause for startup:  2.948819ms
```

```go
Cache:              bigcache
Number of entries:  20000000
Number of repeats:  50
Value size:         100
Heap Objects Total: 419
GC pause for startup:  1.129539ms
```

```go
Cache:              gigacache
Number of entries:  20000000
Number of repeats:  50
Value size:         100
Heap Objects Total: 471
GC pause for startup:  10.828795ms
```

# 🛸Internal

GigaCache structure.

![p1](p1.png)

Key & Idx Defination.

![p2](p2.png)

Bucket structure.

![p3](p3.png)
