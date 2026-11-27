package main

// Workspaces: what a new session starts with, kept to be picked in "New work" instead of set up each time. Its
// folders (the main one and the others it reaches, --add-dir), its permission mode, the rules it may follow without
// asking (--allowedTools), the skills to use and a few lines of context, both given in its first message.
// Kept in ~/.whatnow/workspaces.json.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var wsMu sync.Mutex

func wsFile() string { return filepath.Join(home(), "workspaces.json") }

func wsList() []map[string]any {
	wsMu.Lock()
	defer wsMu.Unlock()
	return wsRead()
}

func wsRead() []map[string]any {
	l, _ := readJSON(wsFile()).([]any)
	out := []map[string]any{}
	for _, x := range l {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func wsStrs(v any, max int) []any {
	out := []any{}
	l, _ := v.([]any)
	for _, x := range l {
		if s, ok := x.(string); ok && strings.TrimSpace(s) != "" && len(out) < max {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

var wsIDRe = regexp.MustCompile(`^[a-z0-9-]{1,60}$`)

// the permission modes of Claude Code
var wsModes = map[string]bool{"default": true, "acceptEdits": true, "plan": true, "auto": true, "dontAsk": true, "bypassPermissions": true}

// wsCheck makes a workspace out of what the page sends, or says what is wrong with it.
func wsCheck(b map[string]any) (map[string]any, string) {
	name := strings.TrimSpace(str(b, "name"))
	if name == "" || len([]rune(name)) > 80 {
		return nil, "give the workspace a name (80 characters at most)"
	}
	root := filepath.Clean(expandUser(strings.TrimSpace(str(b, "root"))))
	if !filepath.IsAbs(root) || !isDir(root) {
		return nil, "the main folder is not there: " + str(b, "root")
	}
	dirs := []any{}
	for _, d := range wsStrs(b["dirs"], 30) {
		p := filepath.Clean(expandUser(d.(string)))
		if !filepath.IsAbs(p) || !isDir(p) {
			return nil, "a folder is not there: " + d.(string)
		}
		dirs = append(dirs, tilde(p))
	}
	mode := str(b, "mode")
	if mode != "" && !wsModes[mode] {
		return nil, "bad permission mode"
	}
	allow := wsStrs(b["allow"], 50)
	for _, a := range allow {
		if len(a.(string)) > 300 || strings.ContainsAny(a.(string), "\n\r") {
			return nil, "a rule is too long or has a line break"
		}
	}
	note := strings.TrimSpace(str(b, "note"))
	if len([]rune(note)) > 3000 {
		return nil, "the context is too long (3000 characters at most)"
	}
	id := str(b, "id")
	if !wsIDRe.MatchString(id) {
		id = workSlug(name) // a fresh id from the name; unique among the workspaces below
	}
	return map[string]any{"id": id, "name": name, "root": tilde(root), "dirs": dirs, "mode": mode, "allow": allow,
		"skills": wsStrs(b["skills"], 40), "note": note}, ""
}

func wsSave(b map[string]any) (int, any) {
	w, bad := wsCheck(b)
	if bad != "" {
		return 400, map[string]any{"error": bad}
	}
	wsMu.Lock()
	defer wsMu.Unlock()
	l := wsRead()
	found := false
	for i, x := range l {
		if x["id"] == w["id"] {
			l[i], found = w, true
		}
	}
	if !found {
		base, n := w["id"].(string), 2
		for wsHas(l, w["id"].(string)) {
			w["id"] = base + "-" + strconv.Itoa(n)
			n++
		}
		l = append(l, w)
	}
	if err := atomicWriteJSON(wsFile(), l); err != nil {
		return 500, map[string]any{"error": err.Error()}
	}
	return 200, map[string]any{"ok": true, "workspace": w}
}

func wsHas(l []map[string]any, id string) bool {
	for _, x := range l {
		if x["id"] == id {
			return true
		}
	}
	return false
}

func wsDelete(id string) (int, any) {
	wsMu.Lock()
	defer wsMu.Unlock()
	l, out := wsRead(), []map[string]any{}
	for _, x := range l {
		if x["id"] != id {
			out = append(out, x)
		}
	}
	if err := atomicWriteJSON(wsFile(), out); err != nil {
		return 500, map[string]any{"error": err.Error()}
	}
	return 200, map[string]any{"ok": true}
}

// wsFromWork: the access of an existing work, as a workspace not saved yet (the editor opens with it).
func wsFromWork(slug string) (int, any) {
	code, p := spawnPlan(slug)
	if code != 200 {
		return code, p
	}
	return 200, map[string]any{"name": workTitle(slug), "root": p["rootShown"], "dirs": p["dirsShown"], "mode": p["mode"],
		"allow": []any{}, "skills": []any{}, "note": ""}
}

// wsSkills: the skills installed for the user (~/.claude/skills/<name>/SKILL.md), to pick from.
func wsSkills() []string {
	out := []string{}
	dir := filepath.Join(claudeHome(), "skills")
	es, _ := os.ReadDir(dir)
	for _, e := range es {
		if isFile(filepath.Join(dir, e.Name(), "SKILL.md")) {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// wsPlan is the plan of a new session from a workspace (what startWork takes).
func wsPlan(id string) (int, map[string]any) {
	for _, w := range wsList() {
		if w["id"] != id {
			continue
		}
		root := filepath.Clean(expandUser(str(w, "root")))
		if !isDir(root) {
			return 404, map[string]any{"error": "the main folder of the workspace is not there any more: " + str(w, "root")}
		}
		dirs, shown := []any{}, []any{}
		for _, d := range wsStrs(w["dirs"], 30) {
			if p := filepath.Clean(expandUser(d.(string))); isDir(p) && p != root {
				dirs, shown = append(dirs, p), append(shown, tilde(p))
			}
		}
		_, isGit := git(root, gitFast, "rev-parse", "--git-dir")
		return 200, map[string]any{"root": root, "rootShown": tilde(root), "dirs": dirs, "dirsShown": shown, "mode": str(w, "mode"),
			"git": isGit, "allow": wsStrs(w["allow"], 50), "skills": wsStrs(w["skills"], 40), "note": str(w, "note"), "workspace": str(w, "name")}
	}
	return 404, map[string]any{"error": "no such workspace"}
}

// wsIntro: what the first message of a session from a workspace says before the task: its context and its skills.
func wsIntro(p map[string]any) string {
	var b strings.Builder
	if n := str(p, "note"); n != "" {
		b.WriteString(n + "\n\n")
	}
	if sk := wsStrs(p["skills"], 40); len(sk) > 0 {
		names := make([]string, len(sk))
		for i, s := range sk {
			names[i] = s.(string)
		}
		b.WriteString("Use these skills when they fit the work: " + strings.Join(names, ", ") + ".\n\n")
	}
	return b.String()
}
