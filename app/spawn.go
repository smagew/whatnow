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
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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

func spawnPrompt(task, slug string) string {
	file := filepath.Join(featuresDir(), slug, "feature.json")
	// The first thing the session does is the page, from the task alone and in seconds: the shape of the file is
	// given here, so it reads no instructions and no code for it. A short or unclear task ("Messaging
	// improvement") is not a reason to explore a whole folder: the plan is one stage, agree on the task, and the
	// session asks. A session told to plan first read the instructions, listed the folders and the other works,
	// and took minutes before it asked what the task was.
	return task + "\n\n---\nThis is a new piece of work, started from the whatnow dashboard, which has opened a page for it already: " + file +
		" (slug \"" + slug + "\"). First give it a title and a plan, from the task above alone: do not read code, other files or other works " +
		"for this. Read that file, then rewrite it with the Write tool in this shape, in the language of the task, keeping its \"projects\":\n" +
		`{"title": "what this work is", "projects": [...], "stages": [{"n": 1, "title": "...", "verdict": "Not started", "plan": ["..."], "facts": []}], ` +
		`"decisions": [], "questions": [], "blockers": [], "nextSteps": []}` + "\n" +
		"Then run " + cliPath + " render " + file + ". If the task is short or unclear (it does not say what to change, where, or what the " +
		"result should be), make the plan a single stage \"Agree on the task\", ask the user what exactly to do, and wait for the answer; do " +
		"not explore the code before that. When the task is clear, do the work and keep the page in step with it; read " +
		"~/.claude/skills/whatnow/SKILL.md when you need more of the page (items for the user, problems, decisions). You may edit that file " +
		"and run " + cliPath + " without asking."
}

// spawnAllow is what a session of a new piece of work may do without asking, besides its permission mode: edit
// its own page and run the commands of the dashboard. Its page is outside the folder it works in, so without this
// every mode but "auto" would stop and ask, in a terminal that may not be open anywhere.
func spawnAllow(slug string) []string {
	dir := filepath.Join(featuresDir(), slug)
	exe := expandUser(cliPath)
	skill := filepath.Dir(exe) // the instructions of the dashboard (SKILL.md, SCHEMA.md, the examples): read only
	return []string{"Edit(/" + dir + "/**)", "Read(/" + dir + "/**)", "Read(/" + skill + "/**)",
		"Bash(" + cliPath + " render:*)", "Bash(" + cliPath + " inbox:*)",
		"Bash(" + exe + " render:*)", "Bash(" + exe + " inbox:*)"}
}

// folderPlan is spawnPlan for a folder the user names: there is no session to copy from, so the permission
// mode is the one the user chose ("" for the default of Claude Code).
func folderPlan(dir, mode string) (int, map[string]any) {
	dir = expandUser(strings.TrimSpace(dir))
	if dir == "" {
		return 400, map[string]any{"error": "choose a folder"}
	}
	if !filepath.IsAbs(dir) {
		return 400, map[string]any{"error": "give the full path of the folder, from / or ~"}
	}
	dir = filepath.Clean(dir)
	if !isDir(dir) {
		return 404, map[string]any{"error": "there is no such folder: " + tilde(dir)}
	}
	if mode != "" && !modeRe.MatchString(mode) {
		return 400, map[string]any{"error": "bad permission mode"}
	}
	_, isGit := git(dir, gitFast, "rev-parse", "--git-dir")
	return 200, map[string]any{"root": dir, "rootShown": tilde(dir), "dirs": []any{}, "dirsShown": []any{}, "mode": mode, "git": isGit,
		"name": filepath.Base(dir)}
}

var notTrusted = regexp.MustCompile(`(?i)not trusted`)

// workSlug is a free slug for a piece of work the dashboard itself opens (no task yet): from the name of its folder.
func workSlug(name string) string {
	base := strings.Trim(regexp.MustCompile(`[^A-Za-z0-9._-]+`).ReplaceAllString(strings.ToLower(name), "-"), "-.")
	if base == "" {
		base = "work"
	}
	slug := base
	for i := 2; isDir(filepath.Join(featuresDir(), slug)); i++ {
		slug = base + "-" + strconv.Itoa(i)
	}
	return slug
}

// openWork writes the first page of a new piece of work, before its session exists: named after the task (its
// first words) or, when there is none, after the folder. The session rewrites it with the plan.
//
// The dashboard opens the page itself, and not the session, for a reason seen on the first real use: the page is
// in ~/.whatnow, outside the folder the session works in, so Claude Code asked for a permission to create it, in
// a terminal nobody was looking at, and the new work sat under "Starting" for ten minutes.
func openWork(root, task string) (string, error) {
	name := filepath.Base(root)
	title, slug := "New work in "+name, workSlug(name)
	stage := map[string]any{"n": 1, "title": "Waiting for the task", "kind": "plan", "verdict": "Not started",
		"plan":  []any{"Give the session its task: write to it in the window of the session"},
		"facts": []any{[]any{"q", "The session is started and waits. When it has the task, it writes the plan here and gives this work its name."}}}
	summary, next := "Opened from the dashboard. It has no task yet.", "Write the task to the session"
	if task != "" {
		title = spawnName(task)
		if ascii := workSlug(title); ascii != "work" && len(ascii) >= 6 {
			slug = ascii
		}
		summary, next = runes(task, 600), "The session writes the plan"
		stage = map[string]any{"n": 1, "title": "Plan", "kind": "plan", "verdict": "Not started",
			"plan":  []any{"The session reads the task and writes the plan of this work here"},
			"facts": []any{[]any{"w", "The session has the task and is writing the plan"}}}
	}
	dir := filepath.Join(featuresDir(), slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	d := map[string]any{"title": title, "sub": "", "summary": summary, "projects": []any{name}, "updated": time.Now().Format("2006-01-02"),
		"stages": []any{stage}, "decisions": []any{}, "questions": []any{}, "blockers": []any{}, "nextSteps": []any{next}}
	return slug, atomicWriteJSON(filepath.Join(dir, "feature.json"), d)
}

// spawnFeature starts a session with the access of an existing piece of work (see spawnPlan).
func spawnFeature(slug, task, place string) (int, any) {
	code, p := spawnPlan(slug)
	if code != 200 {
		return code, p
	}
	return startWork(p, slug, task, place)
}

// startWork starts the session of a new piece of work. place is "same" (the folder itself) or "worktree" (a
// separate working copy of the repository, made by Claude Code, so two sessions do not change the same files).
// With a task, the session is told to make its own page and name it from the task. Without one it starts idle,
// and the dashboard opens an empty page for it, named after the folder.
func startWork(p map[string]any, from, task, place string) (int, any) {
	task = strings.TrimSpace(task)
	if len([]rune(task)) > 8000 {
		return 400, map[string]any{"error": "the task is too long (8000 characters max)"}
	}
	claude := claudeBin()
	if claude == "" {
		return 500, map[string]any{"error": "claude not found: set claudeBin in config.json"}
	}
	root := str(p, "root")
	name := spawnName(task)
	if task == "" {
		name = filepath.Base(root)
	}
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
	slug, err := openWork(root, task)
	if err != nil {
		return 500, map[string]any{"error": "could not open the page of the new work: " + err.Error()}
	}
	args = append(args, "--allowedTools")
	args = append(args, spawnAllow(slug)...)
	for _, a := range wsStrs(p["allow"], 50) { // the rules of a workspace
		args = append(args, a.(string))
	}
	intro := wsIntro(p) // the context and the skills of a workspace come first
	if task != "" {
		args = append(args, "--", intro+spawnPrompt(task, slug)) // "--": the list of tools above takes every word after it
	} else if intro != "" {
		args = append(args, "--", intro+"There is no task yet: say in one line that you are ready, and wait for it.")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, claude, args...)
	cmd.Dir, cmd.Env, cmd.Stdin = root, childEnv(claude), nil
	out, err := cmd.CombinedOutput()
	m := bgID.FindSubmatch(out)
	if m == nil {
		_ = os.RemoveAll(filepath.Join(featuresDir(), slug)) // no session: no page of it either
		msg := runes(strings.TrimSpace(string(out)), 400)
		if msg == "" && err != nil {
			msg = err.Error()
		}
		if notTrusted.MatchString(msg) {
			// Claude Code works only in a folder the user has trusted, and asks for that in its own window.
			// The dashboard does not answer that question for the user.
			return 409, map[string]any{"trust": tilde(root), "error": "Claude Code has not been allowed to work in this folder yet. Open a terminal, run `claude` in " +
				tilde(root) + " once and accept its question about trusting the folder; then press Start again."}
		}
		return 500, map[string]any{"error": "Claude Code did not start the session: " + msg}
	}
	short := string(m[1])
	sid := ""
	for i := 0; i < 6 && sid == ""; i++ { // the page is linked to the session, so its full id is waited for (a few seconds at most)
		if i > 0 {
			time.Sleep(400 * time.Millisecond)
		}
		for id, a := range readAgents() {
			if str(a, "id") == short {
				sid = id
			}
		}
	}
	if sid != "" {
		_ = bind(sid, slug)
	}
	rec := map[string]any{"id": short, "session": sid, "name": name, "task": runes(task, 300), "from": from, "ts": nowMs(),
		"place": place, "root": p["root"], "dirs": p["dirs"], "mode": p["mode"], "feature": slug}
	l := append(readSpawned(), rec)
	if len(l) > 50 {
		l = l[len(l)-50:]
	}
	_ = atomicWriteJSON(spawnedPath(), l)
	return 200, map[string]any{"ok": true, "id": short, "session": sid, "name": name, "feature": slug}
}

// spawnedNow are the started sessions that have no feature yet, for the bar.
func spawnedNow() []any {
	out := []any{}
	bound := bindings()
	now := nowMs()
	var agents agentMap
	l := readSpawned()
	changed := false
	for _, r := range l {
		sid := str(r, "session")
		if now-num(r["ts"]) > spawnKeep || (sid != "" && str(bound, sid) != "") || str(r, "feature") != "" {
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
		// A session started before the dashboard opened the page itself (or one whose page could not be written):
		// it gets its page now, so it leaves "Starting" and its window, with its question, is one click away.
		if sid != "" && isDir(str(r, "root")) {
			if slug, err := openWork(str(r, "root"), str(r, "task")); err == nil {
				_ = bind(sid, slug)
				r["feature"], r["session"] = slug, sid
				changed = true
				continue
			}
		}
		row := map[string]any{"session": sid, "name": r["name"], "task": r["task"], "from": r["from"], "ts": r["ts"]}
		if ses := findSession(sid); ses != nil { // what it does, so that "it asks for a permission" is not missed
			row["state"], row["what"] = ses.State, ses.What
		}
		out = append(out, row)
	}
	if changed {
		_ = atomicWriteJSON(spawnedPath(), l)
	}
	return out
}
