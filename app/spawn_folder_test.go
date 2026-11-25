package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// New work in a folder the user names: the permission mode is the chosen one; without a task the session starts
// idle and the dashboard opens an empty page for it, linked to the session.
func TestNewWorkInAFolder(t *testing.T) {
	e := withDemo(t)
	dir := filepath.Join(e.home, "My Project")
	must(t, os.MkdirAll(dir, 0o755))
	e.setAgents(map[string]map[string]any{"99999999-new1": {"id": "ab12cd34", "name": "My Project", "cwd": dir, "status": "idle", "startedAt": 5}})

	code, r := act2(t, map[string]any{"action": "spawn", "dir": filepath.Join(e.home, "nope"), "text": "x"})
	eq(t, code, 404, r)
	code, r = act2(t, map[string]any{"action": "spawn", "dir": "relative/path", "text": "x"})
	eq(t, code, 400, r)

	code, r = act2(t, map[string]any{"action": "spawn", "dir": dir, "mode": "acceptEdits", "text": "", "place": "same"})
	eq(t, code, 200, r)
	args := strings.Split(strings.TrimSpace(string(mustRead(t, filepath.Join(e.fake, "spawn.args")))), "\n")
	eqList(t, args, []string{"--bg", "-n", "My Project", "--permission-mode", "acceptEdits"}, "no prompt when there is no task")
	if a, _ := filepath.EvalSymlinks(strings.TrimSpace(string(mustRead(t, filepath.Join(e.fake, "spawn.cwd"))))); a != mustEval(t, dir) {
		t.Errorf("started in %s", a)
	}
	slug, _ := r["feature"].(string)
	eq(t, slug, "my-project")
	if _, err := panelHTML(filepath.Join(e.feats, slug, "feature.json"), "/", true); err != nil {
		t.Fatalf("the empty page is not a valid one: %v", err)
	}
	eq(t, bindings()["99999999-new1"], any(slug), "the session is linked to the page")
	eq(t, len(featuresList()["starting"].([]any)), 0, "it has its page already")

	// the same folder again: another page, not the same one
	code, r = act2(t, map[string]any{"action": "spawn", "dir": dir, "text": ""})
	eq(t, code, 200, r)
	eq(t, r["feature"], any("my-project-2"))

	// with a task the session makes its page itself
	code, r = act2(t, map[string]any{"action": "spawn", "dir": dir, "text": "Write the importer"})
	eq(t, code, 200, r)
	eq(t, r["feature"], any(""))
	joined := strings.Join(strings.Split(strings.TrimSpace(string(mustRead(t, filepath.Join(e.fake, "spawn.args")))), "\n"), "|")
	contains(t, joined, "Write the importer", "the task")
	if strings.Contains(joined, "--permission-mode") {
		t.Errorf("no mode was chosen, yet one was passed: %s", joined)
	}

	// a folder Claude Code does not trust yet: said plainly, nothing is answered for the user
	e.fakeSet("spawn.out", "Workspace not trusted. Run `claude` in "+dir+" once and accept the trust prompt, then retry.")
	code, r = act2(t, map[string]any{"action": "spawn", "dir": dir, "text": "x"})
	eq(t, code, 409, r)
	contains(t, fmt.Sprint(r["error"]), "accept its question", "what to do")
	if r["trust"] == nil {
		t.Error("the answer does not name the folder to trust")
	}
}
