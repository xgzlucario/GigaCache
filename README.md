# GigaCache

[![Go Report Card](https://goreportcard.com/badge/github.com/xgzlucario/GigaCache)](https://goreportcard.com/report/github.com/xgzlucario/GigaCache) [![Go Reference](https://pkg.go.dev/badge/github.com/xgzlucario/GigaCache.svg)](https://pkg.go.dev/github.com/xgzlucario/GigaCache) ![](https://img.shields.io/badge/go-1.21.0-orange.svg) ![](https://img.shields.io/github/languages/code-size/xgzlucario/GigaCache.svg) [![codecov](https://codecov.io/gh/xgzlucario/GigaCache/graph/badge.svg?token=yC1xELYaM2)](https://codecov.io/gh/xgzlucario/GigaCache) [![Test and coverage](https://github.com/xgzlucario/GigaCache/actions/workflows/rotom.yml/badge.svg)](https://github.com/xgzlucario/GigaCache/actions/workflows/rotom.yml)

GigaCache 是一个基于 `swissmap` 的高性能 Go 缓存库，为 GB 级序列化数据而设计，支持设置过期时间与淘汰机制，相比 `stdmap` 有更快的速度，更高的内存效率，和更小的延迟。

特性：

1. 只支持序列化的数据，性能超强，插入性能相比 `stdmap` 提升了 **93%**，内存使用减少 **50%**。
2. 采用分片技术减小锁粒度，并分块管理数据
3. 键值对独立的过期时间支持，使用定期淘汰策略驱逐过期的键值对
4. 内置迁移算法，定期整理碎片空间，以释放内存
5. 类似于 `bigcache` 规避 GC 的设计，上亿数据集的 P99 延迟在微秒级别

你可以阅读 [博客文档](https://lucario.cn/posts/gigacache/) 了解更多的技术细节。

# 性能

下面是插入 2000 万条数据的性能对比测试，`GigaCache` 的插入速度相比 `stdmap` 提升了 **93%**，内存使用相比也减少了 **50%** 左右。

```
gigacache
entries: 20000000
alloc: 1327 mb
gcsys: 7 mb
heap inuse: 1327 mb
heap object: 5033 k
gc: 12
pause: 2.348011ms
cost: 10.903936565s
```

```
stdmap
entries: 20000000
alloc: 2702 mb
gcsys: 16 mb
heap inuse: 2709 mb
heap object: 29596 k
gc: 11
pause: 2.564445ms
cost: 21.102264031s
```

**测试环境**

```
goos: linux
goarch: amd64
pkg: github.com/xgzlucario/GigaCache
cpu: AMD Ryzen 7 5800H with Radeon Graphics
```

# 使用

首先安装 GigaCache 到本地：

```bash
go get github.com/xgzlucario/GigaCache
```

运行下面的代码示例：

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

# 内部架构

GigaCache

![p1](p1.png)

Key & Idx

![p2](p2.png)

Bucket

![p3](p3.png)
