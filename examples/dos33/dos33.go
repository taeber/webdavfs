package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/net/webdav"
)

type arrayFlags []string

func (i *arrayFlags) String() string {
	return strings.Join(*i, "\t")
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

/*
type dos33FS struct{}

func (*dos33FS) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	return errors.ErrUnsupported
}

func (hi *dos33FS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	if name == folder.name {
		return &folder, nil
	}
	if name == "/"+hello.name {
		return &hello, nil
	}
	return nil, http.ErrMissingFile
}

func (hi *dos33FS) RemoveAll(ctx context.Context, name string) error {
	return errors.ErrUnsupported
}

func (hi *dos33FS) Rename(ctx context.Context, oldName, newName string) error {
	return errors.ErrUnsupported
}

func (hi *dos33FS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	if name == folder.name {
		return &folder, nil
	}
	if name == "/"+hello.name {
		return &hello, nil
	}
	return nil, http.ErrMissingFile
}

type file struct {
	name    string
	content string
}

func (info *file) Mode() fs.FileMode {
	if strings.HasSuffix(info.name, "/") {
		return fs.ModeDir | 0444
	}
	return 0444
}
func (info *file) Name() string       { return info.name }
func (info *file) ModTime() time.Time { return time.Unix(0, 0) }
func (info *file) IsDir() bool        { return info.Mode().IsDir() }
func (info *file) Sys() any           { return nil }
func (info *file) Size() int64        { return int64(len(info.content)) }

func (f *file) Close() error { return nil }
func (f *file) Read(p []byte) (n int, err error) {
	return bytes.NewBufferString(f.content).Read(p)
}
func (f *file) Stat() (fs.FileInfo, error)        { return f, nil }
func (f *file) Write(p []byte) (n int, err error) { return 0, errors.ErrUnsupported }
func (f *file) Seek(offset int64, whence int) (int64, error) {
	if !f.IsDir() {
		switch whence {
		case io.SeekStart:
			if offset == 0 {
				return 0, nil
			}
		case io.SeekEnd:
			if offset == 0 {
				return f.Size(), nil
			}
		}
	}
	return 0, errors.ErrUnsupported
}
func (file *file) Readdir(count int) ([]fs.FileInfo, error) {
	if !file.IsDir() {
		return nil, errors.ErrUnsupported
	}
	return []fs.FileInfo{&hello}, nil
}
*/

func main() {
	var disks arrayFlags
	addr := flag.String("addr", "127.0.0.1:33333", "HTTP address on which to listen")
	prefix := flag.String("prefix", "/dos33", "URL path prefix")
	flag.Var(&disks, "dsk", "One or more DOS 3.3 DSK files")
	flag.Parse()

	loc := fmt.Sprintf("http://%s%s", *addr, *prefix)
	uri, err := url.Parse(loc)
	if err != nil {
		log.Fatalln(err)
	}

	fs := webdav.NewMemFS()
	if err := writeReadOnlyFile(fs, "README", readme); err != nil {
		log.Fatalln(err)
	}
	if err := writeReadOnlyFile(fs, "index.html", `<!doctype html>
		<html>
		<head><title>Yo, WebDAV!</title></head>
		<body><h1>Yo, yo yo!</h1></body>
		</html>`); err != nil {
		log.Fatalln(err)
	}

	for _, dskpath := range disks {
		dsk := filepath.Base(dskpath)
		fs.Mkdir(context.TODO(), dsk, 0777)

		dirs := []string{"files", "applesoft", "binary", "intbasic", "text", "a", "b", "r", "s", "locks"}
		for _, dir := range dirs {
			fs.Mkdir(context.TODO(), dsk+"/"+dir, 0777)
		}

		if err := writeReadOnlyFile(fs, dsk+"/catalog", "Not implemented yet"); err != nil {
			log.Fatalln(err)
		}
		if err := writeReadOnlyFile(fs, dsk+"/vtoc", "Not implemented yet"); err != nil {
			log.Fatalln(err)
		}
	}

	handler := webdav.Handler{
		Prefix:     *prefix,
		LockSystem: webdav.NewMemLS(),
		FileSystem: fs,
		Logger:     func(r *http.Request, e error) { log.Println(r.Method, r.URL.Path, e) },
	}

	log.Println("Serving DOS3.3 DSK filesystem over WebDAV")
	log.Println(" Address:", uri)
	http.ListenAndServe(*addr, &handler)
}

const readme = `DOS 3.3 DSK Filesystem Folder Structure

Each DSK is represented as a folder with the following files and folders.

  files/      Read-only versions of all files, as raw binary.
  catalog     a close approximation of running CATLOG from DOS.
  locks/      All locked files. Lock a file by adding it, unlock by deleting it.
	vtoc        Volume Table of Contents information that might be helpful.

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

func writeReadOnlyFile(fs webdav.FileSystem, filename, contents string) error {
	file, err := fs.OpenFile(context.TODO(), filename, os.O_CREATE|os.O_WRONLY, 0444)
	if err != nil {
		return err
	}
	file.Write(bytes.NewBufferString(contents).Bytes())
	return nil
}
