package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"taeber.rapczak.com/webdavfs/examples/dos33"
)

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

	dos33.ListenAndServe(*addr, *prefix, disks...)
}
