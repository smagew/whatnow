//go:build darwin && cgo

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
static void wnActivate(void) {
	dispatch_async(dispatch_get_main_queue(), ^{ [NSApp activateIgnoringOtherApps:YES]; });
}
*/
import "C"

// appActivate makes this app the active one: a window shown from another process's request comes to the front.
func appActivate() { C.wnActivate() }
