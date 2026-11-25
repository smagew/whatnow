package main

// A new feature with the access of an existing one: a clean Claude Code session (no context of the old one)
// started in the background in the same folder, with the same added folders and the same permission mode. The
// rules, the MCP servers and the instruction files belong to the folder, so the new session gets them by
// starting there.
//
// The user gives the task; the session is asked to put its work on the dashboard as a feature of its own, so the
// feature is named after the task. Until it has done that, the bar shows the session as "starting".
//
// What a new session is started with is kept with it (spawned.json): the day whatnow gets named workspaces,
// that record is what a workspace is.

import (
	"context"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	modeRe    = regexp.MustCompile(`^[A-Za-z]{2,40}$`)
	spawnKeep = int64(24 * 60 * 60 * 1000) // a session that never made a feature leaves the bar after a day
)

func spawnedPath() string { return filepath.Join(home(), "spawned.json") }

func readSpawned() []map[string]any {
	l, _ := parseJSON(readFile(spawnedPath())).([]any)
	out := []map[string]any{}
	for _, x := range l {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// spawnSource is the session of a feature whose access a new one copies: an open one if there is one, else the
// one seen last.
func spawnSource(slug string) (string, *transcript) {
	names := activeSessions()
	var sids []string
	for sid, f := range bindings() {
		if f == slug {
			sids = append(sids, sid)
		}
	}
	rank := func(sid string) int64 {
		var t int64
		if s := findSession(sid); s != nil {
			t = s.T
		}
		if names[sid] != nil {
			t += 1 << 50
		}
		return t
	}
	sort.Slice(sids, func(i, j int) bool { return rank(sids[i]) > rank(sids[j]) })
	for _, sid := range sids {
		if info := transcriptInfo(sid); info != nil {
			return sid, info
		}
	}
	return "", nil
}

// spawnPlan says what a new session started from this feature would get: shown in the form before anything is
// started, and used to start it.
func spawnPlan(slug string) (int, map[string]any) {
	if !slugRe.MatchString(slug) {
		return 400, map[string]any{"error": "bad feature"}
	}
	sid, info := spawnSource(slug)
	if info == nil {
		return 404, map[string]any{"error": "no session of this feature is known, so there is no access to copy"}
	}
	root := info.Root
	if s := findSession(sid); root == "" && s != nil {
		root = s.Cwd
	}
	if !isDir(root) {
		return 404, map[string]any{"error": "the folder this feature was started in is not there any more: " + tilde(root)}
	}
	dirs := []any{}
	seen := map[string]bool{root: true}
	for _, d := range info.AddDirs {
		// a folder inside the working folder is reachable already (and Claude Code does not list it)
		if d == "" || seen[d] || !isDir(d) || strings.HasPrefix(d, root+string(filepath.Separator)) {
			continue
		}
		seen[d] = true
		dirs = append(dirs, d)
	}
	mode := ""
	if a := accessOf(sid); a != nil && modeRe.MatchString(a.Mode) {
		mode = a.Mode
	}
	_, isGit := git(root, gitFast, "rev-parse", "--git-dir")
	shown := make([]any, len(dirs))
	for i, d := range dirs {
		shown[i] = tilde(d.(string))
	}
	return 200, map[string]any{"session": sid, "name": activeSessions().name(sid), "root": root, "rootShown": tilde(root),
		"dirs": dirs, "dirsShown": shown, "mode": mode, "git": isGit}
}

// spawnName is the name of the new session until its feature has one: the first words of the task.
func spawnName(task string) string {
	w := strings.Fields(strings.Map(func(r rune) rune {
		if r < ' ' || r == '"' || r == '`' {
			return ' '
		}
		return r
	}, task))
	name := ""
	for _, x := range w {
		if len([]rune(name))+len([]rune(x)) > 40 {
			break
		}
		name = strings.TrimSpace(name + " " + x)
	}
	if name == "" {
		name = "new feature"
	}
	return name
}

func spawnPrompt(task string) string {
	return task + "\n\n---\nThis is a new piece of work, started from the whatnow dashboard. Before you start it, put it on the dashboard " +
		"as a feature of its own: read ~/.claude/skills/whatnow/SKILL.md and SCHEMA.md, write a feature.json with a short slug and a " +
		"title that says what this work is (from the task above), the plan as stages, then run " + cliPath + " render --index. " +
		"Write the content in the language of the task. Then do the work and keep the dashboard in step with it."
}

// spawnFeature starts the session. place is "same" (the folder of the feature) or "worktree" (a separate working
// copy of the repository, made by Claude Code, so the two sessions do not change the same files).
func spawnFeature(slug, task, place string) (int, any) {
	task = strings.TrimSpace(task)
	if task == "" {
		return 400, map[string]any{"error": "write the task first"}
	}
	if len([]rune(task)) > 8000 {
		return 400, map[string]any{"error": "the task is too long (8000 characters max)"}
	}
	code, p := spawnPlan(slug)
	if code != 200 {
		return code, p
	}
	claude := claudeBin()
	if claude == "" {
		return 500, map[string]any{"error": "claude not found: set claudeBin in config.json"}
	}
	name := spawnName(task)
	args := []string{"--bg", "-n", name}
	if place == "worktree" {
		if p["git"] != true {
			return 400, map[string]any{"error": "the folder is not a git repository, so there is no separate copy to make"}
		}
		args = append(args, "--worktree", "wn-"+newID()[:8])
	}
	for _, d := range p["dirs"].([]any) {
		args = append(args, "--add-dir", d.(string))
	}
	if m := str(p, "mode"); m != "" {
		args = append(args, "--permission-mode", m)
	}
	args = append(args, spawnPrompt(task))
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, claude, args...)
	cmd.Dir, cmd.Env, cmd.Stdin = str(p, "root"), childEnv(claude), nil
	out, err := cmd.CombinedOutput()
	m := bgID.FindSubmatch(out)
	if m == nil {
		msg := runes(strings.TrimSpace(string(out)), 400)
		if msg == "" && err != nil {
			msg = err.Error()
		}
		return 500, map[string]any{"error": "Claude Code did not start the session: " + msg}
	}
	short := string(m[1])
	sid := ""
	// The full id is in the list of agents a moment later. It is not waited for: the bar finds it by the short one.
	for id, a := range readAgents() {
		if str(a, "id") == short {
			sid = id
		}
	}
	rec := map[string]any{"id": short, "session": sid, "name": name, "task": runes(task, 300), "from": slug, "ts": nowMs(),
		"place": place, "root": p["root"], "dirs": p["dirs"], "mode": p["mode"]}
	l := append(readSpawned(), rec)
	if len(l) > 50 {
		l = l[len(l)-50:]
	}
	_ = atomicWriteJSON(spawnedPath(), l)
	return 200, map[string]any{"ok": true, "id": short, "session": sid, "name": name}
}

// spawnedNow are the started sessions that have no feature yet, for the bar.
func spawnedNow() []any {
	out := []any{}
	bound := bindings()
	now := nowMs()
	var agents agentMap
	for _, r := range readSpawned() {
		sid := str(r, "session")
		if now-num(r["ts"]) > spawnKeep || (sid != "" && str(bound, sid) != "") {
			continue
		}
		if agents == nil {
			agents = activeSessions()
		}
		if sid == "" { // the full id was not known when it was started
			for id, a := range agents {
				if str(a, "id") == str(r, "id") {
					sid = id
				}
			}
			if sid != "" && str(bound, sid) != "" {
				continue
			}
		}
		if sid != "" && agents != nil && agents[sid] == nil && now-num(r["ts"]) > 60000 {
			continue // it is gone (stopped, removed) and made no feature
		}
		out = append(out, map[string]any{"session": sid, "name": r["name"], "task": r["task"], "from": r["from"], "ts": r["ts"]})
	}
	return out
}
