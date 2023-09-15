# GigaCache
Powerful, fast, expiration supported cache for managing Gigabytes of data.

# Usage

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
    m := cache.NewGigaCache[string]()
    
    m.Set("foo", []byte("bar")) // Set with key
    m.SetEx("foo1", []byte("bar1"), time.Minute) // Set key with expired duration
    m.SetTx("foo2", []byte("bar2"), time.Now().Add(time.Minute).UnixNano()) // Set key with expired deadline
    
    val,ok := m.Get("foo")
    fmt.Println(string(val), ok) // bar, true

    val, ts, ok := m.GetTx("foo1")
    fmt.Println(string(val), ts, ok) // bar1, 1687458634306210383(nanoseconds), true
}
```

# Benchmark

**Environment**

```
goos: linux
goarch: amd64
pkg: github.com/xgzlucario/GigaCache
cpu: 13th Gen Intel(R) Core(TM) i5-13600KF
```

**Set**

Gigache Set operation has better performance than stdmap.

| Benchmark           | Iter    | time/op     | bytes/op | alloc/op    |
| ------------------- | ------- | ----------- | -------- | ----------- |
| Set/stdmap-20       | 4059187 | 315.2 ns/op | 156 B/op | 1 allocs/op |
| Set/syncmap-20      | 2632218 | 385.3 ns/op | 126 B/op | 5 allocs/op |
| Set/gigacache-20    | 4897693 | 277.9 ns/op | 133 B/op | 1 allocs/op |
| Set/gigacache/Tx-20 | 4355415 | 328.2 ns/op | 161 B/op | 1 allocs/op |

**Get** from 100k entries.

| Benchmark           | Iter    | time/op     | bytes/op | alloc/op    |
| ------------------- | ------- | ----------- | -------- | ----------- |
| Get/stdmap-20       | 8906018 | 150.1 ns/op | 7 B/op   | 0 allocs/op |
| Get/syncmap-20      | 7723198 | 168.5 ns/op | 7 B/op   | 0 allocs/op |
| Get/gigacache-20    | 7293346 | 167.1 ns/op | 7 B/op   | 0 allocs/op |
| Get/gigacache/Tx-20 | 5621548 | 204.0 ns/op | 7 B/op   | 0 allocs/op |

**Delete**

| Benchmark              | Iter    | time/op     | bytes/op | alloc/op    |
| ---------------------- | ------- | ----------- | -------- | ----------- |
| Delete/stdmap-20       | 8321840 | 167.3 ns/op | 7 B/op   | 0 allocs/op |
| Delete/syncmap-20      | 7192041 | 176.8 ns/op | 7 B/op   | 0 allocs/op |
| Delete/gigacache-20    | 5177775 | 258.1 ns/op | 13 B/op  | 1 allocs/op |
| Delete/gigacache/Tx-20 | 3510531 | 306.2 ns/op | 7 B/op   | 0 allocs/op |

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

**Eliminate Bench**

compressThreshold with 200s test.

| compressThreshold | count  | bytes | ccount | rate  | avg    |
| ----------------- | ------ | ----- | ------ | ----- | ------ |
| 0.5               | 40234w | 6413w | 107531 | 69.7% | 0.12ns |
| 0.6               | 41032w | 5512w | 162890 | 78.5% | 0.11ns |
| 0.7               | 37757w | 4578w | 225590 | 83.6% | 0.16ns |
| 0.8               | 37623w | 4150w | 325649 | 90.9% | 0.17ns |
| 0.9               | 36276w | 3564w | 517681 | 96.2% | 0.17ns |