package wasm

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"syscall/js"
)

const queryParam = "gowasmfreshversion"

func getQueryJS() (url.Values, error) {
	queryS := js.Global().Get("window").Get("location").Get("search").String()
	if len(queryS) > 0 {
		// Strip out the leading "?"
		queryS = queryS[1:]
	}
	v, err := url.ParseQuery(queryS)
	if err != nil {
		return url.Values{}, err
	}
	return v, nil
}

func setQueryJS(v url.Values) {
	js.Global().Get("window").Get("location").Set("search", v.Encode())
}

func getLatestVersion(endpoint string) string {
	resp, err := http.Get(endpoint)
	if err != nil {
		panic(err)
	}
	data, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		panic(err)
	}
	return string(data)
}

// EnsureFresh ensures the code is fresh
func EnsureFresh(endpoint string) {
	var loopCB js.Callback
	loopCB = js.NewCallback(func(args []js.Value) {
		// Need to make this a goroutine so it doesn't block. Or something...
		go func() {
			ensureFresh(endpoint)
			js.Global().Call("setTimeout", loopCB)
		}()
	})
	js.Global().Call("setTimeout", loopCB)
}

func ensureFresh(endpoint string) {
	version := getLatestVersion(endpoint)
	if version == "timeout" {
		// Nothing new, just retry.
		return
	}
	q, err := getQueryJS()
	if err != nil {
		panic(err)
	}
	if version != q.Get(queryParam) {
		q.Set(queryParam, version)
		setQueryJS(q)
	}
}
