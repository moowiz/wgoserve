package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/moowiz/wgoserve"
)

var (
	listen = flag.String("listen", ":8080", "listen address")
	dir    = flag.String("dir", ".", "directory to serve")
)

// serveFiles serves files from disk.
func serveFiles(w http.ResponseWriter, req *http.Request) {
	if strings.HasSuffix(req.URL.Path, ".wasm") {
		w.Header().Set("content-type", "application/wasm")
	}

	http.FileServer(http.Dir(*dir)).ServeHTTP(w, req)
}

func main() {
	flag.Parse()
	log.Printf("listening on %q...", *listen)
	if len(os.Args) == 1 {
		fmt.Fprintf(os.Stderr, "usage: %s <directory> [flags to go build]\nFlags default are \"-o <directory>/out.wasm\"", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
		os.Exit(2)
	}
	mainTarget := os.Args[1]
	var err error
	mainTarget, err = filepath.Abs(mainTarget)
	if err != nil {
		panic(fmt.Errorf("%v can't be coerced to an absolute path", mainTarget))
	}

	var flags []string
	if len(os.Args) > 2 {
		flags = os.Args[2:]
	} else {
		info, err := os.Stat(mainTarget)
		if err != nil {
			panic(err)
		}
		if !info.IsDir() {
			mainTarget = filepath.Dir(mainTarget)
		}
		flags = []string{"-o", filepath.Join(mainTarget, "out.wasm")}
	}
	http.HandleFunc("/version", wgoserve.KeepFresh(mainTarget, flags))
	http.HandleFunc("/", serveFiles)
	log.Fatal(http.ListenAndServe(*listen, nil))
}
