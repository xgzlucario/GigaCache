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
    m := cache.New(cache.DefaultOptions)

    m.Set("foo", []byte("bar"))
    // Set with expired time.
    m.SetEx("foo1", []byte("bar1"), time.Minute)
     // Set with deadline.
    m.SetTx("foo2", []byte("bar2"), time.Now().Add(time.Minute).UnixNano())

    val, ts, ok := m.Get("foo")
    fmt.Println(string(val), ok) // bar, (nanosecs), true

    ok := m.Delete("foo1") // true
    if ok { 
        // ...
    }

    // or Range cache
    m.Scan(func(key, val []byte, ts int64) bool {
        // ...
        return false
    })
}
```

# 🚀Benchmark

**Environment**

```
goos: linux
goarch: amd64
pkg: github.com/xgzlucario/GigaCache
cpu: AMD Ryzen 7 5800H with Radeon Graphics
```

**Set**

Gigache Set operation has better performance than stdmap.

| Benchmark        | Iter    | time/op     | bytes/op | alloc/op    |
| ---------------- | ------- | ----------- | -------- | ----------- |
| Set/stdmap-16    | 2486958 | 516.9 ns/op | 143 B/op | 1 allocs/op |
| Set/GigaCache-16 | 2948646 | 465.9 ns/op |  97 B/op | 1 allocs/op |

**Get** from 1 million entries.

| Benchmark        | Iter     | time/op     | bytes/op | alloc/op    |
| ---------------- | -------- | ----------- | -------- | ----------- |
| Get/stdmap-16    | 5539920  | 242.2 ns/op |  7 B/op  | 0 allocs/op |
| Get/GigaCache-16 | 7041771  | 151.8 ns/op | 10 B/op  | 1 allocs/op |

**Scan** from 100k entries.

| Benchmark         | Iter   | time/op        | bytes/op   | alloc/op       |
| ----------------- | ------ | -------------- | ---------- | -------------- |
| Scan/stdmap-16    | 103    | 13304184 ns/op |     0 B/op |    0 allocs/op |
| Scan/GigaCache-16 | 224    |  5401054 ns/op | 25794 B/op | 1060 allocs/op |

**Delete**

| Benchmark              | Iter       | time/op      | bytes/op  | alloc/op    |
| ---------------------- | ---------- | ------------ | --------- | ----------- |
| Delete/stdmap-16       | 1000000000 | 0.2383 ns/op |   0 B/op  | 0 allocs/op |
| Delete/GigaCache-16    | 1000000000 | 0.7658 ns/op |   0 B/op  | 1 allocs/op |

# 🎢Integrated Bench

Run bench with `go run example/*.go`.

In the bench test below, GigaCache has better memory efficiency, and faster insertion performance than stdmap.

```go
gigacache
entries: 20000000
alloc: 1153 mb
gcsys: 30 mb
heap inuse: 1155 mb
heap object: 1515 k
gc: 15
pause: 362.249µs
cost: 5.436793342s
```

```go
stdmap
entries: 20000000
alloc: 2663 mb
gcsys: 64 mb
heap inuse: 2664 mb
heap object: 29482 k
gc: 11
pause: 385.449µs
cost: 8.033432768s
```

# 🛸Internal

GigaCache structure.

![p1](p1.png)

Key & Idx Defination.

![p2](p2.png)

Bucket structure.

![p3](p3.png)
