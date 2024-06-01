package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path/filepath"

	"golang.org/x/net/webdav"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:33333", "HTTP address on which to listen")
	root := flag.String("root", ".", "root path of files to serve")
	prefix := flag.String("prefix", "", "URL path prefix")
	flag.Parse()

	loc := fmt.Sprintf("http://%s%s", *addr, *prefix)
	uri, err := url.Parse(loc)
	if err != nil {
		log.Fatalln(err)
	}

	abs, err := filepath.Abs(*root)
	if err != nil {
		log.Fatalln(err)
	}

	handler := webdav.Handler{
		Prefix:     *prefix,
		LockSystem: webdav.NewMemLS(),
		FileSystem: webdav.Dir(abs),
		Logger:     func(r *http.Request, e error) { log.Println(r.Method, r.URL.Path, e) },
	}

	log.Println("Serving passthru filesystem over WebDAV")
	log.Println("    Root:", abs)
	log.Println(" Address:", uri)
	http.ListenAndServe(*addr, &handler)
}
