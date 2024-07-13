package dos33

import (
	"slices"
	"strings"
)

// transform maps items from type T to result type R using fn.
func transform[T, R any](items []T, fn func(T) R) []R {
	mapped := make([]R, 0, len(items))
	for _, item := range items {
		mapped = append(mapped, fn(item))
	}
	return mapped
}

/// stringSlice

type stringSlice []string

func w(s string) stringSlice                 { return strings.Split(s, " ") }
func (w stringSlice) Contains(s string) bool { return slices.Contains(w, s) }
