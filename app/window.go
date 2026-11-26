package main

// A work in a window of its own (for another monitor): `whatnow window <work>` is a second process of the app that
// shows only the page of that work. It has its own window (and its own icon in the Dock on macOS), reads the same
// files as the main window, and its terminal and chat work the same way (both run in the window's own process,
// never on the port).
//
// The windows talk through small files in ~/.whatnow/windows/:
//   - <work>.json: the window of that work is open (written every 2 s; older than 6 s means it is gone);
//   - <work>.focus: the main window asks it to come to the front;
//   - <work>.back: the main window asks it to close and give the work back;
//   - main.json: the main window is open (same rule); main.goto: the work to show there now.
// Where each separate window stood and how big it was is kept in windows/places.json, per work.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	soloSlug string          // the work of this process when it is a separate window, "" in the main window
	winCtx   context.Context // the window of this process, once it exists
)

const winBeat = 2 * time.Second

// winSlug: the name of a work, never "." or ".." or a hidden name (it becomes a path and a page address)
func winSlug(s string) bool { return slugRe.MatchString(s) && !strings.HasPrefix(s, ".") }

func winDir() string { return filepath.Join(home(), "windows") }

func winFile(name string) string { return filepath.Join(winDir(), name) }

// fresh: a heartbeat file written in the last three beats
func fresh(path string) bool {
	st, err := os.Stat(path)
	return err == nil && time.Since(st.ModTime()) < 3*winBeat
}

// soloOpen says whether the work is open in a separate window now.
func soloOpen(slug string) bool { return winSlug(slug) && fresh(winFile(slug+".json")) }

// detachedSet: the works open in separate windows, for the bar of the main window.
func detachedSet() map[string]bool {
	out := map[string]bool{}
	es, _ := os.ReadDir(winDir())
	for _, e := range es {
		n := e.Name()
		if s, ok := strings.CutSuffix(n, ".json"); ok && s != "main" && s != "places" && soloOpen(s) {
			out[s] = true
		}
	}
	return out
}

func touchFile(path string) {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(time.Now().Format(time.RFC3339)), 0o644)
}

// taken: a request file that was there, removed so that it is acted on once
func taken(path string) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	_ = os.Remove(path)
	return strings.TrimSpace(string(b)), true
}

// detach opens the work in a window of its own, or brings that window to the front when it is open already.
// startSolo is the start of the other process; the tests replace it.
var startSolo = func(slug string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "window", slug)
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func detach(slug string) (int, any) {
	if !winSlug(slug) || !isDir(filepath.Join(featuresDir(), slug)) {
		return 400, map[string]any{"error": "bad work"}
	}
	if soloOpen(slug) {
		touchFile(winFile(slug + ".focus"))
		return 200, map[string]any{"ok": true, "focused": true}
	}
	if err := startSolo(slug); err != nil {
		return 500, map[string]any{"error": "could not open the window: " + err.Error()}
	}
	touchFile(winFile(slug + ".json")) // counts as open at once; the window keeps it fresh from now on
	return 200, map[string]any{"ok": true}
}

// reattach: from the main window, the separate window of the work is asked to close; the work is shown here.
func reattach(slug string) (int, any) {
	if !winSlug(slug) {
		return 400, map[string]any{"error": "bad work"}
	}
	if soloOpen(slug) {
		touchFile(winFile(slug + ".back"))
	}
	return 200, map[string]any{"ok": true}
}

// backToMain: in a separate window, "Back to the main window": the main window shows the work (it is started when
// it is not open), and this window closes.
var startMain = func() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe)
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

var quitWindow = func() {
	if winCtx != nil {
		runtime.Quit(winCtx)
	}
}

func backToMain(slug string) (int, any) {
	if !winSlug(slug) {
		return 400, map[string]any{"error": "bad work"}
	}
	_ = os.MkdirAll(winDir(), 0o755)
	_ = os.WriteFile(winFile("main.goto"), []byte(slug), 0o644)
	if !fresh(winFile("main.json")) {
		if err := startMain(); err != nil {
			return 500, map[string]any{"error": "could not open the main window: " + err.Error()}
		}
	}
	_ = os.Remove(winFile(slug + ".json"))
	go func() { time.Sleep(150 * time.Millisecond); quitWindow() }()
	return 200, map[string]any{"ok": true}
}

// front brings the window of this process to the front, also when another app is active
func front(ctx context.Context) {
	runtime.WindowUnminimise(ctx)
	runtime.WindowShow(ctx)
	appActivate()
}

// winWatch runs in every window process: it keeps the heartbeat of this window and acts on what the other windows
// ask. It ends with the process.
func winWatch(ctx context.Context) {
	beat := winFile("main.json")
	if soloSlug != "" {
		beat = winFile(soloSlug + ".json")
	}
	last := time.Time{}
	for {
		if time.Since(last) >= winBeat {
			touchFile(beat)
			last = time.Now()
		}
		if soloSlug == "" {
			if slug, ok := taken(winFile("main.goto")); ok && winSlug(slug) {
				front(ctx)
				runtime.WindowExecJS(ctx, "location.href='/f/"+slug+"/'") // a slug is [a-z0-9-] only
			}
		} else {
			if _, ok := taken(winFile(soloSlug + ".focus")); ok {
				front(ctx)
			}
			if _, ok := taken(winFile(soloSlug + ".back")); ok {
				backToMain(soloSlug)
			}
		}
		time.Sleep(400 * time.Millisecond)
	}
}

// winGone: the window of this process closes; the others stop counting it as open.
func winGone() {
	if soloSlug != "" {
		_ = os.Remove(winFile(soloSlug + ".json"))
	} else {
		_ = os.Remove(winFile("main.json"))
	}
}

// The place and the size of each separate window, per work.
var placesMu sync.Mutex

func placeOf(slug string) []int {
	placesMu.Lock()
	defer placesMu.Unlock()
	m, _ := readJSON(winFile("places.json")).(map[string]any)
	l, _ := m[slug].([]any)
	if len(l) != 4 {
		return nil
	}
	out := make([]int, 4)
	for i, v := range l {
		out[i] = int(num(v))
	}
	if out[2] < 420 || out[3] < 360 {
		return nil
	}
	return out
}

func placeSave(slug string, x, y, w, h int) {
	placesMu.Lock()
	defer placesMu.Unlock()
	m, _ := readJSON(winFile("places.json")).(map[string]any)
	if m == nil {
		m = map[string]any{}
	}
	m[slug] = []int{x, y, w, h}
	_ = os.MkdirAll(winDir(), 0o755)
	_ = atomicWriteJSON(winFile("places.json"), m)
}

// workTitle: the name of a work, for the title of its window
func workTitle(slug string) string {
	m, _ := readJSON(filepath.Join(featuresDir(), slug, "feature.json")).(map[string]any)
	if t, _ := m["title"].(string); t != "" {
		return t
	}
	return slug
}
