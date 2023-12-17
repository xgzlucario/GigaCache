package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

/*
[Cache] 200s | 46128w | len: 297w | alloc: 55.3MB / 71.8MB (78.0%)
[Evict] probe: 4103w / 20429w (20.1%) | mtime: 58470
[Mem] mem: 444MB | sys: 962MB | gc: 119 | gcpause: 72 us
90th = 0.39 us
99th = 0.68 us
100th = 57.28 us

[Cache] 200s | 44893w | len: 222w | alloc: 52.0MB / 67.7MB (77.6%)
[Evict] probe: 4107w / 20035w (20.5%) | mtime: 46060
[Mem] mem: 660MB | sys: 915MB | gc: 100 | gcpause: 69 us
90th = 0.43 us
99th = 0.72 us
100th = 48.63 us
*/

func TestReuseArray(t *testing.T) {
	assert := assert.New(t)
	arr := newReuseSlice(4)

	// push
	arr.push(0, 999)
	assert.Equal(arr.key, []int{0, 0, 0, 0})
	assert.Equal(arr.val, []int{0, 0, 0, 0})

	arr.push(2, 5)
	assert.Equal(arr.key, []int{2, 0, 0, 0})
	assert.Equal(arr.val, []int{5, 0, 0, 0})

	arr.push(3, 6)
	assert.Equal(arr.key, []int{2, 3, 0, 0})
	assert.Equal(arr.val, []int{5, 6, 0, 0})

	arr.push(1, 7)
	assert.Equal(arr.key, []int{2, 3, 1, 0})
	assert.Equal(arr.val, []int{5, 6, 7, 0})

	arr.push(4, 8)
	assert.Equal(arr.key, []int{2, 3, 1, 4})
	assert.Equal(arr.val, []int{5, 6, 7, 8})

	arr.push(5, 9)
	assert.Equal(arr.key, []int{2, 3, 5, 4})
	assert.Equal(arr.val, []int{5, 6, 9, 8})

	// pop
	val, ok := arr.pop(0)
	assert.Equal(val, -1)
	assert.False(ok)

	val, ok = arr.pop(1)
	assert.Equal(arr.key, []int{0, 3, 5, 4})
	assert.Equal(arr.val, []int{5, 6, 9, 8})
	assert.Equal(val, 5)
	assert.True(ok)

	val, ok = arr.pop(999)
	assert.Equal(arr.key, []int{0, 3, 5, 4})
	assert.Equal(arr.val, []int{5, 6, 9, 8})
	assert.Equal(val, -1)
	assert.False(ok)
}
