package main

import (
	"bytes"
	"fmt"
	cache "github.com/xgzlucario/GigaCache"
	"strconv"
)

func main() {
	opt := cache.DefaultOptions
	opt.DisableEvict = true
	opt.ShardCount = 1
	opt.OnHashConflict = func(key, val []byte) {
		panic(fmt.Sprintf("hash conflict: %s %s", key, val))
	}
	m := cache.New(opt)

	for i := 0; ; i++ {
		if i%30000 == 0 {
			fmt.Println("progress:", i/10000, "w")
		}
		num := cache.FastRand64()
		k := strconv.FormatUint(num, 36)
		v := []byte(strconv.FormatUint(num>>48, 36))

		m.Set(k, v)

		val, ts, ok := m.Get(k)
		if !bytes.Equal(val, v) {
			panic("val is not equal")
		}
		if ts != 0 || !ok {
			panic("error")
		}
	}
}
