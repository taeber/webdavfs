package dos33

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"golang.org/x/net/webdav"
)

// ListenAndServe starts a new WebDAV server at http://{addr}{prefix} with each
// of the disks exposing the DOS 3.3 DSK filesystem.
func ListenAndServe(addr, prefix string, disks ...string) error {
	loc := fmt.Sprintf("http://%s%s", addr, prefix)
	uri, err := url.Parse(loc)
	if err != nil {
		log.Fatalln(err)
	}

	dosfs := newFileSystem(disks...)

	handler := webdav.Handler{
		Prefix:     prefix,
		LockSystem: webdav.NewMemLS(),
		FileSystem: dosfs,
		Logger:     func(r *http.Request, e error) { log.Println(r.Method, r.URL.Path, e) },
	}

	log.Println("Serving DOS3.3 DSK filesystem over WebDAV")
	log.Println(" Address:", uri)
	for _, dsk := range dosfs.disks {
		log.Printf("          %s/%s/\n", uri, url.PathEscape(diskName(dsk)))
	}

	return http.ListenAndServe(addr, &handler)
}

/// dos33FS

type dos33FS struct {
	created time.Time
	disks   []*diskette
}

var (
	// Directories under each DSK folder.
	dskDirs = w("files applesoft binary intbasic text a b r s locks")
	// Files under each DSK folder.
	dskFiles = w("CATALOG VTOC")
)

func (fs *dos33FS) OpenFile(ctx context.Context, name string, flag int, perm fs.FileMode) (webdav.File, error) {
	req := fs.parsePath(name)

	if req.IsRoot() {
		return rootDirectory(fs), nil
	}

	if req.IsReadme() {
		return newMemFile("README", readme, fs.created), nil
	}

	if dsk := req.DiskRoot(); dsk != nil {
		return diskDirectory(dsk), nil
	}

	if dsk, folder := req.DiskDir(); dsk != nil {
		return &directory{fileInfo: *newDirInfo(folder, dsk.ModTime())}, nil
	}

	if dsk, name := req.DiskSpecial(); dsk != nil {
		switch name {
		case "CATALOG":
			return dsk.CATALOGFile()
		case "VTOC":
			return dsk.VTOCFile()
		}
		panic("Logic error: unknown special file: " + name)
	}

	return nil, http.ErrMissingFile
}

func (fs *dos33FS) Stat(ctx context.Context, name string) (fs.FileInfo, error) {
	req := fs.parsePath(name)

	if req.IsRoot() {
		return newDirInfo("/", fs.created), nil
	}

	if req.IsReadme() {
		return newFileInfo("README", fs.created), nil
	}

	if dsk := req.DiskRoot(); dsk != nil {
		return newDirInfo(dsk.name, dsk.ModTime()), nil
	}

	if dsk, folder := req.DiskDir(); dsk != nil {
		return newDirInfo(folder, dsk.ModTime()), nil
	}

	if dsk, name := req.DiskSpecial(); dsk != nil {
		return newFileInfo(name, dsk.ModTime()), nil
	}

	return nil, http.ErrMissingFile
}

func (*dos33FS) Mkdir(_ context.Context, _ string, _ fs.FileMode) error { return errors.ErrUnsupported }
func (fs *dos33FS) RemoveAll(_ context.Context, _ string) error         { return errors.ErrUnsupported }
func (fs *dos33FS) Rename(_ context.Context, _ string, _ string) error  { return errors.ErrUnsupported }

func (fs *dos33FS) find(name string) *diskette {
	for _, dsk := range fs.disks {
		if dsk.name == name {
			return dsk
		}
	}
	return nil
}

func (fs *dos33FS) parsePath(path string) (p fspath) {
	for _, part := range strings.Split(path, "/") {
		if part != "" {
			p.parts = append(p.parts, part)
		}
	}
	p.fs = fs
	return
}

type fspath struct {
	fs    *dos33FS
	parts []string
}

func (p fspath) IsRoot() bool   { return len(p.parts) == 0 }
func (p fspath) IsReadme() bool { return len(p.parts) == 1 && p.parts[0] == "README" }
func (p fspath) DiskRoot() *diskette {
	if len(p.parts) == 1 {
		return p.uncheckedFind()
	}
	return nil
}
func (p fspath) DiskDir() (*diskette, string) {
	if len(p.parts) == 2 && dskDirs.Contains(p.parts[1]) {
		if dsk := p.uncheckedFind(); dsk != nil {
			return dsk, p.parts[1]
		}
	}
	return nil, ""
}
func (p fspath) DiskSpecial() (*diskette, string) {
	if len(p.parts) == 2 && dskFiles.Contains(p.parts[1]) {
		if dsk := p.uncheckedFind(); dsk != nil {
			return dsk, p.parts[1]
		}
	}
	return nil, ""
}
func (p fspath) uncheckedFind() *diskette { return p.fs.find(p.parts[0]) }

// newFileSystem returns a new DOS 3.3 DSK Filesystem.
func newFileSystem(disks ...string) *dos33FS {
	fs := &dos33FS{
		created: time.Now(),
	}
	for _, name := range disks {
		dsk, err := loadDiskette(name)
		if err != nil {
			log.Fatalln("Could not load diskette:", name, err)
			continue
		}
		fs.disks = append(fs.disks, dsk)
	}
	return fs
}

func rootDirectory(fs *dos33FS) *directory {
	return &directory{
		fileInfo: *newDirInfo("/", fs.created),
		children: slices.Concat(
			dirs(transform(fs.disks, diskName)...),
			files("README"),
		),
	}
}

func diskDirectory(dsk *diskette) *directory {
	return &directory{
		fileInfo: *newDirInfo(dsk.name, dsk.ModTime()),
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

func dirs(names ...string) []fs.FileInfo {
	return transform(names, func(name string) fs.FileInfo { return newDirInfo(name, time.Time{}) })
}

func files(names ...string) (infos []fs.FileInfo) {
	return transform(names, func(name string) fs.FileInfo { return newFileInfo(name, time.Time{}) })
}

func newDirInfo(name string, modTime time.Time) *fileInfo {
	return &fileInfo{
		name:    name,
		mode:    fs.ModeDir | fs.ModePerm,
		modTime: modTime,
		isDir:   true,
	}
}

func newFileInfo(name string, modTime time.Time) *fileInfo {
	return &fileInfo{
		name:    name,
		mode:    0444,
		modTime: modTime,
	}
}

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

func newMemFile(name, content string, modTime time.Time) *memFile {
	return &memFile{
		fileInfo: fileInfo{
			name:    name,
			mode:    0444,
			modTime: modTime,
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
