package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/slices"
)

func TestReuseArray(t *testing.T) {
	assert := assert.New(t)
	arr := newReuseSlice(3)

	// push
	arr.push(0, 999)
	assert.Equal(arr.key, []int{0, 0, 0})
	assert.Equal(arr.val, []int{0, 0, 0})

	arr.push(2, 5)
	assert.Equal(arr.key, []int{2, 0, 0})
	assert.Equal(arr.val, []int{5, 0, 0})

	arr.push(3, 6)
	assert.Equal(arr.key, []int{2, 3, 0})
	assert.Equal(arr.val, []int{5, 6, 0})

	arr.push(1, 7)
	assert.Equal(arr.key, []int{2, 3, 1})
	assert.Equal(arr.val, []int{5, 6, 7})

	arr.push(4, 8)
	assert.Equal(arr.key, []int{2, 3, 4})
	assert.Equal(arr.val, []int{5, 6, 8})

	arr.push(5, 9)
	assert.Equal(arr.key, []int{5, 3, 4})
	assert.Equal(arr.val, []int{9, 6, 8})

	// pop
	val, ok := arr.pop(0)
	assert.Equal(val, -1)
	assert.False(ok)

	val, ok = arr.pop(1)
	assert.Equal(arr.key, []int{5, 0, 4})
	assert.Equal(arr.val, []int{9, 6, 8})
	assert.Equal(val, 6)
	assert.True(ok)

	val, ok = arr.pop(999)
	assert.Equal(arr.key, []int{5, 0, 4})
	assert.Equal(arr.val, []int{9, 6, 8})
	assert.Equal(val, -1)
	assert.False(ok)
}

func TestArray(t *testing.T) {
	assert := assert.New(t)
	arr := newReuseSlice(8)

	for i := 0; i < 10000; i++ {
		arr.push(i, i)
		assert.Equal(slices.Min(arr.key), arr.key[i%8])
		assert.True(slices.Equal(arr.key, arr.val))
	}
}
