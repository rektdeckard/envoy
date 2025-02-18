package main

func prepend[T any](s []T, v T) []T {
	return append([]T{v}, s...)
}
