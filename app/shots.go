package main

// Screenshots for the agent. In the window of a session the user takes a picture of a part of the screen (the
// system selection, as Cmd+Shift+4 does); it is kept with the work, shown as a small picture in the window, and
// goes to the session with a message as a file path. Claude Code opens a picture named in a message itself (checked
// on a real session in the default permission mode: it read the file without asking).
//
// Only the app window can take pictures or see them (handler.local): a screenshot can show anything on the screen.

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

var shotRe = regexp.MustCompile(`^shot-[0-9]{8}-[0-9]{6}(-[0-9]+)?\.png$`)

func shotsDir(slug string) string {
	if !slugRe.MatchString(slug) {
		slug = "_sessions"
	}
	return filepath.Join(home(), "shots", slug)
}

// takeShot shows the system selection and waits for it; "" (no error) when the user pressed Esc.
func takeShot(slug string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", errors.New("taking a screenshot from the window works on macOS only, for now")
	}
	dir := shotsDir(slug)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := "shot-" + time.Now().Format("20060102-150405") + ".png"
	for i := 2; isFile(filepath.Join(dir, name)); i++ {
		name = strings.TrimSuffix(name, ".png")
		if j := strings.LastIndex(name, "-"); j > len("shot-20060102-150405")-1 {
			name = name[:j]
		}
		name = name + "-" + strconv.Itoa(i) + ".png"
	}
	path := filepath.Join(dir, name)
	// -i the selection (Space switches to picking a window), -x no sound, -o no window shadow
	out, err := exec.Command("/usr/sbin/screencapture", "-i", "-x", "-o", path).CombinedOutput()
	if !isFile(path) {
		if err != nil && len(out) > 0 {
			return "", errors.New(strings.TrimSpace(string(out)))
		}
		return "", nil
	}
	return name, nil
}

// listShots are the screenshots of a piece of work, newest first.
func listShots(slug string) []any {
	l, _ := os.ReadDir(shotsDir(slug))
	out := []any{}
	for _, e := range l {
		if !shotRe.MatchString(e.Name()) && !fileRe.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		kind, shown := "shot", e.Name()
		if fileRe.MatchString(e.Name()) {
			kind, shown = "file", e.Name()[len("file-20060102-150405-"):]
		}
		out = append(out, map[string]any{"name": e.Name(), "path": filepath.Join(shotsDir(slug), e.Name()), "t": info.ModTime().UnixMilli(), "size": info.Size(),
			"kind": kind, "shown": shown, "image": isImageName(e.Name())})
	}
	sort.Slice(out, func(i, j int) bool { return num(out[i].(map[string]any)["t"]) > num(out[j].(map[string]any)["t"]) })
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}

// shotFile is the path of one screenshot, "" when the name is not one of ours.
func shotFile(slug, name string) string {
	if !shotRe.MatchString(name) && !fileRe.MatchString(name) {
		return ""
	}
	p := filepath.Join(shotsDir(slug), name)
	if !isFile(p) {
		return ""
	}
	return p
}

// Files attached to a message: any file, kept with the work next to its screenshots, sent as a path (as a
// screenshot is). Their name keeps the original one after the time, so the session and the user see what it is.
var fileRe = regexp.MustCompile(`^file-[0-9]{8}-[0-9]{6}-[A-Za-z0-9._-]{1,100}$`)

const maxAttach = 50 << 20 // 50 MB a file

var attachUnsafe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// attachName is a free name for an attached file, from its original name.
func attachName(dir, orig string) string {
	base := strings.Trim(attachUnsafe.ReplaceAllString(filepath.Base(orig), "_"), "._")
	if base == "" {
		base = "file"
	}
	if r := []rune(base); len(r) > 80 {
		ext := filepath.Ext(base)
		if len(ext) > 12 {
			ext = ""
		}
		base = string(r[:80-len(ext)]) + ext
	}
	stamp := time.Now().Format("20060102-150405")
	name := "file-" + stamp + "-" + base
	for i := 2; isFile(filepath.Join(dir, name)); i++ {
		name = "file-" + stamp + "-" + strconv.Itoa(i) + "_" + base
	}
	return name
}

// saveAttach keeps a file for a work; its name in the list, or an error (too big, not writable).
func saveAttach(slug, orig string, r io.Reader) (string, error) {
	dir := shotsDir(slug)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := attachName(dir, orig)
	p := filepath.Join(dir, name)
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	n, err := io.Copy(f, io.LimitReader(r, maxAttach+1))
	f.Close()
	if err == nil && n > maxAttach {
		err = errors.New("the file is larger than 50 MB")
	}
	if err != nil {
		os.Remove(p)
		return "", err
	}
	return name, nil
}

// attachPaths copies files picked in the system dialog (the app window).
func attachPaths(slug string, paths []string) ([]string, error) {
	out := []string{}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return out, err
		}
		st, err := f.Stat()
		if err == nil && st.IsDir() {
			err = errors.New(filepath.Base(p) + " is a folder")
		}
		var name string
		if err == nil {
			name, err = saveAttach(slug, p, f)
		}
		f.Close()
		if err != nil {
			return out, errors.New(filepath.Base(p) + ": " + err.Error())
		}
		out = append(out, name)
	}
	return out, nil
}

// pickFiles is the system dialog for choosing files (the app window has one).
var pickFiles func() ([]string, error)

func isImageName(n string) bool {
	switch strings.ToLower(filepath.Ext(n)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return true
	}
	return false
}
