package main

import (
	"runtime"
	"syscall"
	"time"
)

var setThreadExecutionState = syscall.NewLazyDLL("kernel32.dll").NewProc("SetThreadExecutionState")

const (
	esContinuous      = 0x80000000
	esSystemRequired  = 0x00000001
	esDisplayRequired = 0x00000002
)

func awakeCan() bool { return setThreadExecutionState.Find() == nil }

// holdAwake on Windows: SetThreadExecutionState belongs to a thread, so one goroutine locked to its thread holds it
// until it is told to stop (or the process ends). The time is the timer's.
func holdAwake(_ time.Duration, screen bool) (func(), error) {
	flags := uintptr(esContinuous | esSystemRequired)
	if screen {
		flags |= esDisplayRequired
	}
	done, ok := make(chan struct{}), make(chan error)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if r, _, e := setThreadExecutionState.Call(flags); r == 0 {
			ok <- e
			return
		}
		ok <- nil
		<-done
		setThreadExecutionState.Call(uintptr(esContinuous))
	}()
	if err := <-ok; err != nil {
		return nil, err
	}
	return func() { close(done) }, nil
}
