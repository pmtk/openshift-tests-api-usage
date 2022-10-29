package main

func getValues[K comparable, V comparable](m map[K]V) []V {
	s := make([]V, 0, len(m))
	for _, v := range m {
		s = append(s, v)
	}
	return s
}

func filter[T comparable](xs []T, pred func(T) bool) []T {
	xs2 := []T{}
	for _, x := range xs {
		if pred(x) {
			xs2 = append(xs2, x)
		}
	}
	return xs2
}
func getIndex[T comparable](xs []T, pred func(T) bool) int {
	for idx, x := range xs {
		if pred(x) {
			return idx
		}
	}
	return -1
}
