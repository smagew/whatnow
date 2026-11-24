package main

// Shared test fixtures: an isolated WHATNOW_HOME / FEATURES_DIR / CLAUDE_CONFIG_DIR per test, a fake `claude`
// program (config.json "claudeBin") for `claude agents` and `claude --resume`, a fake --watch process that makes
// a session "awake", and an in-process runner for the hook.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const fakeWatcherEnv = "WHATNOW_TEST_FAKE_WATCHER"

var (
	nodeBin     string // found before PATH is narrowed
	fakeWatcher struct {
		once sync.Once
		cmd  *exec.Cmd
		err  error
	}
)

func TestMain(m *testing.M) {
	if os.Getenv(fakeWatcherEnv) == "1" {
		// A stand-in for a running `whatnow hook --watch`: `ps` shows this command line. Ends with the tests.
		for i := 0; i < 600 && os.Getppid() != 1; i++ {
			time.Sleep(time.Second)
		}
		os.Exit(0)
	}
	nodeBin, _ = exec.LookPath("node")
	tmp, err := os.MkdirTemp("", "whatnow-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// Nothing may reach the real ~/.whatnow, ~/.claude or the real claude program, even a test that forgets isolate().
	fakeHome := filepath.Join(tmp, "user")
	_ = os.MkdirAll(fakeHome, 0o755)
	os.Setenv("HOME", fakeHome)
	os.Setenv("USERPROFILE", fakeHome)
	if runtime.GOOS != "windows" {
		os.Setenv("PATH", strings.Join([]string{"/usr/local/bin", "/usr/bin", "/bin"}, string(os.PathListSeparator)))
	}
	os.Setenv("WHATNOW_HOME", filepath.Join(tmp, "whatnow"))
	os.Setenv("FEATURES_DIR", filepath.Join(tmp, "whatnow", "features"))
	os.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(tmp, "claude"))
	code := m.Run()
	if fakeWatcher.cmd != nil && fakeWatcher.cmd.Process != nil {
		_ = fakeWatcher.cmd.Process.Kill()
		_ = fakeWatcher.cmd.Wait()
	}
	os.RemoveAll(tmp)
	os.Exit(code)
}

type testEnv struct {
	t                     *testing.T
	home, feats, claude   string
	fake, fakeBin, config string
}

// isolate gives the test its own folders and a fake claude that lists no open session.
func isolate(t *testing.T) *testEnv {
	t.Helper()
	base := t.TempDir()
	e := &testEnv{t: t, home: filepath.Join(base, "whatnow"), claude: filepath.Join(base, "claude"), fake: filepath.Join(base, "fake")}
	e.feats = filepath.Join(e.home, "features")
	e.config = filepath.Join(e.home, "config.json")
	for _, d := range []string{e.feats, e.claude, e.fake} {
		must(t, os.MkdirAll(d, 0o755))
	}
	t.Setenv("WHATNOW_HOME", e.home)
	t.Setenv("FEATURES_DIR", e.feats)
	t.Setenv("CLAUDE_CONFIG_DIR", e.claude)
	resetCaches()
	t.Cleanup(resetCaches)
	if runtime.GOOS != "windows" {
		e.installFakeClaude()
	}
	return e
}

func resetCaches() {
	agents.mu.Lock()
	agents.v, agents.at = nil, time.Time{}
	agents.mu.Unlock()
	tcacheMu.Lock()
	tcache = map[string]cacheItem{}
	tcacheMu.Unlock()
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Dir(path), 0o755))
	must(t, os.WriteFile(path, data, 0o644))
}

func readText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	must(t, err)
	return string(b)
}

// js turns Go literals into what parseJSON gives for the same JSON (json.Number, []any, map[string]any).
func js(v any) any { return parseJSON(encodeJSON(v)) }

func jsMap(v any) map[string]any { return js(v).(map[string]any) }

// writeConfig merges keys into config.json, keeping the fake claude.
func (e *testEnv) writeConfig(kv map[string]any) {
	c := readMap(e.config)
	for k, v := range kv {
		c[k] = v
	}
	writeFile(e.t, e.config, encodeJSON(c))
}

// ------------------------------------------------------------------ fake claude

func (e *testEnv) installFakeClaude() {
	e.fakeBin = filepath.Join(e.fake, "claude")
	script := fmt.Sprintf(`#!/bin/sh
d='%s'
printf '%%s\n' "$1 $2" >> "$d/calls"
case "$1" in
agents) sleep "$(cat "$d/sleep")"; cat "$d/agents.json" ;;
--resume) printf '%%s\n' "$2" >> "$d/resumed"; cat "$d/resume.out"; exit "$(cat "$d/resume.code")" ;;
esac
`, e.fake)
	writeFile(e.t, e.fakeBin, []byte(script))
	must(e.t, os.Chmod(e.fakeBin, 0o755))
	e.fakeSet("sleep", "0")
	e.fakeSet("agents.json", "[]")
	e.fakeSet("resume.out", "started bg")
	e.fakeSet("resume.code", "0")
	e.writeConfig(map[string]any{"claudeBin": e.fakeBin})
}

func (e *testEnv) fakeSet(name, content string) {
	writeFile(e.t, filepath.Join(e.fake, name), []byte(content))
}

// fakeLines is what the fake claude logged: "calls" (every run) or "resumed" (session ids).
func (e *testEnv) fakeLines(name string) []string {
	b, err := os.ReadFile(filepath.Join(e.fake, name))
	if err != nil {
		return nil
	}
	return strings.Fields(strings.TrimSpace(string(b)))
}

func (e *testEnv) fakeCalls() int {
	b, err := os.ReadFile(filepath.Join(e.fake, "calls"))
	if err != nil {
		return 0
	}
	return strings.Count(string(b), "\n")
}

// setAgents is what `claude agents --json` lists, keyed by session id; the cache is filled as if just read.
func (e *testEnv) setAgents(m map[string]map[string]any) {
	l := []any{}
	for id, a := range m {
		x := map[string]any{"sessionId": id}
		for k, v := range a {
			x[k] = v
		}
		l = append(l, x)
	}
	e.fakeSet("agents.json", string(encodeJSON(l)))
	v := agentMap{}
	for _, x := range js(l).([]any) {
		a := x.(map[string]any)
		v[a["sessionId"].(string)] = a
	}
	agents.mu.Lock()
	agents.v, agents.at = v, time.Now()
	agents.mu.Unlock()
}

// setAgentsAt says when the list of open sessions was read.
func setAgentsAt(at time.Time) {
	agents.mu.Lock()
	agents.at = at
	agents.mu.Unlock()
}

// ------------------------------------------------------------------ fake watcher

// wake makes a session awake as if its --watch hook ran: its pid file points at a process whose command line
// is "... whatnow hook --watch".
func (e *testEnv) wake(sid string) {
	e.t.Helper()
	if runtime.GOOS == "windows" {
		e.t.Skip("the fake watcher relies on ps")
	}
	fakeWatcher.once.Do(func() {
		c := exec.Command(os.Args[0], "whatnow hook", "--watch")
		c.Env = append(os.Environ(), fakeWatcherEnv+"=1")
		fakeWatcher.err = c.Start()
		fakeWatcher.cmd = c
	})
	must(e.t, fakeWatcher.err)
	writeFile(e.t, filepath.Join(e.home, "watch", sid+".pid"), []byte(fmt.Sprint(fakeWatcher.cmd.Process.Pid)))
	if !watcherAlive(sid) {
		e.t.Fatal("the fake watcher is not recognised as a watcher")
	}
}

func (e *testEnv) unwake(sid string) { os.Remove(filepath.Join(e.home, "watch", sid+".pid")) }

// ------------------------------------------------------------------ the demo feature and sessions

func demoFeature() map[string]any {
	return map[string]any{
		"title": "Demo feature", "projects": []any{"api", "web"}, "branches": []any{},
		"ladder": map[string]any{"api": []any{"ok", "ok", "no", "no", "no", "no"}, "web": []any{"ok", "no", "no", "no", "no", "no"}},
		"stages": []any{map[string]any{"n": 1, "title": "Design", "verdict": "ok", "plan": []any{"x"},
			"facts": []any{[]any{"y", "Schema written"}, []any{"q", "Not done: check against the real database"}}}},
	}
}

func (e *testEnv) demoSrc() string { return filepath.Join(e.feats, "demo-feature", "feature.json") }

// withDemo is the Env of test_inbox.py: the demo feature in the features folder.
func withDemo(t *testing.T) *testEnv {
	e := isolate(t)
	writeFile(t, e.demoSrc(), encodeJSON(demoFeature()))
	return e
}

func (e *testEnv) loadDemo() map[string]any {
	e.t.Helper()
	d, err := load(e.demoSrc())
	must(e.t, err)
	return d
}

func (e *testEnv) fid(idx int) string {
	return e.loadDemo()["stages"].([]any)[0].(map[string]any)["fids"].([]any)[idx].(string)
}

// session writes a hook file for a session and links it to a feature ("" for none).
func (e *testEnv) session(sid, state, feature string) {
	e.sessionAt(sid, state, feature, nowMs())
}

func (e *testEnv) sessionAt(sid, state, feature string, t int64) {
	writeFile(e.t, filepath.Join(e.home, "sessions", sid+".json"), encodeJSON(map[string]any{
		"session": sid, "cwd": e.home, "project": "api", "branch": "main", "state": state, "what": "", "t": t}))
	if feature != "" {
		must(e.t, bind(sid, feature))
	}
}

func (e *testEnv) saveEntry(m map[string]any) {
	must(e.t, saveEntry(jsMap(m)))
}

// ------------------------------------------------------------------ the hook, in process

// hook runs runHook with payload on stdin (a string is sent as is) and returns the exit code and the output.
func (e *testEnv) hook(args []string, payload any) (int, string, string) {
	e.t.Helper()
	in, ok := payload.(string)
	if !ok {
		in = string(encodeJSON(payload))
	}
	dir := e.t.TempDir()
	open := func(name string) *os.File {
		f, err := os.OpenFile(filepath.Join(dir, name), os.O_RDWR|os.O_CREATE, 0o644)
		must(e.t, err)
		return f
	}
	writeFile(e.t, filepath.Join(dir, "in"), []byte(in))
	fin, fout, ferr := open("in"), open("out"), open("err")
	hookIO.Lock()
	oldIn, oldOut, oldErr := os.Stdin, os.Stdout, os.Stderr
	os.Stdin, os.Stdout, os.Stderr = fin, fout, ferr
	rc := runHook(args)
	os.Stdin, os.Stdout, os.Stderr = oldIn, oldOut, oldErr
	hookIO.Unlock()
	for _, f := range []*os.File{fin, fout, ferr} {
		f.Close()
	}
	return rc, readText(e.t, filepath.Join(dir, "out")), readText(e.t, filepath.Join(dir, "err"))
}

var hookIO sync.Mutex

func gitInit(t *testing.T, dir string) {
	t.Helper()
	must(t, os.MkdirAll(dir, 0o755))
	if out, err := exec.Command("git", "init", "-q", "-b", "feature/demo-feature", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
}

func entryOf(t *testing.T, obj any) map[string]any {
	t.Helper()
	m, _ := obj.(map[string]any)
	e, ok := m["entry"].(map[string]any)
	if !ok {
		t.Fatalf("no entry in %v", obj)
	}
	return e
}

func ids(l []map[string]any) []string {
	out := []string{}
	for _, e := range l {
		out = append(out, fmt.Sprint(e["id"]))
	}
	return out
}

func eq[T comparable](t *testing.T, got, want T, what ...any) {
	t.Helper()
	if got != want {
		t.Errorf("%v: got %#v, want %#v", fmt.Sprint(what...), got, want)
	}
}

func eqList[T comparable](t *testing.T, got, want []T, what ...any) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%v: got %#v, want %#v", fmt.Sprint(what...), got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("%v: got %#v, want %#v", fmt.Sprint(what...), got, want)
			return
		}
	}
}

func contains(t *testing.T, s, sub string, what ...any) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("%v: %q does not contain %q", fmt.Sprint(what...), s, sub)
	}
}
