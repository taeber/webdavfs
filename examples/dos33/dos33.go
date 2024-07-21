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
	"os"
	"strings"
	"time"

	"golang.org/x/net/webdav"
	"taeber.rapczak.com/webdavfs/examples/dos33/dsk"
)

type specialName = string

func snReadme() specialName                 { return "README.txt" }
func snDos() specialName                    { return "_dos" }
func snCatalog() specialName                { return "CATALOG.txt" }
func snVtoc() specialName                   { return "VTOC.txt" }
func snLock(filename string) specialName    { return fmt.Sprintf("%s,locked", filename) }
func snDeleted(filename string) specialName { return fmt.Sprintf("_%s.garbage", filename) }

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
		log.Printf("          %s/%s/\n", uri, url.PathEscape(dsk.Name()))
	}

	return http.ListenAndServe(addr, &handler)
}

// dos33FS is the [webdav.FileSystem] implementation for DOS 3.3 Diskettes.
type dos33FS struct {
	created time.Time
	disks   []*dsk.Diskette
	// type [webdav.FileSystem] interface
}

func (dfs *dos33FS) OpenFile(_ context.Context, name string, _ int, _ fs.FileMode) (webdav.File, error) {
	root := &rootDir{dfs: dfs}
	name = strings.TrimLeft(name, "/")
	if file, err := walk(root, name); err != nil {
		return nil, err
	} else {
		return file.Open()
	}
}

func (dfs *dos33FS) Stat(_ context.Context, name string) (fs.FileInfo, error) {
	root := &rootDir{dfs: dfs}
	name = strings.TrimLeft(name, "/")
	if file, err := walk(root, name); err != nil {
		return nil, err
	} else {
		return file.Stat()
	}
}

func walk(parent fileWrapper, pathname string) (fileWrapper, error) {
	if pathname == "" {
		return parent, nil
	}

	split := strings.SplitN(pathname, "/", 2)
	name := split[0]

	child, found := parent.Children()[name]
	if !found {
		return nil, os.ErrNotExist
	}
	if len(split) == 1 {
		return child, nil
	}
	if child.IsDir() {
		return walk(child, split[1])
	}
	return nil, os.ErrInvalid // child is not a directory
}

func (*dos33FS) Mkdir(_ context.Context, _ string, _ fs.FileMode) error { return errors.ErrUnsupported }
func (*dos33FS) RemoveAll(_ context.Context, _ string) error            { return errors.ErrUnsupported }
func (*dos33FS) Rename(_ context.Context, _ string, _ string) error     { return errors.ErrUnsupported }

// newFileSystem returns a new DOS 3.3 DSK Filesystem.
func newFileSystem(disks ...string) *dos33FS {
	dfs := dos33FS{created: time.Now()}
	for _, name := range disks {
		dsk, err := dsk.LoadDiskette(name)
		if err != nil {
			log.Fatalln("Could not load diskette:", name, err)
			continue
		}
		dfs.disks = append(dfs.disks, dsk)
	}
	return &dfs
}

// fileWrapper is the base interface for all dos33FS files.
type fileWrapper interface {
	Open() (webdav.File, error)
	Stat() (fs.FileInfo, error)
	Children() map[string]fileWrapper
	IsDir() bool
}

func readDir(file fileWrapper) ([]fs.FileInfo, error) {
	if !file.IsDir() {
		return nil, errors.ErrUnsupported
	}

	children := make([]fs.FileInfo, 0, len(file.Children()))
	for _, child := range file.Children() {
		if info, err := child.Stat(); err == nil {
			children = append(children, info)
		}
	}

	return children, nil
}

// fileInfo is the simplest implementation of [fs.FileInfo].
type fileInfo struct {
	name    string
	size    int64
	isDir   bool
	modTime time.Time
}

func (f *fileInfo) Name() string { return f.name }
func (f *fileInfo) Size() int64  { return f.size }
func (f *fileInfo) Mode() fs.FileMode {
	if f.isDir {
		return fs.ModeDir | fs.ModePerm
	} else {
		return fs.ModePerm
	}
}
func (f *fileInfo) ModTime() time.Time { return f.modTime }
func (f *fileInfo) IsDir() bool        { return f.Mode().IsDir() }
func (f *fileInfo) Sys() any           { return nil }

// anyDir is a partial implementation of [fileWrapper] methods common to any directory.
type anyDir struct{}

// func (dir *anyDir) Open() (webdav.File, error)         { return dir, nil }
// func (dir *anyDir) Readdir(int) ([]fs.FileInfo, error) { return readDir(dir) }
func (*anyDir) IsDir() bool                    { return true }
func (*anyDir) Close() error                   { return nil }
func (*anyDir) Read([]byte) (int, error)       { return -1, errors.ErrUnsupported }
func (*anyDir) Seek(int64, int) (int64, error) { return -1, errors.ErrUnsupported }
func (*anyDir) Write([]byte) (int, error)      { return -1, errors.ErrUnsupported }

// anyFile is a partial implementation of [fileWrapper] methods common to every file.
type anyFile struct{}

func (*anyFile) IsDir() bool                        { return false }
func (*anyFile) Close() error                       { return nil }
func (*anyFile) Children() map[string]fileWrapper   { return nil }
func (*anyFile) Readdir(int) ([]fs.FileInfo, error) { return nil, errors.ErrUnsupported }

// rootDir is
type rootDir struct {
	anyDir
	dfs *dos33FS
}

func (dir *rootDir) Open() (webdav.File, error)         { return dir, nil }
func (dir *rootDir) Readdir(int) ([]fs.FileInfo, error) { return readDir(dir) }
func (dir *rootDir) Stat() (fs.FileInfo, error) {
	return &fileInfo{
		modTime: dir.dfs.created,
		isDir:   true,
	}, nil
}
func (dir *rootDir) Children() map[string]fileWrapper {
	kids := make(map[string]fileWrapper)
	kids[snReadme()] = newMemFile(snReadme(), readme, dir.dfs.created)
	for _, dsk := range dir.dfs.disks {
		kids[dsk.Name()] = &dskDir{dsk: dsk}
	}
	return kids
}

// memDir is an in-memory directory.
type memDir struct {
	anyDir
	name     string
	modTime  time.Time
	children map[string]fileWrapper
}

func (dir *memDir) Open() (webdav.File, error)         { return dir, nil }
func (dir *memDir) Readdir(int) ([]fs.FileInfo, error) { return readDir(dir) }
func (dir *memDir) Stat() (fs.FileInfo, error) {
	return &fileInfo{
		name:    dir.name,
		isDir:   true,
		modTime: dir.modTime,
	}, nil
}
func (dir *memDir) Children() map[string]fileWrapper { return dir.children }

// dskDir
type dskDir struct {
	anyDir
	dsk *dsk.Diskette
}

func (dir *dskDir) Open() (webdav.File, error)         { return dir, nil }
func (dir *dskDir) Readdir(int) ([]fs.FileInfo, error) { return readDir(dir) }
func (dir *dskDir) Stat() (fs.FileInfo, error) {
	return &fileInfo{
		name:    dir.dsk.Name(),
		isDir:   true,
		modTime: dir.dsk.ModTime(),
	}, nil
}
func (dir *dskDir) Children() map[string]fileWrapper {
	kids := make(map[string]fileWrapper)
	kids[snDos()] = &memDir{
		name:    snDos(),
		modTime: dir.dsk.ModTime(),
		children: map[string]fileWrapper{
			snCatalog(): newMemFile(snCatalog(), dsk.RunCatalog(dir.dsk), dir.dsk.ModTime()),
			snVtoc():    newMemFile(snVtoc(), dir.dsk.VTOCFile(), dir.dsk.ModTime()),
		},
	}
	for _, file := range dir.dsk.Catalog() {
		// TODO: handle the case where the path-safe name conflicts (like inverted HELLO and HELLO)
		name := file.Name().PathSafe()
		if file.IsDeleted() {
			name = snDeleted(name)
		}
		if file.IsLocked() {
			kids[snLock(name)] = newMemFile(snLock(name), "", dir.dsk.ModTime())
		}
		kids[name] = &dskFile{dsk: dir.dsk, file: file}
	}

	return kids
}

// dskFile is a raw (binary) representation of a file on diskette.
type dskFile struct {
	anyFile
	dsk     *dsk.Diskette
	file    dsk.FileEntry
	content *bytes.Reader
}

func (f *dskFile) Open() (webdav.File, error) { return f, nil }
func (f *dskFile) Read(p []byte) (int, error) {
	if err := f.load(); err != nil {
		return 0, err
	}
	return f.content.Read(p)
}
func (f *dskFile) Seek(offset int64, whence int) (int64, error) {
	if err := f.load(); err != nil {
		return 0, err
	}
	return f.content.Seek(offset, whence)
}
func (*dskFile) Write([]byte) (int, error) { return 0, os.ErrInvalid }
func (f *dskFile) Stat() (fs.FileInfo, error) {
	name := f.file.Name().PathSafe()
	if f.file.IsDeleted() {
		name = snDeleted(name)
	}
	return &fileInfo{
		name:    name,
		size:    int64(f.file.SectorsUsed() * f.dsk.SectorSize()),
		modTime: f.dsk.ModTime(),
	}, nil
}

func (f *dskFile) load() error {
	if f.content == nil {
		buf, err := f.dsk.ReadAll(f.file)
		if err != nil {
			return err
		}
		f.content = bytes.NewReader(buf)
	}
	return nil
}

// memFile is an in-memory file.
type memFile struct {
	anyFile
	name    string
	modTime time.Time
	content *bytes.Reader
}

func (file *memFile) Open() (webdav.File, error) { return file, nil }
func (f *memFile) Read(p []byte) (int, error)    { return f.content.Read(p) }
func (f *memFile) Seek(offset int64, whence int) (int64, error) {
	return f.content.Seek(offset, whence)
}
func (f *memFile) Stat() (fs.FileInfo, error) {
	return &fileInfo{
		name:    f.name,
		size:    f.content.Size(),
		modTime: f.modTime,
	}, nil
}
func (*memFile) Write(p []byte) (int, error) { return 0, os.ErrInvalid }

func newMemFile(name, content string, modTime time.Time) *memFile {
	return &memFile{
		name:    name,
		modTime: modTime,
		content: bytes.NewReader([]byte(content)),
	}
}

// Contents of README.txt.
const readme = `DOS 3.3 DSK Filesystem Folder Structure

TODO: update with new structure

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
