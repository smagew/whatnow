//go:build windows

package main

import (
	"errors"
	"io"
	"os/exec"
)

// The terminal needs ConPTY on Windows; not written yet, so the window does not offer it there.
const termSupported = false

func startPty(cmd *exec.Cmd, cols, rows int) (io.ReadWriteCloser, func(cols, rows int), error) {
	return nil, nil, errors.New("not supported on Windows yet")
}
