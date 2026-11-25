//go:build !darwin || !cgo

package main

import "context"

// Notifications of the app itself exist on macOS only (notify_darwin.go); elsewhere the system command is used.
func nativeNotifyInit(ctx context.Context)                        {}
func nativeNotify(title, sub, body, slug string, sound bool) bool { return false }
