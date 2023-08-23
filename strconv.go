package cache

type Number interface {
	~int | ~int32 | ~int64 | ~uint | ~uint32 | ~uint64
}

const (
	VALID = 255
	RADIX = VALID - 1
)

func FormatNumber[T Number](n T) []byte {
	if n < 0 {
		panic("negative number")
	}
	if n == 0 {
		return []byte{0}
	}

	sb := make([]byte, 0, 1)
	for n > 0 {
		sb = append(sb, byte(n%RADIX))
		n /= RADIX
	}

	return sb
}

func ParseNumber[T Number](b []byte) T {
	var n T
	for i := len(b) - 1; i >= 0; i-- {
		n = n*RADIX + T(b[i])
	}
	return n
}
