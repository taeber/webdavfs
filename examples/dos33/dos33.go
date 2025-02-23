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
	"path"
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
func parseLockName(lockfile string) (string, bool) {
	name := strings.TrimSuffix(lockfile, ",locked")
	if name != lockfile {
		return name, true
	} else {
		return "", false
	}
}

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

func (dfs *dos33FS) OpenFile(_ context.Context, name string, _ int, mode fs.FileMode) (webdav.File, error) {
	writePerms := mode.Perm()&0222 != 0
	root := &rootDir{dfs: dfs}
	name = strings.TrimLeft(name, "/")
	file, basedir, err := walk(root, name)
	if errors.Is(err, os.ErrNotExist) && basedir != nil && writePerms {
		return basedir.Create(path.Base(name))
	} else if err != nil {
		return nil, err
	} else {
		return file.Open()
	}
}

func (dfs *dos33FS) Stat(_ context.Context, name string) (fs.FileInfo, error) {
	root := &rootDir{dfs: dfs}
	name = strings.TrimLeft(name, "/")
	if file, _, err := walk(root, name); err != nil {
		return nil, err
	} else {
		return file.Stat()
	}
}

func walk(parent fileWrapper, pathname string) (file, prev fileWrapper, err error) {
	if pathname == "" {
		return parent, nil, nil
	}

	split := strings.SplitN(pathname, "/", 2)
	name := split[0]

	child, found := parent.Children()[name]
	if !found {
		return nil, parent, os.ErrNotExist
	}
	if len(split) == 1 {
		return child, parent, nil
	}
	if child.IsDir() {
		return walk(child, split[1])
	}
	return nil, parent, os.ErrInvalid // child is not a directory
}

func (*dos33FS) Mkdir(context.Context, string, fs.FileMode) error { return errors.ErrUnsupported }
func (*dos33FS) Rename(context.Context, string, string) error     { return errors.ErrUnsupported }
func (dfs *dos33FS) RemoveAll(_ context.Context, name string) error {
	root := &rootDir{dfs: dfs}
	name = strings.TrimLeft(name, "/")
	if file, _, err := walk(root, name); err != nil {
		return err
	} else {
		return file.Delete()
	}
}

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

	IsDir() bool
	Children() map[string]fileWrapper
	Create(string) (webdav.File, error)

	Delete() error
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
func (*anyDir) Delete() error                  { return errors.ErrUnsupported }
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
func (*anyFile) Create(string) (webdav.File, error) { return nil, errors.ErrUnsupported }

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
func (*rootDir) Create(string) (webdav.File, error) { return nil, errors.ErrUnsupported }

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
func (*memDir) Create(string) (webdav.File, error)   { return nil, errors.ErrUnsupported }

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
			kids[snLock(name)] = &lockFile{dsk: dir.dsk, file: file}
		}
		kids[name] = &dskFile{dsk: dir.dsk, file: file}
	}

	return kids
}
func (dir *dskDir) Create(name string) (webdav.File, error) {
	if filename, ok := parseLockName(name); ok {
		file := dir.dsk.FindFile(filename)
		if file == nil {
			return nil, errors.ErrUnsupported
		}
		if err := dir.dsk.Lock(file); err != nil {
			return nil, err
		}
		lck := lockFile{dsk: dir.dsk, file: file}
		return lck.Open()
	}
	return nil, errors.ErrUnsupported
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
func (*dskFile) Write([]byte) (int, error) { return 0, errors.ErrUnsupported }
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
func (f *dskFile) Delete() error {
	return f.dsk.Delete(f.file)
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

type lockFile struct {
	anyFile
	dsk  *dsk.Diskette
	file dsk.FileEntry
}

func (lck *lockFile) Open() (webdav.File, error) {
	return newMemFile(snLock(lck.file.Name().PathSafe()), "", lck.dsk.ModTime()), nil
}
func (lck *lockFile) Delete() error {
	return lck.dsk.Unlock(lck.file)
}
func (lck *lockFile) Stat() (fs.FileInfo, error) {
	name := lck.file.Name().PathSafe()
	if lck.file.IsDeleted() {
		name = snDeleted(name)
	}
	name = snLock(name)
	return &fileInfo{
		name:    name,
		modTime: lck.dsk.ModTime(),
	}, nil
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
func (*memFile) Write(p []byte) (int, error) { return 0, errors.ErrUnsupported }

func (*memFile) Delete() error { return errors.ErrUnsupported }

func newMemFile(name, content string, modTime time.Time) *memFile {
	return &memFile{
		name:    name,
		modTime: modTime,
		content: bytes.NewReader([]byte(content)),
	}
}

// Contents of README.txt.
const readme = `DOS 3.3 DSK Filesystem Folder Structure

Each DSK is represented as a folder containing all the files on it.

**Locks**

There are also lock files (ending in ",locked") which represent the lock
state of the file.
You can delete the lock to unlock a file.
You can create a lock to lock a file.

**Garbage Files**

Files that have been deleted can be viewed as well.
They start with an underscore and end with ".garbage".

**_dos/**

The _dos directory contains special files and folders.

  CATALOG.txt  a close approximation of running CATLOG from DOS.
  VTOC.txt     Volume Table of Contents information that might be helpful.

In the future, there will be special "text" folders, for view BASIC and TEXT
files as regular text. Conversion will happen automatically on load and save!

  _dos/applesoft/
  _dos/intbasic/
  _dos/text/
`
