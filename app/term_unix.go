//go:build !windows

package main

import (
	"io"
	"os/exec"

	"github.com/creack/pty"
)

const termSupported = true

func winsize(cols, rows int) *pty.Winsize {
	if cols <= 0 || cols > 1000 {
		cols = 120
	}
	if rows <= 0 || rows > 1000 {
		rows = 32
	}
	return &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)}
}

// startPty runs cmd in a new pseudo-terminal; the returned func changes the size of its screen.
func startPty(cmd *exec.Cmd, cols, rows int) (io.ReadWriteCloser, func(cols, rows int), error) {
	f, err := pty.StartWithSize(cmd, winsize(cols, rows))
	if err != nil {
		return nil, nil, err
	}
	return f, func(c, r int) { _ = pty.Setsize(f, winsize(c, r)) }, nil
}
