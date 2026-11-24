//go:build !windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// isWatcher: a pid file outlives a reboot or a kill -9, and the number may by then belong to some other
// program, which must not get our SIGTERM. `ps` shows the command line, so the Python watcher counts too.
func isWatcher(pid int) bool {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	return err == nil && isWatcherCmd(string(out))
}

func stopProcess(pid int) { _ = syscall.Kill(pid, syscall.SIGTERM) }

// parentGone: when Claude Code exits, its hooks are handed over to init.
func parentGone(int) bool { return os.Getppid() == 1 }
