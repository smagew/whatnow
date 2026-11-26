//go:build termdebug

package main

import "os"

// A build for testing the terminal pages in a browser engine without the app window: `go build -tags termdebug`.
// It gives the 127.0.0.1 port the rights of the app window, so it must never be installed or released.
const debugLocal = true

// WHATNOW_DEBUG_SOLO=<work>: the pages as the separate window of that work shows them (see window.go).
func init() {
	if s := os.Getenv("WHATNOW_DEBUG_SOLO"); winSlug(s) {
		soloSlug = s
	}
}
