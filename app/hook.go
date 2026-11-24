package main

// whatnow hook: registered for several Claude Code hook events (see `whatnow hooks print`). Claude Code passes
// one JSON object on stdin. The hook (1) keeps the live state of each session in <home>/sessions/<id>.json and
// (2) appends interesting Bash events (git commit, git push, test runs) to <home>/events.jsonl.
//
// It never prints, never blocks and always exits 0, so a bug here cannot disturb a session.
// Three more modes deliver the messages typed on the page:
//
//	--deliver  (UserPromptSubmit, synchronous) prints pending messages; Claude Code adds them to the prompt.
//	--watch    (Stop and SessionStart, asyncRewake) waits while the session is idle; when a message arrives it
//	           exits 2, Claude Code wakes the session and shows stderr to Claude.
//	--nudge    (Stop, asyncRewake) reminds a session that did not update its feature's dashboard this turn.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	maxLog        = 5 * 1024 * 1024
	nudgeCooldown = 1800 // seconds between reminders to one session
)

var (
	commitRe   = regexp.MustCompile(`(?:^|[;&|(\s])git\s+(?:-C\s+\S+\s+)?commit\b`)
	pushRe     = regexp.MustCompile(`(?:^|[;&|(\s])git\s+(?:-C\s+\S+\s+)?push\b`)
	testRe     = regexp.MustCompile(`\b(phpunit|pytest|unittest|jest|vitest|go test|cargo test|npm (?:run )?test|yarn test|pnpm test|mvn test|gradle test)\b`)
	partSplit  = regexp.MustCompile(`[;&|\n]+`)
	envAssign  = regexp.MustCompile(`^\w+=`)
	msgFlag    = regexp.MustCompile(`-m\s+(?:"([^"]*)"|'([^']*)')`)
	heredoc    = regexp.MustCompile(`(?s)<<-?\s*'?(\w+)'?\s*\n(.*?)\n`)
	exitCode   = regexp.MustCompile(`^Exit code (\d+)`)
	ranRe      = regexp.MustCompile(`Ran (\d+) tests?`)
	failsRe    = regexp.MustCompile(`(?:failures|errors)=(\d+)`)
	okLine     = regexp.MustCompile(`(?m)^OK\b`)
	okCount    = regexp.MustCompile(`OK \((\d+) tests?`)
	phpunitRe  = regexp.MustCompile(`Tests:\s*(\d+)(?:,\s*Assertions:\s*\d+)?(?:,\s*Failures:\s*(\d+))?`)
	badRe      = regexp.MustCompile(`\b(\d+) (?:failed\b|errors? in \d)`)
	passedRe   = regexp.MustCompile(`(\d+) passed`)
	skillCmdRe = regexp.MustCompile(`(?:render|inbox)\.py([^|;&\n]*)|whatnow (?:render|inbox)([^|;&\n]*)`)
	notARun    = setOf("pip", "pip3", "pipx", "brew", "apt", "apt-get", "git", "grep", "egrep", "rg", "ag", "cat", "bat", "ls",
		"find", "fd", "echo", "printf", "head", "tail", "less", "sed", "awk", "wc", "which", "type", "man",
		"cd", "mkdir", "touch", "rm", "cp", "mv", "code", "vim", "nano", "open")
	installWords = setOf("install", "add", "remove", "uninstall", "i")
	writeTools   = setOf("Edit", "Write", "MultiEdit", "NotebookEdit")
)

func setOf(s ...string) map[string]bool {
	m := map[string]bool{}
	for _, x := range s {
		m[x] = true
	}
	return m
}

// isTestRun is true when the command really runs a test tool. A command that only mentions one
// (pip install pytest, grep unittest, git commit -m "fix jest") is not a test run.
func isTestRun(cmd string) bool {
	for _, part := range partSplit.Split(cmd, -1) {
		loc := testRe.FindStringIndex(part)
		if loc == nil {
			continue
		}
		var before []string
		for _, w := range strings.Fields(part[:loc[0]]) {
			if !envAssign.MatchString(w) {
				before = append(before, w)
			}
		}
		first := ""
		if len(before) > 0 {
			first = filepath.Base(strings.TrimLeft(before[0], "("))
		}
		install := false
		for _, w := range before {
			install = install || installWords[w]
		}
		if notARun[first] || install {
			continue
		}
		return true
	}
	return false
}

func gitOut(cwd string, args ...string) string {
	c := exec.Command("git", append([]string{"-C", cwd}, args...)...)
	var out bytes.Buffer
	c.Stdout = &out
	if c.Start() != nil {
		return ""
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case <-done:
		return strings.TrimSpace(out.String())
	case <-time.After(1500 * time.Millisecond):
		_ = c.Process.Kill()
		return ""
	}
}

func commitMessage(cmd string) string {
	text := ""
	if m := msgFlag.FindStringSubmatch(cmd); m != nil {
		text = m[1] + m[2]
		if strings.HasPrefix(text, "$(cat") {
			text = ""
		}
	}
	if text == "" {
		if h := heredoc.FindStringSubmatch(cmd); h != nil {
			text = h[2]
		}
	}
	return runes(strings.Join(strings.Fields(text), " "), 90)
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

func testSummary(out string) string {
	if ran := ranRe.FindStringSubmatch(out); ran != nil { // python unittest
		if strings.Contains(out, "FAILED") {
			bad := 0
			for _, m := range failsRe.FindAllStringSubmatch(out, -1) {
				bad += atoi(m[1])
			}
			if bad > 0 {
				return fmt.Sprintf("tests failed: %d of %s", bad, ran[1])
			}
			return "tests failed"
		}
		if okLine.MatchString(out) {
			return "tests passed: " + ran[1]
		}
	}
	if m := okCount.FindStringSubmatch(out); m != nil {
		return "tests passed: " + m[1]
	}
	if m := phpunitRe.FindStringSubmatch(out); m != nil && m[2] != "" {
		return fmt.Sprintf("tests failed: %s of %s", m[2], m[1])
	}
	// pytest / jest / vitest: "3 failed, 10 passed", "2 errors in 0.5s". Failures first: a pipe
	// (pytest | tail) hides the exit code, so the text is all we have.
	bad := 0
	for _, m := range badRe.FindAllStringSubmatch(out, -1) {
		bad += atoi(m[1])
	}
	m := passedRe.FindStringSubmatch(out)
	if bad > 0 {
		if m != nil {
			return fmt.Sprintf("tests failed: %d of %d", bad, bad+atoi(m[1]))
		}
		return fmt.Sprintf("tests failed: %d", bad)
	}
	if m != nil {
		return "tests passed: " + m[1]
	}
	return ""
}

func classify(p map[string]any, failed bool) (kind string, ok bool, text string) {
	in, _ := p["tool_input"].(map[string]any)
	cmd := str(in, "command")
	stdout := ""
	if resp, isMap := p["tool_response"].(map[string]any); isMap {
		stdout = str(resp, "stdout") + "\n" + str(resp, "stderr") // unittest prints to stderr
	}
	errText := str(p, "error")
	code := ""
	if m := exitCode.FindStringSubmatch(errText); m != nil {
		code = m[1]
	}
	switch {
	case commitRe.MatchString(cmd) && !failed:
		if msg := commitMessage(cmd); msg != "" {
			return "commit", true, `commit "` + msg + `"`
		}
		return "commit", true, "commit"
	case pushRe.MatchString(cmd):
		if failed {
			return "push", false, "push failed"
		}
		return "push", true, "push to origin"
	case isTestRun(cmd):
		txt := testSummary(stdout)
		if txt == "" {
			txt = testSummary(errText)
		}
		if failed || strings.HasPrefix(txt, "tests failed") {
			if txt == "" {
				txt = "tests failed"
				if code != "" {
					txt += " (exit " + code + ")"
				}
			}
			return "tests", false, txt
		}
		if txt == "" {
			txt = "tests passed"
		}
		return "tests", true, txt
	}
	return "", false, ""
}

func appendEvent(ev map[string]any) {
	path := filepath.Join(home(), "events.jsonl")
	_ = os.MkdirAll(home(), 0o755)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return
	}
	_, _ = f.Write(append(encodeJSON(ev), '\n')) // one write: lines of parallel sessions do not interleave
	f.Close()
	if st, err := os.Stat(path); err == nil && st.Size() > maxLog {
		if b, err := os.ReadFile(path); err == nil {
			lines := bytes.Split(bytes.TrimRight(b, "\n"), []byte("\n"))
			if len(lines) > 4000 {
				lines = lines[len(lines)-4000:]
			}
			_ = atomicWrite(path, append(bytes.Join(lines, []byte("\n")), '\n'))
		}
	}
}

func atomicWrite(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".tmp-")
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err == nil {
		err = f.Close()
	} else {
		f.Close()
	}
	if err != nil {
		os.Remove(f.Name())
		return err
	}
	return os.Rename(f.Name(), path)
}

// isWatcherCmd recognises our --watch processes in `ps` output, the Python one and this one.
func isWatcherCmd(cmdline string) bool {
	return strings.Contains(cmdline, "--watch") && (strings.Contains(cmdline, "hook.py") || strings.Contains(cmdline, "whatnow hook"))
}


func readPid(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return atoi(strings.TrimSpace(string(b)))
}

// featureIDs lists the slugs that have a readable feature.json.
func featureIDs() []string {
	var out []string
	entries, err := os.ReadDir(featuresDir())
	if err != nil {
		return out
	}
	for _, e := range entries {
		if _, ok := readJSON(filepath.Join(featuresDir(), e.Name(), "feature.json")).(map[string]any); ok {
			out = append(out, e.Name())
		}
	}
	return out
}

// slugIn is the feature whose folder inside the features directory is mentioned in text (a path or a shell
// command); the longest folder name wins. "" when no feature is mentioned.
func slugIn(text string, ids []string) string {
	if text == "" {
		return ""
	}
	root := strings.TrimRight(featuresDir(), "/")
	variants := []string{root}
	if real, err := filepath.EvalSymlinks(root); err == nil && real != root {
		variants = append(variants, real)
	}
	if h := userHome(); strings.HasPrefix(root, h) {
		variants = append(variants, "~"+root[len(h):])
	}
	best := ""
	for _, id := range ids {
		for _, r := range variants {
			p := r + "/" + id
			if strings.Contains(text, p+"/") || strings.Contains(text, p+`"`) || strings.HasSuffix(text, p) {
				if len(id) > len(best) {
					best = id
				}
			}
		}
	}
	return best
}

// autoBind links a session that WORKS on a feature to it: it edits the feature's files, or runs render /
// inbox on it. Reading or listing the files does not count.
func autoBind(sid string, tool string, input any) {
	in, ok := input.(map[string]any)
	if !ok {
		return
	}
	var texts []string
	switch {
	case writeTools[tool]:
		texts = []string{str(in, "file_path"), str(in, "notebook_path")}
	case tool == "Bash": // only the arguments of render / inbox count, not a path elsewhere in the command
		for _, m := range skillCmdRe.FindAllStringSubmatch(str(in, "command"), -1) {
			texts = append(texts, m[1]+m[2])
		}
	default:
		return
	}
	ids := featureIDs()
	for _, t := range texts {
		if slug := slugIn(t, ids); slug != "" {
			_ = bind(sid, slug)
			return
		}
	}
}

func featureOf(sid string) string { s, _ := bindings()[sid].(string); return s }

// claim atomically takes every undelivered message addressed to this session (or to its feature when no
// session was named). The lock file is created exclusively, so two watchers never deliver the same message.
func claim(sid, slug string) []map[string]any {
	var got []map[string]any
	for _, e := range inboxEntries() {
		if e["state"] != "new" {
			continue
		}
		es, hasSession := e["session"].(string)
		mine := (hasSession && es == sid) || (!truthy(e["session"]) && slug != "" && e["feature"] == slug)
		if !mine {
			continue
		}
		id := fmt.Sprint(e["id"])
		if !takeLock(id) {
			continue
		}
		if fresh := readMap(filepath.Join(inboxDir(), id+".json")); len(fresh) > 0 {
			e = fresh
		}
		if e["state"] != "new" {
			continue
		}
		e["state"], e["session"], e["deliveredAt"] = "delivered", sid, nowMs()
		_ = saveEntry(e)
		if f, _ := e["feature"].(string); f != "" {
			_ = bind(sid, f) // a session that took a message works on that feature
		}
		got = append(got, e)
	}
	return got
}

func messages(got []map[string]any) string {
	parts := make([]string, len(got))
	for i, e := range got {
		parts[i] = formatMessage(e)
	}
	return strings.Join(parts, "\n\n") + "\n"
}

func sessionFile(sid string) string {
	return filepath.Join(home(), "sessions", unsafeName.ReplaceAllString(sid, "_")+".json")
}

func envFloat(k string, def float64) float64 {
	if v, err := strconv.ParseFloat(os.Getenv(k), 64); err == nil {
		return v
	}
	return def
}

func hookWatch(sid string) int {
	wdir := filepath.Join(home(), "watch")
	_ = os.MkdirAll(wdir, 0o755)
	pidf := filepath.Join(wdir, unsafeName.ReplaceAllString(sid, "_")+".pid")
	if old := readPid(pidf); old > 0 && old != os.Getpid() && isWatcher(old) {
		stopProcess(old) // one watcher per session
	}
	_ = os.WriteFile(pidf, []byte(strconv.Itoa(os.Getpid())), 0o644)
	defer func() {
		if readPid(pidf) == os.Getpid() {
			os.Remove(pidf)
		}
	}()
	start, ppid := time.Now(), os.Getppid()
	deadline := start.Add(time.Duration(envFloat("WHATNOW_WATCH_SECONDS", 3300) * float64(time.Second))) // below the hook timeout
	poll := time.Duration(envFloat("WHATNOW_WATCH_POLL", 2) * float64(time.Second))
	for time.Now().Before(deadline) {
		// read the binding again: the session may be attached to a feature while it waits
		if got := claim(sid, featureOf(sid)); len(got) > 0 {
			os.Stderr.WriteString(messages(got))
			return 2
		}
		st := readMap(sessionFile(sid))
		if st["state"] == "closed" {
			return 0
		}
		if time.Since(start) > 5*time.Second && st["state"] == "work" && num(st["t"]) > start.Add(3*time.Second).UnixMilli() {
			return 0 // the user typed: the synchronous hook delivers at that prompt
		}
		if parentGone(ppid) {
			return 0 // Claude Code is gone
		}
		time.Sleep(poll)
	}
	return 0
}

func hookNudge(p map[string]any, sid string) int {
	if truthy(p["stop_hook_active"]) {
		return 0 // this turn was itself caused by a stop hook: never chain
	}
	slug := featureOf(sid)
	if slug == "" || config()["nudge"] == false {
		return 0
	}
	key := unsafeName.ReplaceAllString(sid, "_")
	promptT := num(readMap(sessionFile(sid))["promptT"])
	if promptT == 0 {
		return 0
	}
	st, err := os.Stat(filepath.Join(featuresDir(), slug, "feature.json"))
	if err != nil || st.ModTime().UnixMilli() >= promptT {
		return 0 // the feature file was updated during this turn
	}
	mark := filepath.Join(home(), "nudged", key)
	_ = os.MkdirAll(filepath.Dir(mark), 0o755)
	if b, err := os.ReadFile(mark); err == nil {
		seen, at, _ := strings.Cut(strings.TrimSpace(string(b)), " ")
		if seen == strconv.FormatInt(promptT, 10) {
			return 0 // already reminded for this prompt: never loop
		}
		if t, err := strconv.ParseFloat(at, 64); err == nil && float64(time.Now().UnixNano())/1e9-t < nudgeCooldown {
			return 0 // the reminder itself counts as a prompt: wait before the next
		}
	}
	_ = os.WriteFile(mark, []byte(fmt.Sprintf("%d %f", promptT, float64(time.Now().UnixNano())/1e9)), 0o644)
	fmt.Fprintf(os.Stderr, `[whatnow] You finished a turn on feature "%s" without updating its dashboard. If you told the user anything `+
		`about status, open items, results or what is left, make feature.json match it now (set "who" on every blocker, `+
		`keep evidence for anything marked done), then run %s render --index. `+
		`If nothing changed, reply with just "dashboard in sync".`+"\n", slug, cliPath)
	return 2
}

// runHook never fails: any trouble ends in exit code 0.
func runHook(args []string) (rc int) {
	defer func() {
		if recover() != nil {
			rc = 0
		}
	}()
	b, _ := io.ReadAll(os.Stdin)
	p, _ := parseJSON(b).(map[string]any)
	if p == nil {
		return 0
	}
	sid, event := str(p, "session_id"), str(p, "hook_event_name")
	if sid == "" {
		return 0
	}
	mode := ""
	if len(args) > 0 {
		mode = args[0]
	}
	switch mode {
	case "--deliver":
		if got := claim(sid, featureOf(sid)); len(got) > 0 {
			os.Stdout.WriteString(messages(got))
		}
		return 0
	case "--watch":
		return hookWatch(sid)
	case "--nudge":
		return hookNudge(p, sid)
	}
	cwd := str(p, "cwd")
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	spath := sessionFile(sid)
	if event == "SessionEnd" {
		if prev := readMap(spath); len(prev) > 0 {
			prev["state"], prev["what"], prev["t"] = "closed", "session closed", nowMs()
			_ = atomicWriteJSON(spath, prev)
		}
		return 0
	}
	top := gitOut(cwd, "rev-parse", "--show-toplevel")
	branch := gitOut(cwd, "branch", "--show-current")
	if branch == "" {
		branch = gitOut(cwd, "rev-parse", "--short", "HEAD")
	}
	prev := readMap(spath) // read after the slow git calls: a parallel hook may have written meanwhile
	project := top
	if project == "" {
		project = strings.TrimRight(cwd, "/")
	}
	or := func(v any, def any) any {
		if v == nil {
			return def
		}
		return v
	}
	state := map[string]any{
		"session": sid, "cwd": cwd, "project": filepath.Base(project), "branch": branch,
		"state": or(prev["state"], "idle"), "what": or(prev["what"], ""), "t": nowMs(), "promptT": or(prev["promptT"], 0),
	}
	switch event {
	case "SessionStart":
		state["state"], state["what"] = "idle", "session started"
	case "UserPromptSubmit":
		state["state"], state["what"], state["promptT"] = "work", "received a prompt", state["t"]
	case "PostToolUse", "PostToolUseFailure":
		tool := str(p, "tool_name")
		if tool == "Bash" {
			kind, ok, text := classify(p, event == "PostToolUseFailure")
			if text == "" {
				text = "running commands"
			}
			state["state"], state["what"] = "work", text
			if kind != "" {
				appendEvent(map[string]any{"ts": state["t"], "session": sid, "project": state["project"],
					"branch": branch, "kind": kind, "ok": ok, "text": text})
			}
		} else {
			state["state"] = "work"
		}
		autoBind(sid, tool, p["tool_input"])
	case "Stop":
		state["state"], state["what"] = "wait", "answered, waiting for you"
	case "Notification":
		switch str(p, "notification_type") {
		case "permission_prompt":
			state["state"], state["what"] = "wait", "waiting for permission"
		case "idle_prompt":
			state["state"], state["what"] = "idle", "idle"
		default:
			return 0
		}
	default:
		return 0
	}
	_ = atomicWriteJSON(spath, state)
	return 0
}
