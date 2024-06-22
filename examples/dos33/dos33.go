package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/net/webdav"
)

/// dos33FS

type dos33FS struct {
	disks []*diskette
}

var (
	// Directories under each DSK folder.
	dskDirs = words("files applesoft binary intbasic text a b r s locks")
	// Files under each DSK folder.
	dskFiles = words("CATALOG VTOC")
)

func (*dos33FS) Mkdir(ctx context.Context, name string, perm fs.FileMode) error {
	return errors.ErrUnsupported
}

func (fs *dos33FS) OpenFile(ctx context.Context, name string, flag int, perm fs.FileMode) (webdav.File, error) {
	parts := pathparts(name)
	switch len(parts) {
	case 0:
		return rootDirectory(fs), nil

	case 1:
		if parts[0] == "README" {
			return newMemFile("README", readme), nil
		}

		dsk := fs.find(parts[0])
		if dsk == nil {
			break
		}
		return diskDirectory(dsk), nil

	case 2:
		dsk := fs.find(parts[0])
		if dsk == nil {
			break
		}
		if slices.Contains(dskDirs, parts[1]) {
			return &directory{fileInfo: *dirInfo(parts[1])}, nil
		}
		if slices.Contains(dskFiles, parts[1]) {
			return newMemFile(parts[1], "TODO: implement"), nil
		}
	}

	return nil, http.ErrMissingFile
}

func (fs *dos33FS) RemoveAll(ctx context.Context, name string) error {
	return errors.ErrUnsupported
}

func (fs *dos33FS) Rename(ctx context.Context, oldName, newName string) error {
	return errors.ErrUnsupported
}

func (fs *dos33FS) Stat(ctx context.Context, name string) (fs.FileInfo, error) {
	parts := pathparts(name)
	switch len(parts) {
	case 0:
		return dirInfo("/"), nil

	case 1:
		if parts[0] == "README" {
			return file(parts[0]), nil
		}

		dsk := fs.find(parts[0])
		if dsk == nil {
			break
		}
		dfi := dirInfo(dsk.name)
		if fi, err := dsk.file.Stat(); err == nil {
			dfi.modTime = fi.ModTime()
		}
		return dfi, nil

	case 2:
		dsk := fs.find(parts[0])
		if dsk == nil {
			break
		}
		if slices.Contains(dskDirs, parts[1]) {
			return dirInfo(parts[1]), nil
		}
		if slices.Contains(dskFiles, parts[1]) {
			return file(parts[1]), nil
		}
	}

	return nil, http.ErrMissingFile
}

func (fs *dos33FS) find(name string) *diskette {
	for _, dsk := range fs.disks {
		if dsk.name == name {
			return dsk
		}
	}
	return nil
}

// NewFileSystem returns a new DOS 3.3 DSK Filesystem.
func NewFileSystem(disks ...string) webdav.FileSystem {
	fs := dos33FS{}
	for _, name := range disks {
		dsk, err := loadDiskette(name)
		if err != nil {
			log.Fatalln("Could not load diskette:", name, err)
			continue
		}
		fs.disks = append(fs.disks, &dsk)
	}
	return &fs
}

func rootDirectory(filesys *dos33FS) *directory {
	return &directory{
		fileInfo: *dirInfo("/"),
		children: slices.Concat(
			dirs(transform(filesys.disks, diskName)...),
			files("README"),
		),
	}
}

func diskDirectory(dsk *diskette) *directory {
	return &directory{
		fileInfo: *dirInfo(dsk.name),
		children: slices.Concat(
			dirs(dskDirs...),
			files(dskFiles...),
		),
	}
}

/// directory

type directory struct {
	fileInfo
	children []fs.FileInfo
}

func (f *directory) Readdir(count int) ([]fs.FileInfo, error) {
	if count > 0 {
		return f.children[0:count], nil
	}
	return f.children, nil
}
func (f *directory) Stat() (fs.FileInfo, error)                   { return f, nil }
func (f *directory) Close() error                                 { return nil }
func (f *directory) Write(p []byte) (int, error)                  { return 0, errors.ErrUnsupported }
func (f *directory) Read(p []byte) (int, error)                   { return 0, errors.ErrUnsupported }
func (f *directory) Seek(offset int64, whence int) (int64, error) { return 0, errors.ErrUnsupported }

/// fileInfo

type fileInfo struct {
	name    string
	mode    fs.FileMode
	modTime time.Time
	isDir   bool
	sys     any
	size    int64
}

func (info *fileInfo) Name() string       { return info.name }
func (info *fileInfo) Mode() fs.FileMode  { return info.mode }
func (info *fileInfo) IsDir() bool        { return info.isDir }
func (info *fileInfo) ModTime() time.Time { return info.modTime }
func (info *fileInfo) Sys() any           { return info.sys }
func (info *fileInfo) Size() int64        { return info.size }

func file(name string) fs.FileInfo        { return &fileInfo{name: name, mode: 0444} }
func files(names ...string) []fs.FileInfo { return transform(names, file) }
func dirs(names ...string) []fs.FileInfo  { return transform(names, dir) }
func dir(name string) fs.FileInfo         { return dirInfo(name) }
func dirInfo(name string) *fileInfo {
	return &fileInfo{
		name:  name,
		isDir: true,
		mode:  fs.ModeDir | fs.ModePerm,
	}
}

/// diskette

type diskette struct {
	path     string // Path on host
	name     string
	file     *os.File
	readonly bool
}

func loadDiskette(path string) (dsk diskette, err error) {
	dsk.path = path
	dsk.name = filepath.Base(path)
	dsk.file, err = os.OpenFile(path, os.O_RDWR, os.FileMode(0))
	if err == nil {
		return
	}
	if errors.Is(err, os.ErrPermission) {
		dsk.file, err = os.OpenFile(path, os.O_RDONLY, os.FileMode(0))
		dsk.readonly = true
	}
	return
}

func diskName(dsk *diskette) string { return dsk.name }

/// memFile

type memFile struct {
	fileInfo
	content *bytes.Reader
}

func (file *memFile) Size() int64                              { return file.content.Size() }
func (file *memFile) Close() error                             { return nil }
func (file *memFile) Read(p []byte) (n int, err error)         { return file.content.Read(p) }
func (file *memFile) Readdir(count int) ([]fs.FileInfo, error) { return nil, errors.ErrUnsupported }
func (file *memFile) Stat() (fs.FileInfo, error)               { return file, nil }
func (file *memFile) Write(p []byte) (n int, err error)        { return 0, errors.ErrUnsupported }
func (file *memFile) Seek(offset int64, whence int) (int64, error) {
	return file.content.Seek(offset, whence)
}

func newMemFile(name, content string) *memFile {
	return &memFile{
		fileInfo: fileInfo{
			name: name,
			mode: 0444,
		},
		content: bytes.NewReader([]byte(content)),
	}
}

const readme = `DOS 3.3 DSK Filesystem Folder Structure

Each DSK is represented as a folder with the following files and folders.

  files/      Read-only versions of all files, as raw binary.
  CATALOG     a close approximation of running CATLOG from DOS.
  locks/      All locked files. Lock a file by adding it, unlock by deleting it.
	VTOC        Volume Table of Contents information that might be helpful.

You can edit and create files by type under these folders:
  applesoft/
  intbasic/
  binary/
  text/
  a/
  b/
  r/
  s/

For the following "text" folders, the appropriate conversion takes place:
  applesoft/
  intbasic/
  text/
`

func pathparts(name string) []string {
	if name == "" || name[0] != '/' {
		name = "/" + name
	}
	parts := strings.Split(name, "/")
	if parts[len(parts)-1] == "" {
		return parts[1 : len(parts)-1]
	} else {
		return parts[1:]
	}
}

// transform maps items from type T to result type R using fn.
func transform[T, R any](items []T, fn func(T) R) []R {
	mapped := make([]R, 0, len(items))
	for _, item := range items {
		mapped = append(mapped, fn(item))
	}
	return mapped
}

func words(s string) []string { return strings.Split(s, " ") }

/// main

func main() {
	addr := flag.String("addr", "127.0.0.1:33333", "HTTP address on which to listen")
	prefix := flag.String("prefix", "/dos33", "URL path prefix")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "dos33 is a WebDAV-based filesystem for Apple DOS 3.3 DSKs.")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "usage: dos33 [-addr ADDR] [-prefix PREFIX] DSK...")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "DSK is one or more files for the WebDAV server to expose.")
		fmt.Fprintln(os.Stderr)
		for _, name := range []string{"addr", "prefix"} {
			f := flag.Lookup(name)
			fmt.Fprintf(os.Stderr, "-%s %s\n", f.Name, strings.ToUpper(f.Name))
			fmt.Fprintf(os.Stderr, "  %s (default \"%s\")\n", f.Usage, f.DefValue)
		}
	}
	flag.Parse()

	if flag.NArg() < 1 {
		log.Println("No DSK files provided.")
		flag.Usage()
		os.Exit(2)
	}

	disks := flag.Args()

	loc := fmt.Sprintf("http://%s%s", *addr, *prefix)
	uri, err := url.Parse(loc)
	if err != nil {
		log.Fatalln(err)
	}

	handler := webdav.Handler{
		Prefix:     *prefix,
		LockSystem: webdav.NewMemLS(),
		FileSystem: NewFileSystem(disks...),
		Logger:     func(r *http.Request, e error) { log.Println(r.Method, r.URL.Path, e) },
	}

	log.Println("Serving DOS3.3 DSK filesystem over WebDAV")
	log.Println(" Address:", uri)
	http.ListenAndServe(*addr, &handler)
}
