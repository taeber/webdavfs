package dos33

import (
	"context"
	"io/fs"
	"os"
	"slices"
	"testing"
	"time"
)

func TestListRoot(t *testing.T) {
	fs := newFileSystem("DISK.DSK")

	file, err := fs.OpenFile(context.Background(), "/", 0, os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}

	files, err := file.Readdir(0)
	if err != nil {
		t.Fatal(err)
	}

	actual := transform(files, name)
	slices.Sort(actual)
	expected := []string{"DISK", "README.txt"}
	if !slices.Equal(expected, actual) {
		t.Fatal(expected, "!=", actual)
	}
}

func TestListDisk(t *testing.T) {
	fs := newFileSystem("DISK.DSK")

	file, err := fs.OpenFile(context.Background(), "/DISK", 0, os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}

	files, err := file.Readdir(0)
	if err != nil {
		t.Fatal(err)
	}

	actual := transform(files, name)
	slices.Sort(actual)
	expected := []string{"HELLO", "PROG", "_dos"}
	if !slices.Equal(expected, actual) {
		t.Fatal(expected, "!=", actual)
	}
}

func TestBadDiskName_ThrowsMissing(t *testing.T) {
	fs := newFileSystem()

	_, err := fs.OpenFile(context.Background(), "/missing", 0, os.ModePerm)
	if err != nil && err.Error() != "file does not exist" {
		t.Fatal("Expected missing file error")
	}
}

func TestDiskHasModTime(t *testing.T) {
	fs := newFileSystem("DISK.DSK")

	info, err := fs.Stat(context.Background(), "/DISK")
	if err != nil {
		t.Fatal(err)
	}

	t.Log("modtime =", info.ModTime())
	if info.ModTime() == (time.Time{}) {
		t.Fatal("Expected DSK to have ModTime set")
	}
}

func TestReadmeIsNotEmpty(t *testing.T) {
	fs := newFileSystem()

	file, err := fs.OpenFile(context.Background(), "/README.txt", 0, os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}

	var buf [len("DOS 3.3 DSK Filesystem Folder Structure")]byte

	n, err := file.Read(buf[0:])
	if err != nil {
		t.Fatal(err)
	}
	if n <= 0 {
		t.Fatal("Failed to read from README.txt")
	}
}

func name(info fs.FileInfo) string { return info.Name() }

// transform maps items from type T to result type R using fn.
func transform[T, R any](items []T, fn func(T) R) []R {
	mapped := make([]R, 0, len(items))
	for _, item := range items {
		mapped = append(mapped, fn(item))
	}
	return mapped
}
