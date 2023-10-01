package main

import (
	"fmt"
	"strconv"
	"time"

	"golang.org/x/exp/rand"

	"net/http"
	_ "net/http/pprof"

	cache "github.com/xgzlucario/GigaCache"
)

func main() {
	go http.ListenAndServe("localhost:6060", nil)

	start := time.Now()

	pset := cache.NewPercentile()

	var count int64
	var avgRate, avgBytes, avgTime float64

	bc := cache.New[string]()

	// Stat
	go func() {
		for i := 0; ; i++ {
			time.Sleep(time.Second / 10)

			// benchmark test
			if i > 0 && i%20 == 0 {
				stat := bc.Stat()

				avgRate += stat.ExpRate()
				avgBytes += float64(stat.LenBytes)
				avgTime++

				// Stats
				fmt.Printf("New Cache [%.0fs] [%dw] | len: %dw | alloc: %dw | bytes: %.0fw | rate: %.1f%% | mtime: %d\n",
					time.Since(start).Seconds(),
					count/1e4,
					stat.Len/1e4,
					stat.AllocTimes/1e4,
					avgBytes/avgTime/1e4,
					avgRate/avgTime,
					stat.MigrateTimes)

				// latency
				fmt.Println("latency(micros)")
				pset.Print()

				fmt.Println()
			}
		}
	}()

	// 8 clients set concurrent
	for i := 0; i < 8; i++ {
		go func() {
			for {
				k := strconv.Itoa(int(rand.Uint32()))
				now := time.Now()

				bc.SetEx(k, []byte(k), time.Second)
				count++

				pset.Add(float64(time.Since(now)) / float64(time.Microsecond))
			}
		}()
	}

	select {}
}
