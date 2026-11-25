package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A new feature with the access of an existing one: a session is started in the folder the feature was started
// in, with its added folders and its permission mode, and with the task of the user.
func TestANewFeatureGetsTheAccessOfTheOldOne(t *testing.T) {
	e := withDemo(t)
	e.setAgents(map[string]map[string]any{"77777777-aaaa": {"name": "api-xx", "cwd": e.home, "status": "idle", "startedAt": 5}})
	root := filepath.Join(e.home, "repo")
	extra := filepath.Join(e.home, "other")
	inside := filepath.Join(root, "sub")
	for _, d := range []string{root, extra, inside} {
		must(t, os.MkdirAll(d, 0o755))
	}
	e.session("77777777-aaaa", "idle", "demo-feature")
	e.appendTranscript(
		map[string]any{"cwd": root, "message": map[string]any{"role": "user", "content": "start"}},
		map[string]any{"type": "permission-mode", "permissionMode": "acceptEdits"},
		map[string]any{"type": "attachment", "attachment": map[string]any{"type": "environment", "snapshot": map[string]any{
			"workingDirectory": root, "additionalWorkingDirectories": []any{extra, inside, filepath.Join(e.home, "gone")}}}})

	code, p := spawnPlan("demo-feature")
	eq(t, code, 200, p)
	eq(t, p["root"], any(root))
	eq(t, p["mode"], any("acceptEdits"))
	// a folder inside the working folder and a folder that is gone are not passed on
	eqList(t, anyStrings(p["dirs"]), []string{extra})
	eq(t, p["git"], any(false))

	code, r := act2(t, map[string]any{"action": "spawn", "feature": "demo-feature", "text": "Add a CSV export", "place": "worktree"})
	eq(t, code, 400, "a separate copy needs a git repository", r)

	code, r = act2(t, map[string]any{"action": "spawn", "feature": "demo-feature", "text": "Add a CSV export to the report page", "place": "same"})
	eq(t, code, 200, r)
	eq(t, r["id"], any("ab12cd34"))
	args := strings.Split(strings.TrimSpace(string(mustRead(t, filepath.Join(e.fake, "spawn.args")))), "\n")
	joined := strings.Join(args, "|")
	for _, want := range []string{"--bg|-n|Add a CSV export to the report page", "--add-dir|" + extra, "--permission-mode|acceptEdits"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the session was started without %q: %v", want, args[:len(args)-1])
		}
	}
	if strings.Contains(joined, "--worktree") || strings.Contains(joined, inside) {
		t.Errorf("unexpected arguments: %v", args)
	}
	if last := args[0]; last != "--bg" {
		t.Errorf("first argument %q", last)
	}
	contains(t, joined, "Add a CSV export to the report page", "the task is the prompt")
	contains(t, joined, "feature.json", "the session is asked to make its feature")
	cwd := strings.TrimSpace(string(mustRead(t, filepath.Join(e.fake, "spawn.cwd"))))
	if a, _ := filepath.EvalSymlinks(cwd); a != mustEval(t, root) {
		t.Errorf("started in %s, want %s", cwd, root)
	}

	// until the session has made its feature, the bar shows it as starting
	st := featuresList()["starting"].([]any)
	eq(t, len(st), 1)
	eq(t, st[0].(map[string]any)["name"], any("Add a CSV export to the report page"))

	code, _ = spawnPlan("no-such-feature")
	eq(t, code, 404)
}

func anyStrings(v any) []string {
	l, _ := v.([]any)
	out := make([]string, len(l))
	for i, x := range l {
		out[i], _ = x.(string)
	}
	return out
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	must(t, err)
	return b
}

func mustEval(t *testing.T, path string) string {
	t.Helper()
	p, err := filepath.EvalSymlinks(path)
	must(t, err)
	return p
}
