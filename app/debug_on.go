//go:build termdebug

package main

// A build for testing the terminal pages in a browser engine without the app window: `go build -tags termdebug`.
// It gives the 127.0.0.1 port the rights of the app window, so it must never be installed or released.
const debugLocal = true
