//go:build windows

package main

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The terminal on Windows is a pseudo console (ConPTY, Windows 10 1809 and newer): the program runs attached to
// it, what it draws comes out of one pipe as the same escape sequences a Unix terminal gets, the keys go into
// another. Written without a Windows machine at hand: it compiles and follows the documented order of calls, and
// it has not been run yet.
const termSupported = true

func coord(cols, rows int) windows.Coord {
	if cols <= 0 || cols > 1000 {
		cols = 120
	}
	if rows <= 0 || rows > 1000 {
		rows = 32
	}
	return windows.Coord{X: int16(cols), Y: int16(rows)}
}

type conpty struct {
	pc      windows.Handle
	in, out *os.File // in: the keys to the program; out: what it draws
	once    sync.Once
}

func (c *conpty) Read(p []byte) (int, error)  { return c.out.Read(p) }
func (c *conpty) Write(p []byte) (int, error) { return c.in.Write(p) }

// Close ends the console: the program in it gets the close event, and the reader of out gets the end of the pipe.
func (c *conpty) Close() error {
	c.once.Do(func() {
		_ = c.in.Close()
		windows.ClosePseudoConsole(c.pc)
		_ = c.out.Close()
	})
	return nil
}

// envBlock is the environment as CreateProcess takes it: NAME=value strings, each ended by a zero, and one more
// zero at the end.
func envBlock(env []string) *uint16 {
	var b []uint16
	for _, kv := range env {
		if kv == "" || strings.IndexByte(kv, 0) >= 0 {
			continue
		}
		b = append(b, utf16.Encode([]rune(kv))...)
		b = append(b, 0)
	}
	b = append(b, 0, 0)
	return &b[0]
}

// commandLine: a .cmd or .bat file (claude installed by npm) is not a program, cmd.exe runs it.
func commandLine(cmd *exec.Cmd) string {
	args := cmd.Args
	if len(args) == 0 {
		args = []string{cmd.Path}
	}
	args = append([]string{cmd.Path}, args[1:]...)
	switch strings.ToLower(filepath.Ext(cmd.Path)) {
	case ".cmd", ".bat":
		shell := os.Getenv("ComSpec")
		if shell == "" {
			shell = "cmd.exe"
		}
		// /s with one more pair of quotes around the whole line: cmd.exe then takes the inner quotes as they are
		return windows.ComposeCommandLine([]string{shell, "/d", "/s", "/c"}) + ` "` + windows.ComposeCommandLine(args) + `"`
	}
	return windows.ComposeCommandLine(args)
}

// startPty runs cmd in a new pseudo console; the returned func changes the size of its screen. cmd is not started
// by os/exec (it cannot attach a console), so only cmd.Process is filled in, for the one who stops it.
func startPty(cmd *exec.Cmd, cols, rows int) (io.ReadWriteCloser, func(cols, rows int), error) {
	if windows.NewLazySystemDLL("kernel32.dll").NewProc("CreatePseudoConsole").Find() != nil {
		return nil, nil, errors.New("this Windows has no pseudo console (needs Windows 10 1809 or newer)")
	}
	var inR, inW, outR, outW windows.Handle
	if err := windows.CreatePipe(&inR, &inW, nil, 0); err != nil {
		return nil, nil, err
	}
	if err := windows.CreatePipe(&outR, &outW, nil, 0); err != nil {
		windows.CloseHandle(inR)
		windows.CloseHandle(inW)
		return nil, nil, err
	}
	var pc windows.Handle
	err := windows.CreatePseudoConsole(coord(cols, rows), inR, outW, 0, &pc)
	// the console keeps its own copies of these two ends
	windows.CloseHandle(inR)
	windows.CloseHandle(outW)
	if err != nil {
		windows.CloseHandle(inW)
		windows.CloseHandle(outR)
		return nil, nil, err
	}
	c := &conpty{pc: pc, in: os.NewFile(uintptr(inW), "conpty-in"), out: os.NewFile(uintptr(outR), "conpty-out")}

	al, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		c.Close()
		return nil, nil, err
	}
	defer al.Delete()
	// the value of this attribute is the console handle itself, not a pointer to it
	if err := al.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, *(*unsafe.Pointer)(unsafe.Pointer(&pc)), unsafe.Sizeof(pc)); err != nil {
		c.Close()
		return nil, nil, err
	}
	si := &windows.StartupInfoEx{
		StartupInfo:             windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{}))},
		ProcThreadAttributeList: al.List(),
	}
	line, err := windows.UTF16PtrFromString(commandLine(cmd))
	if err != nil {
		c.Close()
		return nil, nil, err
	}
	var dir *uint16
	if cmd.Dir != "" {
		if dir, err = windows.UTF16PtrFromString(cmd.Dir); err != nil {
			c.Close()
			return nil, nil, err
		}
	}
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	pi := new(windows.ProcessInformation)
	err = windows.CreateProcess(nil, line, nil, nil, false,
		windows.CREATE_UNICODE_ENVIRONMENT|windows.EXTENDED_STARTUPINFO_PRESENT, envBlock(env), dir, &si.StartupInfo, pi)
	if err != nil {
		c.Close()
		return nil, nil, err
	}
	windows.CloseHandle(pi.Thread)
	cmd.Process, _ = os.FindProcess(int(pi.ProcessId))
	// when the program ends, the console is closed: only then the reader sees the end of its output
	go func() {
		_, _ = windows.WaitForSingleObject(pi.Process, windows.INFINITE)
		windows.CloseHandle(pi.Process)
		c.Close()
	}()
	return c, func(cl, r int) { _ = windows.ResizePseudoConsole(c.pc, coord(cl, r)) }, nil
}
