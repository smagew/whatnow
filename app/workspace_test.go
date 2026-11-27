package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaces(t *testing.T) {
	e := isolate(t)
	root, other := filepath.Join(e.home, "code", "shop"), filepath.Join(e.home, "code", "api")
	must(t, os.MkdirAll(root, 0o755))
	must(t, os.MkdirAll(other, 0o755))
	writeFile(t, filepath.Join(e.claude, "skills", "whatnow", "SKILL.md"), []byte("x"))
	writeFile(t, filepath.Join(e.claude, "skills", "frontend-design", "SKILL.md"), []byte("x"))

	code, obj := act(map[string]any{"action": "ws-save", "workspace": map[string]any{"name": "Shop", "root": root, "dirs": []any{other},
		"mode": "acceptEdits", "allow": []any{"Bash(pnpm test:*)"}, "skills": []any{"frontend-design"}, "note": "Tests: pnpm test."}})
	eq(t, code, 200, "save")
	id := jsMap(obj)["workspace"].(map[string]any)["id"].(string)
	l := wsList()
	eq(t, len(l), 1, "workspaces")
	eq(t, strings.Join(wsSkills(), ","), "frontend-design,whatnow", "skills to pick from")

	// the plan of a new session from it: its folders, mode, rules; the context and the skills open the first message
	code, p := wsPlan(id)
	eq(t, code, 200, "plan")
	eq(t, p["root"].(string), root, "root")
	eq(t, len(p["dirs"].([]any)), 1, "dirs")
	eq(t, p["mode"].(string), "acceptEdits", "mode")
	intro := wsIntro(p)
	if !strings.Contains(intro, "Tests: pnpm test.") || !strings.Contains(intro, "frontend-design") {
		t.Errorf("the first message does not carry the context and the skills: %q", intro)
	}

	// what is not right is said
	for _, bad := range []map[string]any{{"name": "", "root": root}, {"name": "x", "root": "/no/such/folder"},
		{"name": "x", "root": root, "dirs": []any{"/no/such"}}, {"name": "x", "root": root, "mode": "nonsense"}} {
		if c, _ := act(map[string]any{"action": "ws-save", "workspace": bad}); c != 400 {
			t.Errorf("saved %v: %d", bad, c)
		}
	}
	// saving again under the same id replaces it; a second one gets its own id
	act(map[string]any{"action": "ws-save", "workspace": map[string]any{"id": id, "name": "Shop 2", "root": root}})
	act(map[string]any{"action": "ws-save", "workspace": map[string]any{"name": "Shop 2", "root": root}})
	l = wsList()
	if len(l) != 2 || l[0]["name"] != "Shop 2" || l[0]["id"] == l[1]["id"] {
		t.Fatalf("after saving again: %v", l)
	}
	act(map[string]any{"action": "ws-delete", "id": id})
	eq(t, len(wsList()), 1, "after delete")
}

// The context actions type into a background session; a session in the user's own terminal gets the command to paste.
func TestContextActions(t *testing.T) {
	e := withDemo(t)
	bg, own := "aaaaaaaa-0000-0000-0000-000000000011", "bbbbbbbb-0000-0000-0000-000000000012"
	e.session(bg, "wait", "demo-feature")
	e.session(own, "wait", "demo-feature")
	e.setAgents(map[string]map[string]any{bg: {"kind": "background", "id": "aaaaaaaa", "pid": 1}, own: {"kind": "interactive", "id": "bbbbbbbb", "pid": 2}})
	typed := make(chan string, 8)
	old := typeCommand
	typeCommand = func(sid string, keys ...string) bool { typed <- sid + ": " + keys[0]; return true }
	t.Cleanup(func() { typeCommand = old })

	code, _ := act(map[string]any{"action": "compact", "session": bg, "text": "keep the plan"})
	eq(t, code, 200, "compact a background session")
	eq(t, <-typed, bg+": /compact keep the plan", "typed")
	code, obj := act(map[string]any{"action": "compact", "session": own, "text": "keep the plan"})
	eq(t, code, 409, "compact a session in your terminal")
	eq(t, jsMap(obj)["command"].(string), "/compact keep the plan", "the command to paste")

	code, _ = act(map[string]any{"action": "clear", "session": bg})
	eq(t, code, 200, "clear")
	eq(t, <-typed, bg+": /clear", "first /clear")
	if next := <-typed; !strings.Contains(next, "feature.json") {
		t.Errorf("after /clear the session is not sent to its page: %q", next)
	}
}
