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

**Performance**

GigaCache is compared to stdmap、[jellydator/ttlcache](https://github.com/jellydator/ttlcache).

**Environment**

```
goos: linux
goarch: amd64
cpu: 13th Gen Intel(R) Core(TM) i5-13600KF
```

**Set**

```bash
BenchmarkSet/stdmap/Set-20         	 4527399	       268.2 ns/op	     140 B/op	       1 allocs/op
BenchmarkSet/gigacache/Set-20      	 6353096	       211.0 ns/op	     110 B/op	       1 allocs/op
BenchmarkSet/gigacache/SetTx-20    	 4177327	       352.9 ns/op	     183 B/op	       1 allocs/op
BenchmarkSet/ttlcache/Set-20       	 2748000	       491.9 ns/op	     187 B/op	       2 allocs/op
```

**Get** from 100k entries.

```
BenchmarkGet/stdmap-20         	10008024	       135.0 ns/op	       7 B/op	       0 allocs/op
BenchmarkGet/gigacache-20      	 6685338	       163.7 ns/op	       7 B/op	       0 allocs/op
BenchmarkGet/ttlcache-20       	 2045643	       510.5 ns/op	      55 B/op	       1 allocs/op
```

**Delete**

```
BenchmarkDelete/stdmap-20         	 8512951	       150.9 ns/op	       7 B/op	       0 allocs/op
BenchmarkDelete/gigacache-20      	32437833	        33.82 ns/op	       7 B/op	       0 allocs/op
BenchmarkDelete/ttlcache-20       	 2001484	       510.7 ns/op	      55 B/op	       1 allocs/op
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