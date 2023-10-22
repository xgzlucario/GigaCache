# GigaCache

[![Go Report Card](https://goreportcard.com/badge/github.com/xgzlucario/GigaCache)](https://goreportcard.com/report/github.com/xgzlucario/GigaCache) [![Go Reference](https://pkg.go.dev/badge/github.com/xgzlucario/GigaCache.svg)](https://pkg.go.dev/github.com/xgzlucario/GigaCache) ![](https://img.shields.io/badge/go-1.21.0-orange.svg) ![](https://img.shields.io/github/languages/code-size/xgzlucario/GigaCache.svg) [![codecov](https://codecov.io/gh/xgzlucario/GigaCache/graph/badge.svg?token=yC1xELYaM2)](https://codecov.io/gh/xgzlucario/GigaCache) [![Test and coverage](https://github.com/xgzlucario/GigaCache/actions/workflows/rotom.yml/badge.svg)](https://github.com/xgzlucario/GigaCache/actions/workflows/rotom.yml)

Powerful, fast, eviction supported cache for managing Gigabytes of data, multi-threads support, Set() method is 2x fast as `built-in map`.

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
    m := cache.New[string]()

    m.Set("foo", []byte("bar"))
    m.SetEx("foo1", []byte("bar1"), time.Minute) // With expired time
    m.SetTx("foo2", []byte("bar2"), time.Now().Add(time.Minute).UnixNano()) // With deadline

    val,ok := m.Get("foo")
    fmt.Println(string(val), ok) // bar, true

    val, ts, ok := m.GetTx("foo1")
    fmt.Println(string(val), ts, ok) // bar1, (nanosecs), true

    ok := m.Delete("foo1")
    if ok { // ... }

    ok = m.Rename("foo", "newFoo")
    if ok { // ... }

    // or Range cache
    m.Scan()
    m.Keys()
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
| Set/stdmap-20    | 3864405 | 335.9 ns/op | 210 B/op | 1 allocs/op |
| Set/GigaCache-20 | 7264236 | 227.4 ns/op | 173 B/op | 2 allocs/op |
| Set/swissmap-20  | 6519584 | 212.5 ns/op | 113 B/op | 0 allocs/op |

**Get** from 100k entries.

| Benchmark        | Iter     | time/op     | bytes/op | alloc/op    |
| ---------------- | -------- | ----------- | -------- | ----------- |
| Get/stdmap-20    | 47223234 | 26.28 ns/op | 7 B/op   | 0 allocs/op |
| Get/GigaCache-20 | 26522108 | 42.99 ns/op | 8 B/op   | 0 allocs/op |
| Get/swissmap-20  | 46438924 | 26.10 ns/op | 7 B/op   | 0 allocs/op |

**Delete**

| Benchmark              | Iter     | time/op     | bytes/op | alloc/op    |
| ---------------------- | -------- | ----------- | -------- | ----------- |
| Delete/stdmap-20       | 87499602 | 14.53 ns/op |	7 B/op	 | 0 allocs/op |
| Delete/GigaCache-20 | 22143832 | 49.78 ns/op |	8 B/op	 | 1 allocs/op |
| Delete/swissmap-20 | 50007508	| 24.14 ns/op |	7 B/op	| 0 allocs/op |

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

# 🛸Eliminate Bench

compressThreshold with 200s test.

| compressThreshold | count(w) | bytes(w) | ccount | rate(%) | avg(ns) |
| ----------------- | -------- | -------- | ------ | ------- | ------- |
| 0.5               | 40882    | 6533     | 103831 | 68.2    | 0.07    |
| 0.6               | 40071    | 5366     | 166297 | 77.8    | 0.07    |
| 0.7               | 39754    | 4880     | 227265 | 84.4    | 0.08    |
| 0.8               | 40567    | 4359     | 289811 | 91.9    | 0.09    |
| 0.9               | 29276    | 2868     | 507613 | 96.1    | 0.10    |

![p1](p1.png)
