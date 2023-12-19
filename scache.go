package cache

import "math"

// spaceCache is used to reuse bytes of space.
// It retains large spaces, discards small spaces, to improve memory efficiency.
type spaceCache struct {
	key []int
	val []int
}

// newSpaceCache return a cache with fixed length.
func newSpaceCache(size int) spaceCache {
	return spaceCache{
		key: make([]int, size),
		val: make([]int, size),
	}
}

// put passes in a fragment space, if the space is larger than the one in the cache, replace it.
func (s *spaceCache) put(key int, val int) {
	if key <= 0 {
		return
	}
	// find min and pos in slice.
	min, pos := min(s.key, -math.MaxInt)

	// if key > min, replace it.
	if key > min {
		s.key[pos] = key
		s.val[pos] = val
	}
}

// fetchGreat returns the smallest val that is greater than want.
func (s *spaceCache) fetchGreat(want int) (int, bool) {
	if want <= 0 {
		return -1, false
	}
	// find min and pos in slice.
	_, pos := min(s.key, want)

	if pos >= 0 {
		s.key[pos] = 0
		return s.val[pos], true
	}
	return -1, false
}

// min find minumum value greater than target in slice.
func min(s []int, target int) (min, pos int) {
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
