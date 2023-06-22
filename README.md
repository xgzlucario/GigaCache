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

```bash
goos: linux
goarch: amd64
pkg: github.com/xgzlucario/GigaCache
cpu: 13th Gen Intel(R) Core(TM) i5-13600KF
BenchmarkSet/stdmap-20         	 4133277	       282.7 ns/op	     153 B/op	       1 allocs/op
BenchmarkSet/gigacache-20      	 5590371	       245.2 ns/op	     124 B/op	       1 allocs/op
BenchmarkSet/bigcache-20       	 5770894	       254.2 ns/op	      44 B/op	       1 allocs/op
BenchmarkGet/stdmap-20         	 8593722	       149.9 ns/op	       7 B/op	       0 allocs/op
BenchmarkGet/gigacache-20      	 6024399	       188.6 ns/op	      23 B/op	       1 allocs/op
BenchmarkGet/bigcache-20       	 7502360	       157.2 ns/op	       7 B/op	       0 allocs/op
PASS
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