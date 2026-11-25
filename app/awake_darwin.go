package main

import (
	"os"
	"os/exec"
	"strconv"
	"time"
)

func awakeCan() bool { return true }

// holdAwake on macOS is the system's own caffeinate: -i keeps it from sleeping when idle, -s also when the lid would
// let it (on power only), -w ends it with this process. d == 0: until it is stopped.
func holdAwake(d time.Duration) (func(), error) {
	args := []string{"-i", "-s", "-w", strconv.Itoa(os.Getpid())}
	if d > 0 {
		args = append(args, "-t", strconv.Itoa(int(d/time.Second)+1))
	}
	cmd := exec.Command("/usr/bin/caffeinate", args...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go func() { _ = cmd.Wait() }()
	return func() { _ = cmd.Process.Kill() }, nil
}
