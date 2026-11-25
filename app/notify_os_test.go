package main

import "runtime"

// runtimeAlerts: this system has a way to show a notification.
func runtimeAlerts() bool { return runtime.GOOS == "darwin" || runtime.GOOS == "linux" }
