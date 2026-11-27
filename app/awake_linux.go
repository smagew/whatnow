package main

import (
	"errors"
	"os/exec"
	"syscall"
	"time"
)

func awakeCan() bool {
	_, err := exec.LookPath("systemd-inhibit")
	return err == nil
}

// holdAwake on Linux is a systemd inhibitor lock held by `systemd-inhibit cat`. cat reads a pipe of this process:
// when the process ends, even without a word, the pipe closes, cat ends and the lock goes. The time is the timer's.
func holdAwake(_ time.Duration, screen bool) (func(), error) {
	p, err := exec.LookPath("systemd-inhibit")
	if err != nil {
		return nil, errors.New("systemd-inhibit not found")
	}
	what := "--what=sleep:idle" // idle also keeps the screen from blanking where the desktop honours it
	_ = screen
	cmd := exec.Command(p, what, "--who=whatnow", "--why=Claude Code sessions are working", "--mode=block", "cat")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go func() { _ = cmd.Wait() }()
	return func() {
		_ = in.Close()
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}, nil
}
