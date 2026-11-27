package main

// The pages and their API: the home page, a page per feature, and the buttons on them.

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	workStale = 10 * 60 * 1000        // "work" without a single event for 10 min is not trusted
	dropAfter = 6 * 3600 * 1000       // an open session silent for 6 h is not shown
	historyMs = 30 * 24 * 3600 * 1000 // closed sessions are remembered for resuming
	bgMargin  = 5000                  // ms the list of open sessions must be newer than the hook event it overrides
	unstickMs = 10000                 // ms a message may wait for a session that cannot be woken
	maxBody   = 16 * 1024
	agentsTTL = 8 * time.Second
	trackText = "Please track the work of this session in the whatnow dashboard as a feature. Read ~/.claude/skills/whatnow/SKILL.md and SCHEMA.md, then write a feature.json (short slug) from what we have discussed and decided: the phases of the plan as stages (plan items; facts only for what was really done and verified, with evidence; deployment, tests and review as stages or items of the plan, only if the work really has them), and the open items as blockers with \"who\" set. Write the content in the language we work in. Then run " + cliPath + " render --index and tell me the slug."
)

var (
	slugRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	fontRe = regexp.MustCompile(`^/font/geologica-[a-z-]+\.woff2$`)
	fidRe  = regexp.MustCompile(`^[0-9a-f]{10}$`)
	pageRe = regexp.MustCompile(`^/f/([^/]+)/?$`)
	public = []string{"id", "kind", "feature", "fid", "action", "text", "item", "stage", "state", "route", "session",
		"sessionName", "ts", "deliveredAt", "note", "error", "asleep"}
)

// ------------------------------------------------------------------ open sessions (`claude agents`)

type agentMap map[string]map[string]any

var agents struct {
	mu   sync.Mutex
	at   time.Time
	v    agentMap
	busy sync.Mutex
}

// activeSessions lists the sessions open right now; nil when claude is not available. Cached for a few
// seconds; one refresh at a time, the other requests get the previous answer meanwhile.
func activeSessions() agentMap {
	agents.mu.Lock()
	at, v := agents.at, agents.v
	agents.mu.Unlock()
	if time.Since(at) < agentsTTL {
		return v
	}
	if at.IsZero() {
		agents.busy.Lock()
	} else if !agents.busy.TryLock() {
		return v
	}
	defer agents.busy.Unlock()
	agents.mu.Lock()
	fresh := time.Since(agents.at) < agentsTTL
	agents.mu.Unlock()
	if !fresh {
		nv := readAgents()
		agents.mu.Lock()
		agents.v, agents.at = nv, time.Now()
		agents.mu.Unlock()
	}
	agents.mu.Lock()
	defer agents.mu.Unlock()
	return agents.v
}

func agentsAt() int64 {
	agents.mu.Lock()
	defer agents.mu.Unlock()
	if agents.at.IsZero() {
		return 0
	}
	return agents.at.UnixMilli()
}

func readAgents() agentMap {
	claude := claudeBin()
	if claude == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, claude, "agents", "--json")
	cmd.Env = childEnv(claude)
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return nil
	}
	l, ok := parseJSON(out).([]any)
	if !ok {
		return nil
	}
	m := agentMap{}
	for _, x := range l {
		if a, ok := x.(map[string]any); ok {
			if id, _ := a["sessionId"].(string); id != "" {
				m[id] = a
			}
		}
	}
	return m
}

func (a agentMap) name(sid string) string { return str(a[sid], "name") }

// ------------------------------------------------------------------ sessions

type session struct {
	ID      string  `json:"id"`
	Project any     `json:"project"`
	Branch  string  `json:"branch"`
	Cwd     string  `json:"cwd"`
	State   string  `json:"state"`
	What    string  `json:"what"`
	T       int64   `json:"t"`
	Feature any     `json:"feature"`
	Name    string  `json:"name"`
	Quiet   bool    `json:"quiet,omitempty"`
	Hidden  bool    `json:"hidden"`
	Group   string  `json:"group"`
	First   *string `json:"first,omitempty"`
	LastU   *string `json:"lastU,omitempty"`
	LastA   *string `json:"lastA,omitempty"`
	Use     any     `json:"use,omitempty"` // what its process and the processes under it take: {cpu, ram, procs}
	Ctx     any     `json:"ctx,omitempty"` // how full its context is: {used, window, model}
}

func (s *session) feature() string { f, _ := s.Feature.(string); return f }

// projectOf is the repository name of a folder: the nearest parent that has .git, else the folder itself.
func projectOf(cwd string, aliases map[string]any) string {
	d := strings.TrimRight(cwd, "/")
	for probe := d; probe != "" && probe != "/"; probe = filepath.Dir(probe) {
		if _, err := os.Stat(filepath.Join(probe, ".git")); err == nil {
			d = probe
			break
		}
	}
	name := filepath.Base(d)
	if a, ok := aliases[name].(string); ok {
		return a
	}
	return name
}

// readSessions lists every session we know: the hook files (details, closed history) plus, as the source of
// truth for what is open right now, `claude agents`.
func readSessions(now int64, includeClosed, withHidden, details bool) []*session {
	aliases, _ := config()["aliases"].(map[string]any)
	bound := bindings()
	ag := activeSessions()
	hid := hidden()
	out, seen := []*session{}, map[string]bool{}
	names, _ := os.ReadDir(sessionsDir())
	for _, n := range names {
		if !strings.HasSuffix(n.Name(), ".json") {
			continue
		}
		s := readMap(filepath.Join(sessionsDir(), n.Name()))
		if len(s) == 0 {
			continue
		}
		sid, _ := s["session"].(string)
		if sid == "" {
			sid = strings.TrimSuffix(n.Name(), ".json")
		}
		t := num(s["t"])
		age := now - t
		state, _ := s["state"].(string)
		if state == "" {
			state = "idle"
		}
		_, listed := ag[sid]
		isOpen := ag != nil && listed && state != "closed"
		// Claude Code no longer lists it (read after its last activity): it ended without a word (closed with the
		// terminal, crashed, a background session stopped). It is closed, kept like one for resuming, and its
		// work still shows it; as "open but silent" it vanished after 6 h and the work said no session was linked.
		if ag != nil && !listed && state != "closed" && agentsAt() > t+bgMargin {
			state = "closed"
		}
		if state == "closed" && !isOpen {
			if !includeClosed || age > historyMs {
				continue
			}
		} else if !isOpen && age > dropAfter {
			continue
		}
		what := str(s, "what")
		busy := isOpen && str(ag[sid], "status") == "busy"
		if state == "work" && age > workStale && !busy {
			state = "idle"
		} else if (state == "wait" || state == "idle") && busy && what != "waiting for permission" && agentsAt() > t+bgMargin {
			// The turn ended, yet Claude Code still reports the session busy: a background command or agent runs,
			// and the session goes on by itself when it finishes. It does not wait for the user.
			state, what = "bg", "waiting for a background task"
		}
		project := s["project"]
		if p, ok := project.(string); ok {
			if a, ok := aliases[p]; ok {
				project = a
			}
		}
		seen[sid] = true
		if hid[sid] && !withHidden {
			continue
		}
		if _, ok := s["t"]; !ok {
			t = now
		}
		out = append(out, &session{ID: sid, Project: project, Branch: str(s, "branch"), Cwd: str(s, "cwd"), State: state,
			What: what, T: t, Feature: bound[sid], Name: ag.name(sid)})
	}
	for sid, a := range ag {
		if seen[sid] || (hid[sid] && !withHidden) {
			continue
		}
		cwd := str(a, "cwd")
		project := ""
		if cwd != "" {
			project = projectOf(cwd, aliases)
		}
		state, t := "idle", num(a["startedAt"])
		if str(a, "status") == "busy" {
			state = "work"
		}
		if t == 0 {
			t = now
		}
		out = append(out, &session{ID: sid, Project: project, Cwd: cwd, State: state, T: t, Quiet: true, Feature: bound[sid], Name: str(a, "name")})
	}
	homeDir := strings.TrimRight(userHome(), "/")
	for _, r := range out {
		r.Hidden = hid[r.ID]
		r.Use = usageOf(ag[r.ID])
		switch {
		case strings.TrimRight(r.Cwd, "/") == homeDir:
			r.Group = "~ home"
		case truthy(r.Project):
			r.Group = fmt.Sprint(r.Project)
		case filepath.Base(strings.TrimRight(r.Cwd, "/")) != "." && strings.TrimRight(r.Cwd, "/") != "":
			r.Group = filepath.Base(strings.TrimRight(r.Cwd, "/"))
		default:
			r.Group = "unknown"
		}
	}
	if details {
		for _, r := range out {
			if info := transcriptInfo(r.ID); info != nil {
				r.First, r.LastU, r.LastA = &info.First, &info.LastU, &info.LastA
				if info.Ctx > 0 {
					r.Ctx = map[string]any{"used": info.Ctx, "window": ctxWindow(info.Model, info.Ctx), "model": info.Model}
				}
				if info.Mtime > r.T {
					r.T = info.Mtime
				}
				r.Quiet = false
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].T > out[j].T })
	return out
}

func findSession(sid string) *session {
	for _, r := range readSessions(nowMs(), true, true, false) {
		if r.ID == sid {
			return r
		}
	}
	return nil
}

func readEvents(limit int) map[string]any {
	aliases, _ := config()["aliases"].(map[string]any)
	bound := bindings()
	by := map[string]any{}
	b, err := os.ReadFile(filepath.Join(home(), "events.jsonl"))
	if err != nil {
		return by
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) > 3000 {
		lines = lines[len(lines)-3000:]
	}
	for i := len(lines) - 1; i >= 0; i-- {
		e, ok := parseJSON([]byte(lines[i])).(map[string]any)
		if !ok {
			continue
		}
		project := e["project"]
		if p, ok := project.(string); ok {
			if a, ok := aliases[p]; ok {
				project = a
			}
		}
		slug, _ := bound[str(e, "session")].(string)
		if slug == "" {
			continue
		}
		lst, _ := by[slug].([]any)
		if len(lst) < limit {
			k := "bad"
			if e["kind"] == "commit" || e["kind"] == "push" {
				k = "in"
			} else if truthy(e["ok"]) {
				k = "ok"
			}
			ts := e["ts"]
			if ts == nil {
				ts = 0
			}
			by[slug] = append(lst, map[string]any{"k": k, "p": project, "t": str(e, "text"), "ts": ts})
		}
	}
	return by
}

func stateJSON() map[string]any {
	now := nowMs()
	payload := collect(featuresDir())
	closed := closedWorks()
	for _, f := range payload["features"].([]any) {
		m := f.(map[string]any)
		m["href"] = "/f/" + fmt.Sprint(m["id"]) + "/"
		if t := closed[fmt.Sprint(m["id"])]; t != 0 {
			m["closed"] = t
		}
	}
	payload["live"], payload["now"] = true, now
	payload["sessions"] = readSessions(now, true, true, true)
	payload["favorites"] = favorites()
	payload["events"] = readEvents(30)
	return payload
}

// ------------------------------------------------------------------ inbox and routing

func publicOf(e map[string]any) map[string]any {
	out := map[string]any{}
	for _, k := range public {
		if v, ok := e[k]; ok {
			out[k] = v
		}
	}
	return out
}

// awake says whether an open session can take a message now: while it works (its Stop hook picks the
// message up when the turn ends) or while its watcher runs.
func awake(s *session) bool { return s.State == "work" || watcherAlive(s.ID) }

// unstick re-addresses a message of a feature that nobody can take: to a session of the feature that can take
// it now, else to the feature as a whole, marked "asleep" so the page says why it waits.
func unstick(e map[string]any, now int64) map[string]any {
	sid, _ := e["session"].(string)
	ts := num(e["ts"])
	if _, ok := e["ts"]; !ok {
		ts = now
	}
	if !(e["state"] == "new" && truthy(e["feature"]) && e["kind"] != "session" && now-ts > unstickMs) {
		return e
	}
	if sid != "" {
		if row := findSession(sid); row != nil && awake(row) {
			return e
		}
	}
	route, s, name := planRoute(fmt.Sprint(e["feature"]))
	var target any
	if route == "active" && awake(s) {
		target = s.ID
	}
	if route != "active" {
		name = ""
	}
	want := map[string]any{"session": target, "asleep": target == nil, "sessionName": name}
	same := true
	for k, v := range want {
		if cur, ok := e[k]; !ok || cur != v {
			if !(k == "session" && v == nil && !ok) {
				same = false
			}
		}
	}
	if same {
		return e
	}
	id := fmt.Sprint(e["id"])
	if !takeLock(id) { // the same lock a delivery takes
		return e
	}
	defer os.Remove(filepath.Join(inboxDir(), id+".lock"))
	cur := readMap(filepath.Join(inboxDir(), id+".json"))
	if len(cur) == 0 {
		cur = e
	}
	if cur["state"] == "new" {
		for k, v := range want {
			cur[k] = v
		}
		if target == nil {
			delete(cur, "session")
		}
		_ = saveEntry(cur)
	}
	return cur
}

// cancelSync gives up on an open status request, so that a new one can be made.
func cancelSync(slug string) (int, any) {
	n := 0
	for _, e := range inboxEntries() {
		if e["feature"] == slug && e["kind"] == "sync" && (e["state"] == "new" || e["state"] == "delivered") {
			id := fmt.Sprint(e["id"])
			if e["state"] == "new" {
				takeLock(id) // taken for good: it must not arrive later
			}
			if cur := readMap(filepath.Join(inboxDir(), id+".json")); len(cur) > 0 {
				e = cur
			}
			e["state"], e["note"] = "done", "cancelled by the user"
			_ = saveEntry(e)
			n++
		}
	}
	return 200, map[string]any{"ok": true, "cancelled": n}
}

// planRoute decides where a message for this feature goes. No side effects.
func planRoute(slug string) (string, *session, string) {
	var mine []*session
	for _, s := range readSessions(nowMs(), true, false, false) {
		if s.feature() == slug {
			mine = append(mine, s)
		}
	}
	ag := activeSessions()
	var live []*session
	for _, s := range mine {
		_, listed := ag[s.ID]
		if (ag != nil && listed && s.State != "closed") || (ag == nil && s.State != "closed") {
			live = append(live, s)
		}
	}
	if len(live) > 0 {
		sort.SliceStable(live, func(i, j int) bool { return awake(live[i]) && !awake(live[j]) })
		return "active", live[0], ag.name(live[0].ID)
	}
	for _, s := range mine {
		_, listed := ag[s.ID]
		if s.State == "closed" || (ag != nil && !listed) {
			if sidRe.MatchString(s.ID) && isDir(s.Cwd) {
				return "bg", s, ""
			}
		}
	}
	return "none", nil, ""
}

func resumeInBackground(sid, cwd string, entry map[string]any) (bool, string) {
	claude := claudeBin()
	if claude == "" {
		return false, "claude not found: set claudeBin in config.json"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, claude, "--resume", sid, "--bg", formatMessage(entry))
	cmd.Dir, cmd.Env, cmd.Stdin = cwd, childEnv(claude), nil
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 && cmd.ProcessState == nil {
		return false, "could not start claude: " + err.Error()
	}
	return err == nil, runes(strings.TrimSpace(string(out)), 400)
}

// usage of the processes of the machine, read with one `ps` and kept for a few seconds.
var procUse struct {
	mu   sync.Mutex
	at   time.Time
	cpu  map[int]float64
	rss  map[int]int64 // KB
	kids map[int][]int
}

// usageOf is what an open session takes right now: its own process and everything started under it (commands,
// MCP servers). cpu is in percent of one core, ram in MB. nil when it is not known (closed, or no `ps`).
func usageOf(a map[string]any) any {
	pid := int(num(a["pid"]))
	if pid <= 0 {
		return nil
	}
	procUse.mu.Lock()
	defer procUse.mu.Unlock()
	if time.Since(procUse.at) > 5*time.Second {
		procUse.cpu, procUse.rss, procUse.kids = readUsage()
		procUse.at = time.Now()
	}
	if _, ok := procUse.rss[pid]; !ok {
		return nil
	}
	var cpu float64
	var rss int64
	n := 0
	var walk func(p, depth int)
	walk = func(p, depth int) {
		cpu, rss, n = cpu+procUse.cpu[p], rss+procUse.rss[p], n+1
		if depth < 12 {
			for _, k := range procUse.kids[p] {
				walk(k, depth+1)
			}
		}
	}
	walk(pid, 0)
	return map[string]any{"cpu": math.Round(cpu*10) / 10, "ram": rss / 1024, "procs": n}
}

// sessionDirs says where a session may work: the folder it was started in (the agent reaches everything under
// it), the folder it works in now, the folders added to it, and the other folders it worked in lately.
func sessionDirs(s *session, info *transcript) map[string]any {
	short := func(l []string) []any {
		out := []any{}
		for _, d := range l {
			out = append(out, tilde(d))
		}
		return out
	}
	root, now := s.Cwd, s.Cwd
	add, seen := []any{}, []any{}
	if info != nil {
		if info.Root != "" {
			root = info.Root
		}
		if info.Dir != "" {
			now = info.Dir
		}
		add, seen = short(info.AddDirs), short(info.Seen)
	}
	if root == "" {
		root = now
	}
	if now == "" {
		now = root
	}
	return map[string]any{"root": tilde(root), "now": tilde(now), "add": add, "seen": seen}
}

// pickList gives the sessions that can be attached to a feature, open ones first. Sessions already on this
// feature are listed apart, with what they said last.
func pickList(slug string) map[string]any {
	ag := activeSessions()
	titles := map[string]any{}
	for _, f := range collectIndex() {
		titles[f[0]] = f[1]
	}
	type row = map[string]any
	var out, mine []row
	for _, s := range readSessions(nowMs(), true, false, false) {
		_, listed := ag[s.ID]
		live := listed && s.State != "closed"
		state := "closed"
		if live {
			state = s.State
		}
		other := any("")
		if f := s.feature(); f != "" {
			other = f
			if t, ok := titles[f]; ok {
				other = t
			}
		}
		r := row{"id": s.ID, "name": ag.name(s.ID), "project": s.Project, "what": s.What, "t": s.T, "open": live, "state": state, "other": other}
		r["bg"] = live && ag[s.ID]["kind"] == "background" // a background session can be typed into from here
		if s.feature() == slug {
			info := transcriptInfo(s.ID)
			if info != nil && info.Said != "" {
				r["said"], r["saidT"] = info.Said, info.SaidT
			}
			r["dirs"] = sessionDirs(s, info)
			if info != nil && info.Ctx > 0 {
				r["ctx"] = map[string]any{"used": info.Ctx, "window": ctxWindow(info.Model, info.Ctx), "model": info.Model}
			}
			if u := usageOf(ag[s.ID]); u != nil {
				r["use"] = u
			}
			mine = append(mine, r)
		} else {
			out = append(out, r)
		}
	}
	order := func(l []row) {
		sort.SliceStable(l, func(i, j int) bool {
			if l[i]["open"] != l[j]["open"] {
				return l[i]["open"].(bool)
			}
			return l[i]["t"].(int64) > l[j]["t"].(int64)
		})
	}
	order(out)
	order(mine)
	if len(out) > 40 {
		out = out[:40]
	}
	if out == nil {
		out = []row{}
	}
	if mine == nil {
		mine = []row{}
	}
	return map[string]any{"bound": mine, "sessions": out}
}

// collectIndex is the light list of features: [slug, title].
func collectIndex() [][2]string {
	var out [][2]string
	names, _ := os.ReadDir(featuresDir())
	for _, n := range names {
		d := readMap(filepath.Join(featuresDir(), n.Name(), "feature.json"))
		if len(d) == 0 {
			continue
		}
		t, ok := d["title"].(string)
		if !ok {
			t = n.Name()
		}
		out = append(out, [2]string{n.Name(), t})
	}
	return out
}

func attach(body map[string]any) (int, any) {
	slug, _ := body["feature"].(string)
	sid, _ := body["session"].(string)
	if !(slugRe.MatchString(slug) && isFile(filepath.Join(featuresDir(), slug, "feature.json"))) {
		return 404, map[string]any{"error": "unknown feature"}
	}
	if !sidRe.MatchString(sid) {
		return 400, map[string]any{"error": "bad session"}
	}
	_ = bind(sid, slug)
	return 200, map[string]any{"ok": true}
}

// tellSession sends a free message to one session (the Sessions view and the chat window).
func tellSession(sid string, text string) (int, any) {
	text = strings.TrimSpace(text)
	if !sidRe.MatchString(sid) {
		return 400, map[string]any{"error": "bad session"}
	}
	if text == "" {
		return 400, map[string]any{"error": "write the message first"}
	}
	if len([]rune(text)) > 2000 {
		return 400, map[string]any{"error": "text is too long (2000 characters max)"}
	}
	ag := activeSessions()
	row := findSession(sid)
	now := nowMs()
	entry := map[string]any{"id": newID(), "ts": now, "state": "new", "kind": "session", "feature": "", "fid": "session",
		"stage": 0, "stageTitle": "", "item": "Message to a session", "action": "ask", "text": text, "session": sid}
	_, listed := ag[sid]
	switch {
	case (ag != nil && listed) || (ag == nil && row != nil && row.State != "closed"):
		entry["route"], entry["sessionName"] = "active", ag.name(sid)
		if row == nil || !awake(row) { // nothing listens there: a background session is woken through its terminal
			wakeSession(sid)
		}
	case row != nil && isDir(row.Cwd):
		ok, out := resumeInBackground(sid, row.Cwd, entry)
		entry["route"] = "bg"
		if ok {
			entry["state"], entry["deliveredAt"], entry["note"] = "delivered", nowMs(), out
		} else {
			entry["state"], entry["error"] = "failed", out
		}
	default:
		return 404, map[string]any{"error": "this session is closed and its folder is unknown, so it cannot be resumed"}
	}
	_ = saveEntry(entry)
	return 200, map[string]any{"ok": true, "entry": publicOf(entry)}
}

// chat is the recent conversation of one session for the chat window: what the user typed in the terminal ('u'),
// what was sent from the page ('me', with its delivery state) and what Claude answered ('a').
func chat(sid string) map[string]any {
	items := []map[string]any{}
	if info := transcriptInfo(sid); info != nil {
		for _, c := range info.Chat {
			items = append(items, map[string]any{"who": c.who, "text": c.text, "t": c.t})
		}
	}
	var row *session
	looked := false
	for _, e := range inboxEntries() {
		if e["kind"] != "session" || e["session"] != sid {
			continue
		}
		it := map[string]any{"who": "me", "text": e["text"], "t": num(e["ts"])}
		if _, ok := e["text"]; !ok {
			it["text"] = ""
		}
		for _, k := range []string{"id", "state", "deliveredAt", "note", "error", "route"} {
			if v, ok := e[k]; ok {
				it[k] = v
			}
		}
		if e["state"] == "new" {
			if !looked {
				row, looked = findSession(sid), true
			}
			it["asleep"] = row != nil && !awake(row)
		}
		items = append(items, it)
	}
	sort.SliceStable(items, func(i, j int) bool { return num(items[i]["t"]) < num(items[j]["t"]) })
	if len(items) > 30 {
		items = items[len(items)-30:]
	}
	return map[string]any{"items": items}
}

func favorite(body map[string]any) (int, any) {
	kind, _ := body["kind"].(string)
	id, _ := body["id"].(string)
	var key string
	switch {
	case kind == "session" && sidRe.MatchString(id):
		key = "s:" + id
	case kind == "feature" && slugRe.MatchString(id):
		key = "f:" + id
	default:
		return 400, map[string]any{"error": "bad favorite"}
	}
	_ = setFavorite(key, body["action"] == "fav")
	return 200, map[string]any{"ok": true}
}

func act(body map[string]any) (int, any) {
	action, _ := body["action"].(string)
	sid, _ := body["session"].(string)
	text, _ := body["text"].(string)
	switch action {
	case "track":
		return tellSession(sid, trackText)
	case "compact": // the context of a session is full: /compact with what to keep
		return compactSession(sid, text)
	case "clear": // /clear, then the session goes on from the page of its work
		return clearSession(sid)
	case "fresh": // a new session with the same access takes over the work from its page
		return freshSession(sid)
	case "ws-save":
		w, _ := body["workspace"].(map[string]any)
		return wsSave(w)
	case "ws-delete":
		id, _ := body["id"].(string)
		return wsDelete(id)
	case "finish": // the work is over: its background sessions stop, it leaves the bar (after a confirmation on the page)
		slug, _ := body["feature"].(string)
		return finishWork(slug)
	case "reopen":
		slug, _ := body["feature"].(string)
		return reopenWork(slug)
	case "awake": // keep the computer awake for "minutes" (0: off, -1: until turned off)
		return setAwake(num(body["minutes"]), body["screen"] != false) // the screen stays on unless the page says no
	case "broadcast": // one message to several sessions; "interrupt" first stops the background ones (after a confirmation on the page)
		return broadcast(body)
	case "spawn":
		place, _ := body["place"].(string)
		slug, _ := body["feature"].(string)
		if ws, _ := body["workspace"].(string); ws != "" { // a new piece of work from a workspace
			code, p := wsPlan(ws)
			if code != 200 {
				return code, p
			}
			return startWork(p, "", text, place)
		}
		if dir, _ := body["dir"].(string); dir != "" { // a new piece of work in a folder the user named
			mode, _ := body["mode"].(string)
			code, p := folderPlan(dir, mode)
			if code != 200 {
				return code, p
			}
			return startWork(p, "", text, place)
		}
		return spawnFeature(slug, text, place)
	case "fav", "unfav":
		return favorite(body)
	case "hide", "unhide":
		if !sidRe.MatchString(sid) {
			return 400, map[string]any{"error": "bad session"}
		}
		_ = setHidden(sid, action == "hide")
		return 200, map[string]any{"ok": true}
	case "tell":
		return tellSession(sid, text)
	case "attach":
		return attach(body)
	case "sync", "unsync":
		f, _ := body["feature"].(string)
		if !slugRe.MatchString(f) {
			return 400, map[string]any{"error": "bad feature"}
		}
		if action == "sync" {
			return requestSync(f)
		}
		return cancelSync(f)
	}
	slug, _ := body["feature"].(string)
	fid, _ := body["fid"].(string)
	text = strings.TrimSpace(text)
	if !slugRe.MatchString(slug) {
		return 400, map[string]any{"error": "bad feature"}
	}
	if !fidRe.MatchString(fid) {
		return 400, map[string]any{"error": "bad item id"}
	}
	isBlocker := body["kind"] == "blocker"
	if action != "do" && action != "ask" && action != "skip" && action != "unskip" && !(action == "done" && isBlocker) {
		return 400, map[string]any{"error": "bad action"}
	}
	if len([]rune(text)) > 2000 {
		return 400, map[string]any{"error": "text is too long (2000 characters max)"}
	}
	src := filepath.Join(featuresDir(), slug, "feature.json")
	if !isFile(src) {
		return 404, map[string]any{"error": "unknown feature"}
	}
	data, err := load(src)
	if err != nil {
		return 500, map[string]any{"error": "feature.json: " + err.Error()}
	}
	var stage map[string]any
	var fact []any
	if isBlocker { // an item of "what is left"; the user can also close it: "done" (did it) or "skip" (not needed)
		all := append(append([]any{}, data["blockers"].([]any)...), data["closedBlockers"].([]any)...)
		for _, x := range all {
			if b := x.(map[string]any); b["bid"] == fid {
				for _, y := range data["stages"].([]any) {
					if s := y.(map[string]any); num(s["n"]) == num(b["stage"]) {
						stage, fact = s, []any{"q", b["t"]}
						break
					}
				}
				break
			}
		}
	} else {
		for _, y := range data["stages"].([]any) {
			s := y.(map[string]any)
			for i, f := range s["fids"].([]any) {
				if f == fid {
					stage, fact = s, s["facts"].([]any)[i].([]any)
				}
			}
		}
	}
	if stage == nil {
		return 404, map[string]any{"error": "this item no longer exists, reload the page"}
	}
	if action == "skip" || action == "unskip" || action == "done" {
		path := filepath.Join(featuresDir(), slug, "overrides.json")
		ov := readMap(path)
		if action == "unskip" {
			delete(ov, fid)
		} else {
			if k := kindOf(fact[0]); k == "y" || k == "fixed" {
				return 400, map[string]any{"error": "a finished item cannot be marked as not needed"}
			}
			ov[fid] = map[string]any{"kind": action, "reason": text, "ts": nowMs()}
		}
		if err := atomicWriteJSON(path, ov); err != nil {
			return 500, map[string]any{"error": "error: " + err.Error()}
		}
		if !isBlocker || action == "unskip" {
			return 200, map[string]any{"ok": true, "applied": action}
		}
		// an open item closed by the user: it is closed on the page already, and the session is told, so that
		// it records it in feature.json (a fact with the user's word as evidence, or a decision)
		entry := map[string]any{"id": newID(), "ts": nowMs(), "state": "new", "feature": slug, "featureTitle": data["title"], "fid": fid,
			"stage": stage["n"], "stageTitle": stage["title"], "item": fact[1], "action": action, "text": text}
		return 200, map[string]any{"ok": true, "applied": action, "entry": publicOf(sendEntry(entry))}
	}
	entry := map[string]any{"id": newID(), "ts": nowMs(), "state": "new", "feature": slug, "featureTitle": data["title"], "fid": fid,
		"stage": stage["n"], "stageTitle": stage["title"], "item": fact[1], "action": action, "text": text}
	return 200, map[string]any{"ok": true, "entry": publicOf(sendEntry(entry))}
}

// sendEntry routes a new inbox entry (open session / background resume / wait) and stores it.
func sendEntry(entry map[string]any) map[string]any {
	route, s, name := planRoute(fmt.Sprint(entry["feature"]))
	entry["route"] = route
	switch route {
	case "active":
		entry["sessionName"] = name
		if awake(s) {
			entry["session"] = s.ID
		} else { // nobody can take it now: left to the first session of the feature that can
			entry["asleep"] = true
			if wakeSession(s.ID) { // a background session is woken through its terminal, and takes it then
				entry["session"] = s.ID
			}
		}
	case "bg":
		ok, out := resumeInBackground(s.ID, s.Cwd, entry)
		entry["session"] = s.ID
		if ok {
			entry["state"], entry["deliveredAt"], entry["note"] = "delivered", nowMs(), out
		} else {
			entry["state"], entry["error"] = "failed", out
		}
	}
	_ = saveEntry(entry)
	return entry
}

func requestSync(slug string) (int, any) {
	src := filepath.Join(featuresDir(), slug, "feature.json")
	if !isFile(src) {
		return 404, map[string]any{"error": "unknown feature"}
	}
	for _, e := range inboxEntries() {
		if e["feature"] == slug && e["kind"] == "sync" && (e["state"] == "new" || e["state"] == "delivered") {
			return 200, map[string]any{"ok": true, "entry": publicOf(e)} // one request at a time
		}
	}
	data, err := load(src)
	if err != nil {
		return 500, map[string]any{"error": "feature.json: " + err.Error()}
	}
	entry := map[string]any{"id": newID(), "ts": nowMs(), "state": "new", "kind": "sync", "feature": slug, "featureTitle": data["title"],
		"fid": "sync", "stage": 0, "stageTitle": "", "item": "Sync the dashboard with your latest status", "action": "do", "text": ""}
	return 200, map[string]any{"ok": true, "entry": publicOf(sendEntry(entry))}
}

// versionInfo tells the open page the files' version and whether it may be behind the agent.
func versionInfo(slug string) map[string]any {
	now := nowMs()
	st, err := os.Stat(filepath.Join(featuresDir(), slug, "feature.json"))
	if err != nil {
		return map[string]any{"v": "", "behind": nil, "pending": false}
	}
	mtime := st.ModTime().UnixMilli()
	ag := activeSessions()
	var behind any
	for _, s := range readSessions(now, true, false, false) {
		_, listed := ag[s.ID]
		if s.feature() != slug || s.State == "closed" || (len(ag) > 0 && !listed) {
			continue
		}
		if s.State == "wait" && s.T > mtime+30000 {
			name := ag.name(s.ID)
			if name == "" {
				name = fmt.Sprint(s.Project)
			}
			behind = map[string]any{"name": name, "min": int(math.RoundToEven(float64(s.T-mtime) / 60000))}
			break
		}
	}
	// the route of the open request ('active', 'bg', 'none'), or false: the page words its note by it
	var pending any = false
	asleep := ""
	for _, e := range inboxEntries() {
		if e["feature"] == slug && e["kind"] == "sync" && (e["state"] == "new" || e["state"] == "delivered") {
			e = unstick(e, now)
			pending = "active"
			if r, _ := e["route"].(string); r != "" {
				pending = r
			}
			if e["state"] == "new" && truthy(e["asleep"]) {
				pending, asleep = "asleep", str(e, "sessionName")
			}
			break
		}
	}
	return map[string]any{"v": featureVersion(slug), "behind": behind, "pending": pending, "asleep": asleep}
}

func inboxState() []any {
	now := nowMs()
	out := []any{}
	for _, e := range inboxEntries() {
		if (e["state"] == "done" || e["state"] == "failed") && now-num(e["ts"]) > 14*24*3600*1000 {
			for _, ext := range []string{"json", "lock"} {
				os.Remove(filepath.Join(inboxDir(), fmt.Sprintf("%v.%s", e["id"], ext)))
			}
			continue
		}
		out = append(out, publicOf(unstick(e, now)))
	}
	if len(out) > 200 {
		out = out[len(out)-200:]
	}
	return out
}

// featuresList is every feature for the bar on the left. Favorites first, then the features somebody works on
// now (busiest first), then the rest by name, finished ones last. "ask" says a session of the feature ended its
// turn and waits for the user ('reply', or 'permission' for a permission prompt).
func featuresList() map[string]any {
	now := nowMs()
	feats := collect(featuresDir())["features"].([]any)
	order := []string{"work", "bg", "wait", "idle"}
	idx := func(s string) int {
		for i, x := range order {
			if x == s {
				return i
			}
		}
		return -1
	}
	best := map[string]string{}
	type askT struct {
		kind string
		t    int64
	}
	ask := map[string]askT{}
	waiting := 0
	for _, s := range readSessions(now, false, false, false) {
		// The turn ended and nothing runs in the background: Claude wrote something and it is the user's move.
		// A minute later the hook calls the same session "idle", so idle counts too once it has said something.
		var said int64
		if s.State == "wait" || s.State == "idle" {
			if info := transcriptInfo(s.ID); info != nil {
				said = info.SaidT
			}
		}
		asks := s.State == "wait" || (said != 0 && s.What != "session started")
		if asks {
			waiting++
		}
		f := s.feature()
		if f != "" && idx(s.State) >= 0 {
			if cur, ok := best[f]; !ok || idx(s.State) < idx(cur) {
				best[f] = s.State
			}
		}
		if f != "" && asks {
			kind := "reply"
			if s.What == "waiting for permission" {
				kind = "permission"
			}
			if said == 0 {
				said = s.T
			}
			if a, ok := ask[f]; !ok || kind == "permission" || said > a.t {
				ask[f] = askT{kind, said}
			}
		}
	}
	fav := map[string]bool{}
	favs := favorites()
	for _, k := range favs {
		fav[k] = true
	}
	out := []map[string]any{}
	det, closed := detachedSet(), closedWorks()
	for _, x := range feats {
		f := x.(map[string]any)
		id := fmt.Sprint(f["id"])
		if closed[id] != 0 { // finished by the user: not in the bar any more
			continue
		}
		a := ask[id]
		project := "" // the first project the plan names: the bar can group the work by it
		if l, _ := f["projects"].([]any); len(l) > 0 {
			project = fmt.Sprint(l[0])
		}
		out = append(out, map[string]any{"id": id, "name": f["name"], "done": f["done"], "waiting": f["waiting"], "fav": fav["f:"+id],
			"state": best[id], "ask": a.kind, "saidT": a.t, "project": project, "days": f["days"], "detached": det[id]})
	}
	group := func(f map[string]any) int {
		switch {
		case f["fav"].(bool):
			return 0
		case f["state"] != "":
			return 1
		case f["done"].(bool):
			return 3
		}
		return 2
	}
	// Inside a group the order is by name. It used to follow what the sessions do (working first, then waiting…),
	// so the rows changed places by themselves every time a session started or finished a turn.
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if group(a) != group(b) {
			return group(a) < group(b)
		}
		return strings.ToLower(fmt.Sprint(a["name"])) < strings.ToLower(fmt.Sprint(b["name"]))
	})
	return map[string]any{"features": out, "waiting": waiting, "favorites": len(favs), "starting": spawnedNow(), "alerts": alertMode(), "awake": awakeNow()}
}

// ------------------------------------------------------------------ http

type handler struct {
	// local is true for the app's own window: its requests never come from the network, so the checks
	// that keep other web pages away from a port on 127.0.0.1 are not needed there.
	local bool
	port  int
}

// withSide puts the bar on the left into a live page before anything else, with its data: it is drawn in the
// first paint, so going from one feature to another does not flash and shift the page.
// A feature page also gets the first answers of its own polls (sessions, version) for the same reason.
// The data goes into <head> and the bar's code is inline: nothing in <body> waits for the network or holds the
// parser up, so WebKit (the app window) has no moment to show a half-built page.
const fontLink = `<link href="https://fonts.googleapis.com/css2?family=Geologica:wght@400;500;600;700&display=swap" rel="stylesheet">`

func withSide(html string, init map[string]any, local bool) string {
	data := "<script>window.WN_SIDE=" + blob(featuresList())
	if init != nil {
		data += ";window.WN_INIT=" + blob(init)
	}
	if local && termSupported { // the terminal exists in the app window only
		data += ";window.WN_TERM=1"
	}
	if local && soloSlug != "" { // a separate window of one work: no bar, a way back to the main window
		data += ";window.WN_SOLO=" + string(blob(soloSlug))
	}
	// A live page takes its font from this server. The stylesheet of Google Fonts held every page up until another
	// server had answered: 0.4 s of the 0.5 s a page took to appear (and much longer without a network).
	html = strings.Replace(html, fontLink, "<style>"+page("vendor/geologica.css")+"</style>", 1)
	html = strings.Replace(html, "</head>", data+";</script></head>", 1)
	html = strings.Replace(html, "<body>", "<body><script>"+page("side.js")+"</script>", 1)
	if local && termSupported {
		if i := strings.LastIndex(html, "</body>"); i >= 0 {
			html = html[:i] + "<script>" + page("term.js") + "</script>" + html[i:]
		}
	}
	return html
}

func send(w http.ResponseWriter, code int, body []byte, ctype string) {
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_, _ = w.Write(body)
}

// pickDir shows the system dialog for choosing a folder; set by the app window when it starts.
var pickDir func() (string, error)

func sendJSON(w http.ResponseWriter, code int, v any) {
	send(w, code, encodeJSON(v), "application/json; charset=utf-8")
}

func text(w http.ResponseWriter, code int, s string) {
	send(w, code, []byte(s), "text/plain; charset=utf-8")
}

func (h handler) hostsOK(r *http.Request) bool {
	if h.local {
		return true
	}
	host := r.Host
	if i := strings.LastIndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	if host != "127.0.0.1" && host != "localhost" {
		return false
	}
	o := r.Header.Get("Origin")
	return o == "" || o == fmt.Sprintf("http://127.0.0.1:%d", h.port) || o == fmt.Sprintf("http://localhost:%d", h.port)
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() { // keep the server alive whatever a data file contains
		if e := recover(); e != nil {
			text(w, 500, fmt.Sprintf("error: %v", e))
		}
	}()
	if !h.hostsOK(r) {
		text(w, 403, "forbidden")
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/api/upload" {
		h.upload(w, r)
		return
	}
	if r.Method == http.MethodPost {
		h.post(w, r)
		return
	}
	q := r.URL.Query()
	slugArg := func() (string, bool) {
		s := q.Get("feature")
		if !slugRe.MatchString(s) {
			sendJSON(w, 400, map[string]any{"error": "bad feature"})
			return "", false
		}
		return s, true
	}
	switch path := r.URL.Path; path {
	case "/":
		if h.local && soloSlug != "" { // the separate window of a work has only that work
			// not an HTTP redirect: the window (Wails) shows the answer of its first page as it is, "Found."
			send(w, 200, []byte(`<!doctype html><meta charset="utf-8"><script>location.replace("/f/`+soloSlug+`/")</script>`), "text/html; charset=utf-8")
			return
		}
		send(w, 200, []byte(withSide(indexHTML(stateJSON()), nil, h.local)), "text/html; charset=utf-8")
	case "/side.js":
		send(w, 200, []byte(page("side.js")), "text/javascript; charset=utf-8")
	case "/api/state":
		sendJSON(w, 200, stateJSON())
	case "/api/inbox":
		sendJSON(w, 200, map[string]any{"entries": inboxState()})
	case "/api/features":
		sendJSON(w, 200, featuresList())
	case "/api/chat":
		sid := q.Get("session")
		if !sidRe.MatchString(sid) {
			sendJSON(w, 400, map[string]any{"error": "bad session"})
			return
		}
		sendJSON(w, 200, chat(sid))
	case "/api/version":
		if s, ok := slugArg(); ok {
			sendJSON(w, 200, versionInfo(s))
		}
	case "/api/shots", "/shot": // the screenshots of a piece of work: the app window only
		if !h.local {
			text(w, 404, "not found")
			return
		}
		slug := q.Get("feature")
		if path == "/api/shots" {
			sendJSON(w, 200, map[string]any{"shots": listShots(slug), "supported": runtime.GOOS == "darwin"})
			return
		}
		p := shotFile(slug, q.Get("name"))
		if p == "" {
			text(w, 404, "not found")
			return
		}
		b, err := os.ReadFile(p)
		if err != nil {
			text(w, 404, "not found")
			return
		}
		ct := "image/png"
		if fileRe.MatchString(q.Get("name")) {
			ct = map[string]string{".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp", ".png": "image/png"}[strings.ToLower(filepath.Ext(p))]
			if ct == "" { // only pictures are shown in the page; anything else is a file to download, never run
				ct = "application/octet-stream"
				w.Header().Set("Content-Disposition", "attachment")
			}
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		w.WriteHeader(200)
		_, _ = w.Write(b)
	case "/api/workspaces": // the saved workspaces and the skills to pick from; ?from=<work> is the access of a work
		if f := q.Get("from"); f != "" {
			code, obj := wsFromWork(f)
			sendJSON(w, code, obj)
			return
		}
		sendJSON(w, 200, map[string]any{"workspaces": wsList(), "skills": wsSkills()})
	case "/api/finish": // what finishing a work does: which sessions stop, which cannot be stopped from here
		if s, ok := slugArg(); ok {
			code, obj := finishPlan(s)
			sendJSON(w, code, obj)
		}
	case "/api/targets": // the open sessions, for the "Message sessions" form
		sendJSON(w, 200, targets())
	case "/api/sessions":
		if s, ok := slugArg(); ok {
			sendJSON(w, 200, pickList(s))
		}
	case "/api/term/list", "/api/term/read", "/xterm.js", "/xterm.css", "/xterm-addon-fit.js", "/xterm-addon-web-links.js":
		if !h.local { // never on the 127.0.0.1 port
			text(w, 404, "not found")
			return
		}
		switch path {
		case "/api/term/list":
			sendJSON(w, 200, map[string]any{"terms": termList(), "supported": termSupported})
		case "/api/term/read":
			off, _ := strconv.ParseInt(q.Get("off"), 10, 64)
			code, obj := termRead(q.Get("id"), off)
			sendJSON(w, code, obj)
		case "/xterm.css":
			send(w, 200, []byte(page("vendor/xterm.css")), "text/css; charset=utf-8")
		default:
			send(w, 200, []byte(page("vendor"+path)), "text/javascript; charset=utf-8")
		}
	case "/demo/motion", "/demo/talk", "/demo/ask", "/demo/reply", "/demo/bar", "/demo/font": // examples of the interface to choose from (skill/demo)
		send(w, 200, []byte(page("demo"+strings.TrimPrefix(path, "/demo")+".html")), "text/html; charset=utf-8")
	case "/api/spawn": // what a new session would get: started from a feature, or in a folder
		if dir := q.Get("dir"); dir != "" {
			code, v := folderPlan(dir, "")
			sendJSON(w, code, v)
		} else if s, ok := slugArg(); ok {
			code, v := spawnPlan(s)
			sendJSON(w, code, v)
		}
	case "/api/panel": // the data of a feature page: the open page takes it when the plan changes, instead of loading again
		if s, ok := slugArg(); ok {
			src := filepath.Join(featuresDir(), s, "feature.json")
			if !isFile(src) {
				sendJSON(w, 404, map[string]any{"error": "unknown feature"})
				return
			}
			_, data, err := panelPage(src, "/", true)
			if err != nil {
				sendJSON(w, 500, map[string]any{"error": "feature.json: " + err.Error()})
				return
			}
			sendJSON(w, 200, data)
		}
	case "/api/access": // the Access tab: what the sessions of the feature may do and reach
		if s, ok := slugArg(); ok {
			sendJSON(w, 200, accessState(s))
		}
	case "/api/git": // the Git tab; the remote is asked too (the page already has the local state)
		if s, ok := slugArg(); ok {
			sendJSON(w, 200, gitState(s, true))
		}
	case "/api/route":
		if s, ok := slugArg(); ok {
			route, sess, name := planRoute(s)
			var id, project any
			asleep := false
			if sess != nil {
				id, project = sess.ID, sess.Project
				asleep = route == "active" && !awake(sess)
			} else {
				project = ""
			}
			sendJSON(w, 200, map[string]any{"route": route, "session": id, "name": name, "asleep": asleep, "project": project})
		}
	default:
		if fontRe.MatchString(path) { // the font of the pages, from the program itself: a page does not wait for another server
			if b, err := webFS.ReadFile("skill/vendor" + strings.TrimPrefix(path, "/font")); err == nil {
				w.Header().Set("Content-Type", "font/woff2")
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				w.WriteHeader(200)
				_, _ = w.Write(b)
				return
			}
		}
		if m := pageRe.FindStringSubmatch(path); m != nil {
			slug, _ := url.PathUnescape(m[1])
			src := filepath.Join(featuresDir(), slug, "feature.json")
			if slugRe.MatchString(slug) && isFile(src) {
				html, err := panelHTML(src, "/", true)
				if err != nil {
					text(w, 500, "feature.json: "+err.Error())
					return
				}
				send(w, 200, []byte(withSide(html, map[string]any{"ses": pickList(slug), "ver": versionInfo(slug), "inbox": map[string]any{"entries": inboxState()}, "git": gitStateFast(slug), "closed": closedWorks()[slug]}, h.local)), "text/html; charset=utf-8")
				return
			}
		}
		text(w, 404, "not found")
	}
}

// openLink opens a web address in the user's browser. Only http and https: nothing a page shows may start a program.
func openLink(raw string) (int, any) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(raw) > 4000 {
		return 400, map[string]any{"error": "not a web address"}
	}
	openFile(u.String())
	return 200, map[string]any{"ok": true}
}

// post takes the buttons. Anything on the web can make a browser POST to 127.0.0.1, so besides Host/Origin
// the body must be JSON and a custom header is required (a cross-site page cannot add it without a CORS
// preflight, which this server never allows).
func (h handler) post(w http.ResponseWriter, r *http.Request) {
	isTerm := r.URL.Path == "/api/term" && h.local // the terminal: the app window only
	isApp := r.URL.Path == "/api/app" && h.local   // what the app window tells the app about itself
	if r.URL.Path != "/api/act" && !isTerm && !isApp {
		text(w, 404, "not found")
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") || r.Header.Get("X-Whatnow") != "1" {
		sendJSON(w, 403, map[string]any{"error": "forbidden"})
		return
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil || len(b) == 0 || len(b) > maxBody {
		sendJSON(w, 400, map[string]any{"error": "bad body size"})
		return
	}
	body, ok := parseJSON(b).(map[string]any)
	if !ok {
		sendJSON(w, 400, map[string]any{"error": "bad json"})
		return
	}
	if isApp {
		switch str(body, "op") {
		case "focus": // the window says whether it is in front (for the alerts)
			appFocus(num(body["on"]) == 1)
			sendJSON(w, 200, map[string]any{"ok": true})
		case "alerts":
			code, obj := setAlertMode(str(body, "mode"))
			sendJSON(w, code, obj)
		case "detach": // the work in a window of its own (or that window to the front)
			code, obj := detach(str(body, "feature"))
			sendJSON(w, code, obj)
		case "reattach": // from the main window: the separate window of the work closes, the work is shown here
			code, obj := reattach(str(body, "feature"))
			sendJSON(w, code, obj)
		case "back": // in a separate window: back to the main window
			if soloSlug == "" {
				sendJSON(w, 400, map[string]any{"error": "not a separate window"})
				return
			}
			code, obj := backToMain(soloSlug)
			sendJSON(w, code, obj)
		case "shot": // a screenshot for the agent: the system selection of a part of the screen
			name, err := takeShot(str(body, "feature"))
			if err != nil {
				sendJSON(w, 500, map[string]any{"error": err.Error()})
				return
			}
			res := map[string]any{"name": name}
			if name != "" {
				res["path"] = filepath.Join(shotsDir(str(body, "feature")), name)
			}
			sendJSON(w, 200, res)
		case "shotdel":
			if p := shotFile(str(body, "feature"), str(body, "name")); p != "" {
				_ = os.Remove(p)
			}
			sendJSON(w, 200, map[string]any{"ok": true})
		case "pickfiles": // the system dialog for files to attach to a message; they are kept with the work
			if pickFiles == nil {
				sendJSON(w, 501, map[string]any{"error": "no file dialog here"})
				return
			}
			paths, err := pickFiles()
			if err != nil {
				sendJSON(w, 500, map[string]any{"error": err.Error()})
				return
			}
			names, err := attachPaths(str(body, "feature"), paths)
			res := map[string]any{"names": names}
			if err != nil {
				res["error"] = err.Error()
			}
			sendJSON(w, 200, res)
		case "pickdir": // the system dialog for choosing a folder (the app window has one, a browser tab does not)
			if pickDir == nil {
				sendJSON(w, 501, map[string]any{"error": "no folder dialog here"})
				return
			}
			dir, err := pickDir()
			if err != nil {
				sendJSON(w, 500, map[string]any{"error": err.Error()})
				return
			}
			sendJSON(w, 200, map[string]any{"dir": dir})
		default:
			sendJSON(w, 400, map[string]any{"error": "bad op"})
		}
		return
	}
	if isTerm {
		n := func(k string) int { return int(num(body[k])) }
		code, obj := 400, any(map[string]any{"error": "bad op"})
		switch str(body, "op") {
		case "open":
			code, obj = termOpen(str(body, "session"), n("cols"), n("rows"), n("running") == 1)
		case "write":
			code, obj = termWrite(str(body, "id"), str(body, "data"))
		case "resize":
			code, obj = termResize(str(body, "id"), n("cols"), n("rows"))
		case "close":
			code, obj = termClose(str(body, "id"))
		case "link": // a link clicked in the window: the window cannot leave the dashboard, the browser opens it
			code, obj = openLink(str(body, "url"))
		}
		sendJSON(w, code, obj)
		return
	}
	code, obj := act(body)
	sendJSON(w, code, obj)
}

// listen serves the pages to a browser on 127.0.0.1 as well. Returns an error when the port is taken.
func listen(p int) error {
	srv := &http.Server{Addr: fmt.Sprintf("127.0.0.1:%d", p), Handler: handler{port: p, local: debugLocal}, ReadHeaderTimeout: 10 * time.Second}
	go warmGit() // so that the first opening of a feature page does not wait for git
	go awakeResume()
	return srv.ListenAndServe()
}

var _ = bufio.NewReader // keeps the import list stable while the file grows

// upload takes a file dropped on (or pasted into) the window of a session: the raw bytes, its name in ?name=.
// The app window only (as screenshots), with the header no other web page can add.
func (h handler) upload(w http.ResponseWriter, r *http.Request) {
	if !h.local || r.Header.Get("X-Whatnow") != "1" {
		sendJSON(w, 403, map[string]any{"error": "forbidden"})
		return
	}
	q := r.URL.Query()
	name, err := saveAttach(q.Get("feature"), q.Get("name"), r.Body)
	if err != nil {
		sendJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	sendJSON(w, 200, map[string]any{"name": name, "path": filepath.Join(shotsDir(q.Get("feature")), name)})
}
