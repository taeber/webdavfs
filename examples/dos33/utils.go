package dos33

import (
	"encoding/binary"
	"errors"
	"os"
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

// tryOpenFileRW tries to open a file for read-write, but falls back to
// read-only if it fails.
func tryOpenFileRW(path string) (file *os.File, err error, readonly bool) {
	file, err = os.OpenFile(path, os.O_RDWR, os.FileMode(0))
	if errors.Is(err, os.ErrPermission) {
		readonly = true
		file, err = os.OpenFile(path, os.O_RDONLY, os.FileMode(0))
	}
	return
}

// word interprets bytes as a little-endian, 16-bit, unsigned integer.
// This is the representation of the MOS 6502.
func word(bytes []byte) uint16 {
	return binary.LittleEndian.Uint16(bytes)
}

/// stringSlice

type stringSlice []string

func w(s string) stringSlice                 { return strings.Split(s, " ") }
func (w stringSlice) Contains(s string) bool { return slices.Contains(w, s) }
