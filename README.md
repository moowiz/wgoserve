# wgoserve
wgoserve makes it easy to deploy web assembly modules written in golang

## Usage

In your web assembly program, do:

```

import (
    "github.com/moowiz/wgoserve/wasm"
)

func main() {
	go wasm.EnsureFresh("/version")
    ...
}
```

Then, run the wgoserve binary locally:
```
go get -u github.com/moowiz/wgoserve
wgoserve path/to/project
```
