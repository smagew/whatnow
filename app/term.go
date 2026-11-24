package main

// The terminal of the app window: `claude attach` of a background session runs in a pseudo-terminal here, the
// window shows it with xterm.js. Only the app window can use it (handler.local): a terminal is full access to
// the machine, so it is never served on the 127.0.0.1 port, where any browser tab could reach it.
//
// A session can be shown in the terminal when it runs in the background:
//   - it already does: attach;
//   - it is closed: `claude --resume <id> --bg` continues it in the background under the same id, then attach;
//   - it is open in another terminal: nothing can take it from there. The user types /background in that
//     terminal; Claude Code continues the session under a new id and writes that id into the old transcript
//     ("continued-in"). The link of the session to its feature follows that record.

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	termKeep = 512 * 1024 // bytes of output kept for a window that joins late
	termWait = 20 * time.Second
)

var bgID = regexp.MustCompile(`backgrounded · ([0-9A-Za-z_-]+)`)

type term struct {
	ID, Session, Name string
	cmd               *exec.Cmd
	tty               io.ReadWriteCloser
	resize            func(cols, rows int)
	mu                sync.Mutex
	cond              *sync.Cond
	buf               []byte
	start             int64 // offset of buf[0] in everything the process has written
	done              bool
}

var (
	terms   = map[string]*term{} // by terminal id
	termsMu sync.Mutex
)

func (t *term) pump() {
	b := make([]byte, 32*1024)
	for {
		n, err := t.tty.Read(b)
		t.mu.Lock()
		if n > 0 {
			t.buf = append(t.buf, b[:n]...)
			if over := len(t.buf) - termKeep; over > 0 {
				t.buf, t.start = append([]byte(nil), t.buf[over:]...), t.start+int64(over)
			}
		}
		if err != nil {
			t.done = true
		}
		t.cond.Broadcast()
		t.mu.Unlock()
		if err != nil {
			_ = t.cmd.Wait()
			return
		}
	}
}

func (t *term) end() int64 { return t.start + int64(len(t.buf)) }

func (t *term) info() map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	return map[string]any{"id": t.ID, "session": t.Session, "name": t.Name, "done": t.done, "off": t.end()}
}

func termOf(id string) *term {
	termsMu.Lock()
	defer termsMu.Unlock()
	return terms[id]
}

// termFor is the live terminal of a session, if the window already has one.
func termFor(sid string) *term {
	termsMu.Lock()
	defer termsMu.Unlock()
	for _, t := range terms {
		t.mu.Lock()
		live := !t.done && t.Session == sid
		t.mu.Unlock()
		if live {
			return t
		}
	}
	return nil
}

func termList() []any {
	termsMu.Lock()
	defer termsMu.Unlock()
	out := []any{}
	for id, t := range terms {
		i := t.info()
		if i["done"] == true {
			delete(terms, id) // a finished terminal is reported once, then forgotten
		}
		out = append(out, i)
	}
	return out
}

// continuedIn is the id a session went on under after /background, "" when it was not moved.
// Claude Code appends {"type":"continued-in","continuedInSessionId":...} to the old transcript.
func continuedIn(sid string) string {
	path := findTranscript(sid)
	if path == "" {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > 64*1024 {
		_, _ = f.Seek(-64*1024, io.SeekEnd)
	}
	b, _ := io.ReadAll(f)
	next := ""
	for _, line := range bytes.Split(b, []byte("\n")) {
		if !bytes.Contains(line, []byte(`"continued-in"`)) {
			continue
		}
		if row, _ := parseJSON(line).(map[string]any); row != nil && row["type"] == "continued-in" {
			if s := str(row, "continuedInSessionId"); sidRe.MatchString(s) {
				next = s
			}
		}
	}
	return next
}

// follow moves what the dashboard knows about a session (its feature, its star) to the id it continues under.
func follow(old, next string) {
	if slug, _ := bindings()[old].(string); slug != "" && bindings()[next] == nil {
		_ = bind(next, slug)
	}
	for _, k := range favorites() {
		if k == "s:"+old {
			_ = setFavorite("s:"+next, true)
		}
	}
}

// latest follows the chain of /background moves to the id the session lives under now.
func latest(sid string) string {
	for i := 0; i < 8; i++ {
		next := continuedIn(sid)
		if next == "" || next == sid {
			break
		}
		follow(sid, next)
		sid = next
	}
	return sid
}

// termOpen gives the terminal of a session, starting it when the session can be attached. The answer says
// what is missing when it cannot: {"need":"background"} — the session is open in another terminal.
func termOpen(sid string, cols, rows int) (int, any) {
	if !sidRe.MatchString(sid) {
		return 400, map[string]any{"error": "bad session"}
	}
	if !termSupported {
		return 501, map[string]any{"error": "the terminal is not available on this system yet"}
	}
	sid = latest(sid)
	if t := termFor(sid); t != nil {
		return 200, map[string]any{"term": t.info(), "session": sid}
	}
	claude := claudeBin()
	if claude == "" {
		return 500, map[string]any{"error": "claude not found: set claudeBin in config.json"}
	}
	ag := readAgents() // not the cached list: the session may have moved a second ago
	a := ag[sid]
	cwd := ""
	if s := findSession(sid); s != nil {
		cwd = s.Cwd
	}
	if c := str(a, "cwd"); c != "" {
		cwd = c
	}
	if !isDir(cwd) {
		cwd = userHome()
	}
	target, name := str(a, "id"), str(a, "name")
	switch {
	case a != nil && a["kind"] == "background":
	case a != nil: // open in a terminal of its own
		return 200, map[string]any{"need": "background", "session": sid, "name": name}
	default: // closed: continue it in the background, under the same id
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, claude, "--resume", sid, "--bg")
		cmd.Dir, cmd.Env = cwd, childEnv(claude)
		out, err := cmd.CombinedOutput()
		m := bgID.FindSubmatch(out)
		if m == nil {
			msg := runes(strings.TrimSpace(string(out)), 300)
			if msg == "" && err != nil {
				msg = err.Error()
			}
			return 500, map[string]any{"error": "could not continue the session in the background: " + msg}
		}
		target = string(m[1])
		if a = readAgents()[sid]; a != nil { // its name, as the list of sessions shows it
			name = str(a, "name")
		}
	}
	if target == "" {
		target = sid
	}
	if name == "" {
		name = target
	}
	cmd := exec.Command(claude, "attach", target)
	cmd.Dir = cwd
	cmd.Env = append(childEnv(claude), "TERM=xterm-256color", "COLORTERM=truecolor")
	tty, resize, err := startPty(cmd, cols, rows)
	if err != nil {
		return 500, map[string]any{"error": "could not start the terminal: " + err.Error()}
	}
	t := &term{ID: newID(), Session: sid, Name: name, cmd: cmd, tty: tty, resize: resize}
	t.cond = sync.NewCond(&t.mu)
	termsMu.Lock()
	terms[t.ID] = t
	termsMu.Unlock()
	go t.pump()
	return 200, map[string]any{"term": t.info(), "session": sid}
}

// termRead waits for output after offset off. "reset" says the window missed output that is no longer kept.
func termRead(id string, off int64) (int, any) {
	t := termOf(id)
	if t == nil {
		return 404, map[string]any{"error": "no such terminal"}
	}
	timer := time.AfterFunc(termWait, func() { t.mu.Lock(); t.cond.Broadcast(); t.mu.Unlock() })
	defer timer.Stop()
	deadline := time.Now().Add(termWait)
	t.mu.Lock()
	defer t.mu.Unlock()
	for off >= t.end() && !t.done && time.Now().Before(deadline) {
		t.cond.Wait()
	}
	reset := off < t.start || off > t.end()
	if reset {
		off = t.start
	}
	data := t.buf[off-t.start:]
	return 200, map[string]any{"data": base64.StdEncoding.EncodeToString(data), "off": t.end(), "done": t.done, "reset": reset}
}

func termWrite(id, data string) (int, any) {
	t := termOf(id)
	if t == nil {
		return 404, map[string]any{"error": "no such terminal"}
	}
	if _, err := t.tty.Write([]byte(data)); err != nil {
		return 500, map[string]any{"error": err.Error()}
	}
	return 200, map[string]any{"ok": true}
}

func termResize(id string, cols, rows int) (int, any) {
	t := termOf(id)
	if t == nil {
		return 404, map[string]any{"error": "no such terminal"}
	}
	if cols > 0 && rows > 0 && cols < 1000 && rows < 1000 {
		t.resize(cols, rows)
	}
	return 200, map[string]any{"ok": true}
}

// termClose leaves the session: the attach process ends, the background session keeps running.
func termClose(id string) (int, any) {
	t := termOf(id)
	if t == nil {
		return 200, map[string]any{"ok": true}
	}
	_ = t.tty.Close()
	if t.cmd.Process != nil {
		stopProcess(t.cmd.Process.Pid)
	}
	termsMu.Lock()
	delete(terms, id)
	termsMu.Unlock()
	return 200, map[string]any{"ok": true}
}

// termCloseAll is called when the window closes: no attach process is left behind.
func termCloseAll() {
	termsMu.Lock()
	ids := make([]string, 0, len(terms))
	for id := range terms {
		ids = append(ids, id)
	}
	termsMu.Unlock()
	for _, id := range ids {
		termClose(id)
	}
}
