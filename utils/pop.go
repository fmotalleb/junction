package utils

func PopInPlace[T comparable](arr *[]T, key T) bool {
	s := *arr
	for i, v := range s {
		if v == key {
			copy(s[i:], s[i+1:])
			*arr = s[:len(s)-1]
			return true
		}
	}
	return false
}
