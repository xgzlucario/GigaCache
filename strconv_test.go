package cache

import (
	"bytes"
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
