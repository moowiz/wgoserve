package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/moowiz/gowasmfresh"
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
		fmt.Fprintf(os.Stderr, "usage: %s <directory> [flags to go build]\nFlags default are \"-o out.wasm\"", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
		os.Exit(2)
	}
	mainTarget := os.Args[1]
	var flags []string
	if len(os.Args) > 2 {
		flags = os.Args[2:]
	} else {
		flags = []string{"-o", "out.wasm"}
	}
	http.HandleFunc("/version", gowasmfresh.KeepFresh(mainTarget, flags))
	http.HandleFunc("/", serveFiles)
	log.Fatal(http.ListenAndServe(*listen, nil))
}
