//go:build !windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
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

// readUsage lists every process: its share of one core in percent, its memory in KB, and its children.
func readUsage() (map[int]float64, map[int]int64, map[int][]int) {
	cpu, rss, kids := map[int]float64{}, map[int]int64{}, map[int][]int{}
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,%cpu=,rss=").Output()
	if err != nil {
		return cpu, rss, kids
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) != 4 {
			continue
		}
		pid, e1 := strconv.Atoi(f[0])
		ppid, e2 := strconv.Atoi(f[1])
		c, e3 := strconv.ParseFloat(strings.Replace(f[2], ",", ".", 1), 64) // some locales print 1,5
		m, e4 := strconv.ParseInt(f[3], 10, 64)
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
			continue
		}
		cpu[pid], rss[pid] = c, m
		kids[ppid] = append(kids[ppid], pid)
	}
	return cpu, rss, kids
}
