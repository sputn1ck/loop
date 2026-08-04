//go:build !js || !wasm

package main

import (
	"fmt"
	"os"
)

// main reports the target constraint when the browser command is run as a
// native binary. Keeping this stub buildable lets ordinary Go tooling inspect
// and test the shared configuration parser.
func main() {
	_, _ = fmt.Fprintln(
		os.Stderr,
		"loop-wasm requires GOOS=js GOARCH=wasm",
	)
	os.Exit(1)
}
