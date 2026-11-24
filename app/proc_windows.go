//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// exeOf is the file a running process was started from, "" when there is no such process.
func exeOf(pid int) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	var code uint32
	if windows.GetExitCodeProcess(h, &code) != nil || code != 259 { // 259: STILL_ACTIVE
		return ""
	}
	buf := make([]uint16, windows.MAX_PATH)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

// isWatcher: the command line of another process is not easy to read on Windows, so the program is checked:
// only a watcher writes the pid file, and a number reused by another program has another file.
func isWatcher(pid int) bool {
	return strings.EqualFold(filepath.Base(exeOf(pid)), "whatnow.exe")
}

func stopProcess(pid int) {
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
}

// parentGone: Windows does not hand orphans over to another process, so the parent itself is looked up.
func parentGone(ppid int) bool { return exeOf(ppid) == "" }
