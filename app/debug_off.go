//go:build !termdebug

package main

// debugLocal: the 127.0.0.1 port never gets the rights of the app window (the terminal).
const debugLocal = false
