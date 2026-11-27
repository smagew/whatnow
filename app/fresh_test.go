package main

import (
	"path/filepath"
	"testing"
	"time"
)

// "Continue in a fresh session": one new session, the old one lets the work go (the window of the work shows the new
// one, messages no longer wake the old one), and a second press while it starts does not start a second session.
func TestFreshSessionTakesOverTheWork(t *testing.T) {
	e := withDemo(t)
	old, nsid := "aaaaaaaa-0000-0000-0000-000000000021", "cccccccc-0000-0000-0000-000000000023"
	e.session(old, "wait", "demo-feature")
	e.setAgents(map[string]map[string]any{old: {"kind": "background", "id": "aaaaaaaa", "pid": 1}})
	row := map[string]any{"type": "user", "cwd": e.home, "message": map[string]any{"role": "user", "content": "start"}}
	writeFile(t, filepath.Join(e.claude, "projects", "-x", old+".jsonl"), append(encodeJSON(row), '\n'))
	starts, stops := 0, 0
	s0, b0 := startFresh, stopBackground
	startFresh = func(p map[string]any, slug string) (string, error) { starts++; return nsid, nil }
	stopBackground = func(string) error { stops++; return nil }
	t.Cleanup(func() {
		startFresh, stopBackground = s0, b0
		freshMu.Lock()
		freshAt = map[string]time.Time{}
		freshMu.Unlock()
	})

	code, obj := act(map[string]any{"action": "fresh", "session": old})
	eq(t, code, 200, "fresh")
	eq(t, jsMap(obj)["session"].(string), nsid, "the new session")
	b := bindings()
	eq(t, b[nsid], any("demo-feature"), "the new session has the work")
	if _, still := b[old]; still {
		t.Error("the old session still has the work: the window keeps showing it")
	}
	eq(t, stops, 1, "the old background session is stopped")
	// a second press while it starts: no second session
	_ = bind(old, "demo-feature")
	code, _ = act(map[string]any{"action": "fresh", "session": old})
	eq(t, code, 409, "a second press")
	eq(t, starts, 1, "sessions started")
}
