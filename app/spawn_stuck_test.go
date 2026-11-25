package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A session started from the dashboard before it opened the page itself sat under "Starting" for good. It now
// gets its page the next time the bar is drawn, and leaves "Starting".
func TestAStuckStartingSessionGetsItsPage(t *testing.T) {
	e := withDemo(t)
	root := filepath.Join(e.home, "spy")
	must(t, os.MkdirAll(root, 0o755))
	e.setAgents(map[string]map[string]any{"5555aaaa-0000": {"id": "5555aaaa", "kind": "background", "name": "x", "cwd": root, "status": "waiting"}})
	must(t, atomicWriteJSON(spawnedPath(), []any{map[string]any{"id": "5555aaaa", "session": "5555aaaa-0000", "name": "Check the migration",
		"task": "Check how hard the migration to Go would be", "root": root, "ts": nowMs() - 600000}}))

	eq(t, len(spawnedNow()), 0, "it has a page now, so it is not under Starting")
	slug, _ := bindings()["5555aaaa-0000"].(string)
	if slug == "" {
		t.Fatal("the session was not linked to a page")
	}
	if _, err := panelHTML(filepath.Join(e.feats, slug, "feature.json"), "/", true); err != nil {
		t.Fatalf("its page is not valid: %v", err)
	}
	eq(t, readSpawned()[0]["feature"], any(slug), "kept, so it is done once")
	eq(t, len(spawnedNow()), 0)
	l, _ := os.ReadDir(e.feats)
	n := len(l)
	spawnedNow()
	l, _ = os.ReadDir(e.feats)
	eq(t, len(l), n, "no second page")
}
