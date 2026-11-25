package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The pseudo-terminal of this system really runs a program: its output is read, a line typed into it arrives.
func TestAProgramRunsInThePseudoTerminal(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "echo hello-pty; read x; echo got-$x")
	if runtime.GOOS == "windows" {
		cmd = exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe"), "/d", "/q", "/v:on", "/c", "echo hello-pty& set /p x=& echo got-!x!")
	}
	tty, resize, err := startPty(cmd, 80, 24)
	if err != nil {
		t.Fatalf("startPty: %v", err)
	}
	defer tty.Close()
	resize(100, 30)
	out := make(chan string, 4)
	go func() {
		var all []byte
		b := make([]byte, 4096)
		said := false
		for {
			n, err := tty.Read(b)
			all = append(all, b[:n]...)
			if !said && strings.Contains(string(all), "hello-pty") {
				said = true
				out <- "hello"
			}
			if strings.Contains(string(all), "got-key") {
				out <- "got"
				return
			}
			if err != nil {
				out <- "end: " + string(all)
				return
			}
		}
	}()
	wait := func(want string) {
		t.Helper()
		select {
		case s := <-out:
			if s != want {
				t.Fatalf("want %q, the terminal gave %q", want, s)
			}
		case <-time.After(20 * time.Second):
			t.Fatalf("no %q from the terminal in 20 seconds", want)
		}
	}
	wait("hello")
	if _, err := tty.Write([]byte("key\r")); err != nil {
		t.Fatalf("write: %v", err)
	}
	wait("got")
	if cmd.Process == nil {
		t.Error("cmd.Process is not set: the terminal could not be stopped")
	}
}

// claude installed by npm on Windows is a .cmd file: it is run through cmd.exe, with its arguments, also from a
// folder with a space in its name.
func TestACmdFileRunsInThePseudoTerminal(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	f := filepath.Join(t.TempDir(), "fake claude.cmd")
	if err := os.WriteFile(f, []byte("@echo off\r\necho cmd-says-%1-%~2\r\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(f, "attach", "a b")
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	tty, _, err := startPty(cmd, 80, 24)
	if err != nil {
		t.Fatalf("startPty: %v", err)
	}
	defer tty.Close()
	got := make(chan string, 1)
	go func() {
		var all []byte
		b := make([]byte, 4096)
		for {
			n, err := tty.Read(b)
			all = append(all, b[:n]...)
			if strings.Contains(string(all), "cmd-says-attach-a b") || err != nil {
				got <- string(all)
				return
			}
		}
	}()
	select {
	case s := <-got:
		if !strings.Contains(s, "cmd-says-attach-a b") {
			t.Errorf("the .cmd file did not get its arguments: %q", s)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("nothing came out of the .cmd file in 20 seconds")
	}
}
