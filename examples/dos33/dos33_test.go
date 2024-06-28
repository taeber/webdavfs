package dos33

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
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
	expected := []string{"DISK.DSK", "README"}
	if !slices.Equal(expected, actual) {
		t.Fatal(expected, "!=", actual)
	}
}

func TestListDisk(t *testing.T) {
	fs := newFileSystem("DISK.DSK")

	file, err := fs.OpenFile(context.Background(), "/DISK.DSK", 0, os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}

	files, err := file.Readdir(0)
	if err != nil {
		t.Fatal(err)
	}

	actual := transform(files, name)
	expected := []string{
		"files", "applesoft", "binary", "intbasic", "text", "a", "b", "r", "s", "locks",
		"CATALOG", "VTOC",
	}
	if !slices.Equal(expected, actual) {
		t.Fatal(expected, "!=", actual)
	}
}

func TestBadDiskName_ThrowsMissing(t *testing.T) {
	fs := newFileSystem()

	_, err := fs.OpenFile(context.Background(), "/missing.dsk", 0, os.ModePerm)
	if !errors.Is(err, http.ErrMissingFile) {
		t.Fatal("Expected missing file error")
	}
}

func TestDiskHasModTime(t *testing.T) {
	fs := newFileSystem("DISK.DSK")

	info, err := fs.Stat(context.Background(), "/DISK.DSK")
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

	file, err := fs.OpenFile(context.Background(), "/README", 0, os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}

	var buf [len("DOS 3.3 DSK Filesystem Folder Structure")]byte

	n, err := file.Read(buf[0:])
	if err != nil {
		t.Fatal(err)
	}
	if n <= 0 {
		t.Fatal("Failed to read from README")
	}
}

func name(info fs.FileInfo) string { return info.Name() }
