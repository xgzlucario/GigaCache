package cache

import "math"

type reuseSlice struct {
	key []int
	val []int
}

func newReuseSlice(size int) *reuseSlice {
	return &reuseSlice{
		key: make([]int, size),
		val: make([]int, size),
	}
}

func (s *reuseSlice) push(key int, val int) {
	if key <= 0 {
		return
	}
	// find min key in slice.
	minKey, minKeyPos := getMinValue(s.key, -math.MaxInt)

	// if key > minKey, update it.
	if key > minKey {
		s.key[minKeyPos] = key
		s.val[minKeyPos] = val
	}
}

// pop returns the smallest val that is greater than key.
func (s *reuseSlice) pop(key int) (int, bool) {
	if key <= 0 {
		return -1, false
	}
	// find min key in slice.
	_, minKeyPos := getMinValue(s.key, key)

	if minKeyPos >= 0 {
		s.key[minKeyPos] = 0
		return s.val[minKeyPos], true
	}
	return -1, false
}

// getMinValue find a minumum value greater than target in slice.
func getMinValue(s []int, target int) (min, pos int) {
	min = math.MaxInt
	pos = -1

	for i, v := range s {
		if v < min && v >= target {
			min = v
			pos = i
		}
	}
	return
}
