package main

// Screenshots for the agent. In the window of a session the user takes a picture of a part of the screen (the
// system selection, as Cmd+Shift+4 does); it is kept with the work, shown as a small picture in the window, and
// goes to the session with a message as a file path. Claude Code opens a picture named in a message itself (checked
// on a real session in the default permission mode: it read the file without asking).
//
// Only the app window can take pictures or see them (handler.local): a screenshot can show anything on the screen.

import (
	"errors"
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
		if !shotRe.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, map[string]any{"name": e.Name(), "path": filepath.Join(shotsDir(slug), e.Name()), "t": info.ModTime().UnixMilli(), "size": info.Size()})
	}
	sort.Slice(out, func(i, j int) bool { return num(out[i].(map[string]any)["t"]) > num(out[j].(map[string]any)["t"]) })
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}

// shotFile is the path of one screenshot, "" when the name is not one of ours.
func shotFile(slug, name string) string {
	if !shotRe.MatchString(name) {
		return ""
	}
	p := filepath.Join(shotsDir(slug), name)
	if !isFile(p) {
		return ""
	}
	return p
}
