package wgoserve

import (
	"crypto/sha1"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const emptySHA1 = "da39a3ee5e6b4b0d3255bfef95601890afd80709"

func getFileHash(filepath string) string {
	f, err := os.Open(filepath)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

type fileMap map[string]string

func buildFileMap(mainTarget string) fileMap {
	fileMap := map[string]string{}
	for _, filename := range getImportantFiles(mainTarget) {
		fileMap[filename] = getFileHash(filename)
	}
	for _, filepath := range getDependantPackageFiles(mainTarget) {
		fileMap[filepath] = getFileHash(filepath)
	}
	return fileMap
}

func (f fileMap) TotalDigest() []byte {
	keys := make([]string, 0)
	for k := range f {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha1.New()
	for _, key := range keys {
		h.Write([]byte(fmt.Sprintf("%x", key+f[key])))
	}
	return h.Sum(nil)
}

var buildErrMessage string = "wgoserve: error encountered while compiling:\n%v\n"

func buildPackage(mainTarget string, flags []string) error {
	flags = append(flags, mainTarget)
	flags = append([]string{"build"}, flags...)
	cmd := exec.Command("go", flags...)
	cmd.Dir = filepath.Dir(mainTarget)
	cmd.Env = append(os.Environ(),
		"GOOS=js",
		"GOARCH=wasm",
	)
	stdout, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v. Stdout:\n%v", err, string(stdout))
	}
	fmt.Printf("%v: successfully compiled\n", time.Now().Format("15:04:05"))
	return nil
}

func getImportantFiles(dir string) []string {
	// Get package directory, and all relevant go files.
	// HACK: I hope nothing uses a "|" character.
	cmd := exec.Command("go", "list", "-f", "{{ .Dir }}|{{ .GoFiles }}", dir)
	cmd.Env = append(os.Environ(),
		"GOOS=js",
		"GOARCH=wasm",
	)
	stdout, err := cmd.CombinedOutput()
	if err != nil {
		panic(err)
	}
	res := strings.Split(string(stdout), "|")
	if len(res) != 2 {
		panic(fmt.Errorf("bad result from `go list`: %v", string(stdout)))
	}
	pkgDir := res[0]
	result := []string{}
	for _, filename := range parseGoListOutput(res[1]) {
		result = append(result, filepath.Join(pkgDir, filename))
	}
	return result
}

// parses the output of a `go list` command which outputs an array. Very hacky.
func parseGoListOutput(output string) []string {
	var res []string
	for _, val := range strings.Split(output, " ") {
		val = strings.TrimSpace(val)
		if strings.HasPrefix(val, "[") {
			val = val[1:]
		}
		if strings.HasSuffix(val, "]") {
			val = val[:len(val)-1]
		}
		res = append(res, val)
	}
	return res
}

// Gets a list of all of the go files in all packages this package depends on.
// It's a bit slow, because it shells out to `go list` a bunch. Could maybe speed it up, it takes ~2 seconds right now.
func getDependantPackageFiles(mainTarget string) []string {
	cmd := exec.Command("go", "list", "-f", "{{ .Deps }}", mainTarget)
	cmd.Dir = filepath.Dir(mainTarget)
	stdout, err := cmd.CombinedOutput()
	if err != nil {
		panic(err)
	}
	res := []string{}
	mu := &sync.Mutex{}
	wg := &sync.WaitGroup{}
	for _, pkg := range parseGoListOutput(string(stdout)) {
		pkg := pkg
		wg.Add(1)
		go func() {
			impFiles := getImportantFiles(pkg)
			mu.Lock()
			defer mu.Unlock()
			for _, importantFile := range impFiles {
				res = append(res, importantFile)
			}
			wg.Done()
		}()
	}
	wg.Wait()
	return res
}

func makeWatcher(fileMap fileMap) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	numAdded := 0
	for path := range fileMap {
		if err := watcher.Add(path); err != nil {
			return nil, err
		}
		numAdded++
	}
	fmt.Printf("watching %v files\n", numAdded)
	return watcher, nil
}

// KeepFresh keeps the code fresh
func KeepFresh(mainTarget string, flags []string) func(http.ResponseWriter, *http.Request) {
	digestChannel := make(chan []byte, 20)
	go func() {
		var err error
		mainTarget, err = filepath.Abs(mainTarget)
		if err != nil {
			panic(fmt.Errorf("%v can't be coerced to an absolute path", mainTarget))
		}
		if err := buildPackage(mainTarget, flags); err != nil {
			fmt.Printf(buildErrMessage, err)
		}
		fileMap := buildFileMap(mainTarget)
		watcher, err := makeWatcher(fileMap)
		if err != nil {
			panic(err)
		}
		defer watcher.Close()
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				filename := filepath.Clean(event.Name)
				res, ok := fileMap[filename]
				if !ok {
					panic(fmt.Errorf("%v wasn't present in filemap, but was triggered", filename))
				}
				fmt.Printf("file %v changed, with op %v\n", filename, event.Op)
				newDigest := getFileHash(filename)
				if newDigest == emptySHA1 {
					// Visual Studio Code sometimes writes an empty file when editing a go file,
					// for some reason. Just ignore empty files, probably will error on compile anyways.
					continue
				}
				if newDigest != res {
					fileMap[filename] = newDigest
					if err := buildPackage(mainTarget, flags); err != nil {
						fmt.Printf(buildErrMessage, err)
					}
					digestChannel <- fileMap.TotalDigest()
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				fmt.Println("got a watcher error:", err)
			}
		}
	}()
	return func(w http.ResponseWriter, req *http.Request) {
		select {
		case digest, ok := <-digestChannel:
			if !ok {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "ERROR, bad value")
				return
			}

			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "%x", digest)
			return
		case <-time.After(20 * time.Second):
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "timeout")
			return
		}
	}
}
