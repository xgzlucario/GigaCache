package cache

import (
	"bytes"
	"strconv"
	"testing"
	"time"
)

func TestConv(t *testing.T) {
	num := time.Now().UnixNano()

	buf := FormatNumber(num)
	if n := ParseNumber[int64](buf); n != num {
		t.Fatalf("Number Convert error: %d %d", num, n)
	}

	// test zero
	if !bytes.Equal(FormatNumber(0), []byte{0}) {
		t.Fatalf("Number Convert error: %d", 0)
	}

	// test negative number
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("should panic")
		}
	}()
	FormatNumber(-1)
}

func BenchmarkConv(b *testing.B) {
	num := time.Now().UnixNano()

	b.Run("std/10", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			strconv.FormatInt(num, 10)
		}
	})
	b.Run("std/36", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			strconv.FormatInt(num, 36)
		}
	})
	b.Run("formatNumber", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			FormatNumber(num)
		}
	})
}
