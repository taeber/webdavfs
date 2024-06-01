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

	"golang.org/x/net/webdav"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:33333", "HTTP address on which to listen")
	prefix := flag.String("prefix", "", "URL path prefix")
	flag.Parse()

	loc := fmt.Sprintf("http://%s%s", *addr, *prefix)
	uri, err := url.Parse(loc)
	if err != nil {
		log.Fatalln(err)
	}

	fs := webdav.NewMemFS()
	file, err := fs.OpenFile(context.TODO(), "hello.txt", os.O_CREATE|os.O_WRONLY, 0444)
	if err != nil {
		log.Fatalln(err)
	}
	file.Write(bytes.NewBufferString("Hello.\n").Bytes())
	file.Close()

	handler := webdav.Handler{
		Prefix:     *prefix,
		LockSystem: webdav.NewMemLS(),
		FileSystem: fs,
		Logger:     func(r *http.Request, e error) { log.Println(r.Method, r.URL.Path, e) },
	}

	log.Println("Serving hello filesystem over WebDAV")
	log.Println(" Address:", uri)
	http.ListenAndServe(*addr, &handler)
}
