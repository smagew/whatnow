package main

// Settings and the small files shared with the Python skill (common.py). The two must read and write the
// same files in the same way: the Python hook and the agents' scripts keep writing them while the app runs.

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const defaultPort = 7777

func userHome() string {
	h, _ := os.UserHomeDir()
	return h
}

func expandUser(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		return filepath.Join(userHome(), p[1:])
	}
	return p
}

func home() string {
	if v := os.Getenv("WHATNOW_HOME"); v != "" {
		return v
	}
	return filepath.Join(userHome(), ".whatnow")
}

// readJSON returns the parsed file, or nil when it is missing or broken. Numbers stay json.Number,
// so an integer written by Python comes back out exactly as it was.
func readJSON(path string) any {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return parseJSON(b)
}

func parseJSON(b []byte) any {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var v any
	if d.Decode(&v) != nil {
		return nil
	}
	return v
}

func readMap(path string) map[string]any {
	m, _ := readJSON(path).(map[string]any)
	if m == nil {
		m = map[string]any{}
	}
	return m
}

func encodeJSON(v any) []byte {
	var buf bytes.Buffer
	e := json.NewEncoder(&buf)
	e.SetEscapeHTML(false)
	_ = e.Encode(v)
	return bytes.TrimRight(buf.Bytes(), "\n")
}

func atomicWriteJSON(path string, v any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".tmp-")
	if err != nil {
		return err
	}
	if _, err = f.Write(encodeJSON(v)); err == nil {
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

func config() map[string]any { return readMap(filepath.Join(home(), "config.json")) }

func featuresDir() string {
	raw := os.Getenv("FEATURES_DIR")
	if raw == "" {
		raw, _ = config()["featuresDir"].(string)
	}
	if raw == "" {
		raw = filepath.Join(home(), "features")
	}
	p, err := filepath.Abs(expandUser(raw))
	if err != nil {
		return raw
	}
	return p
}

func port() int {
	switch v := config()["port"].(type) {
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i)
		}
	case string:
		var i int
		if _, err := fmt.Sscan(v, &i); err == nil {
			return i
		}
	}
	return defaultPort
}

func sessionsDir() string { return filepath.Join(home(), "sessions") }
func inboxDir() string    { return filepath.Join(home(), "inbox") }

func claudeHome() string {
	if v := os.Getenv("CLAUDE_CONFIG_DIR"); v != "" {
		return v
	}
	return filepath.Join(userHome(), ".claude")
}

// claudeBin finds the claude executable. An app started from the Finder has a short PATH, so besides
// $PATH a few usual install folders are tried. "claudeBin" in config.json wins.
func claudeBin() string {
	if c, _ := config()["claudeBin"].(string); c != "" {
		if _, err := os.Stat(expandUser(c)); err == nil {
			return expandUser(c)
		}
	}
	if p, err := exec.LookPath("claude"); err == nil {
		return p
	}
	for _, c := range []string{"~/.local/bin/claude", "~/.claude/local/claude", "/usr/local/bin/claude", "/opt/homebrew/bin/claude"} {
		if _, err := os.Stat(expandUser(c)); err == nil {
			return expandUser(c)
		}
	}
	return ""
}

func childEnv(claude string) []string {
	env := os.Environ()
	path := strings.Join([]string{filepath.Dir(claude), "/usr/local/bin", "/usr/bin", "/bin", os.Getenv("PATH")}, string(os.PathListSeparator))
	out := []string{"PATH=" + path}
	hasHome := false
	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			continue
		}
		if strings.HasPrefix(kv, "HOME=") {
			hasHome = true
		}
		out = append(out, kv)
	}
	if !hasHome {
		out = append(out, "HOME="+userHome())
	}
	return out
}

func strList(path string) []string {
	var out []string
	if l, ok := readJSON(path).([]any); ok {
		for _, x := range l {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func hidden() map[string]bool {
	out := map[string]bool{}
	for _, s := range strList(filepath.Join(home(), "hidden.json")) {
		out[s] = true
	}
	return out
}

func setHidden(sid string, on bool) error {
	h := hidden()
	if on {
		h[sid] = true
	} else {
		delete(h, sid)
	}
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return atomicWriteJSON(filepath.Join(home(), "hidden.json"), keys)
}

// favorites are keys of starred things: "s:<session id>" and "f:<feature slug>".
func favorites() []string {
	l := strList(filepath.Join(home(), "favorites.json"))
	if l == nil {
		l = []string{}
	}
	return l
}

func setFavorite(key string, on bool) error {
	cur, out := favorites(), []string{}
	found := false
	for _, k := range cur {
		if k == key {
			found = true
			if !on {
				continue
			}
		}
		out = append(out, k)
	}
	if on && !found {
		out = append(out, key)
	}
	return atomicWriteJSON(filepath.Join(home(), "favorites.json"), out)
}

// bindings says which session works on which feature: {session id: feature slug}.
func bindings() map[string]any { return readMap(filepath.Join(home(), "bindings.json")) }

func bind(sid, slug string) error {
	if sid == "" || slug == "" {
		return nil
	}
	b := bindings()
	if b[sid] == slug {
		return nil
	}
	b[sid] = slug
	return atomicWriteJSON(filepath.Join(home(), "bindings.json"), b)
}

// featureVersion changes whenever the feature's files change; the open page compares it to know it must reload.
func featureVersion(slug string) string {
	base := filepath.Join(featuresDir(), slug)
	var out []string
	for _, name := range []string{"feature.json", "overrides.json"} {
		if st, err := os.Stat(filepath.Join(base, name)); err == nil {
			out = append(out, fmt.Sprint(st.ModTime().UnixNano()))
		} else {
			out = append(out, "0")
		}
	}
	return strings.Join(out, ":")
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// watcherAlive is true while the session's --watch hook runs, the only thing that can wake an idle session.
func watcherAlive(sid string) bool {
	b, err := os.ReadFile(filepath.Join(home(), "watch", unsafeName.ReplaceAllString(sid, "_")+".pid"))
	if err != nil {
		return false
	}
	pid := strings.TrimSpace(string(b))
	if _, err := fmt.Sscan(pid, new(int)); err != nil {
		return false
	}
	out, err := exec.Command("ps", "-p", pid, "-o", "command=").Output()
	if err != nil {
		return false
	}
	return isWatcherCmd(string(out))
}

// factID is the stable id of one fact: it survives reordering and changes only when the text changes.
func factID(stage any, text any) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("%v|%v", stage, text)))
	return hex.EncodeToString(sum[:])[:10]
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func nowMs() int64 { return time.Now().UnixMilli() }

// ------------------------------------------------------------------ inbox (messages from the page)

var actions = map[string]string{"do": "Task: do this item", "ask": "Question: answer the user in this session"}

// formatMessage is the text Claude sees when a message from the page is delivered.
func formatMessage(e map[string]any) string {
	get := func(k string) string {
		if v, ok := e[k]; ok && v != nil {
			return fmt.Sprint(v)
		}
		return ""
	}
	or := func(a, b string) string {
		if a != "" {
			return a
		}
		return b
	}
	switch get("kind") {
	case "session":
		return strings.Join([]string{
			"[whatnow] Message from the dashboard.",
			"User note: " + get("text"),
			"Answer it in this conversation. When you are done, mark the message resolved: " +
				cliPath + " inbox resolve " + get("id") + ` "one-line summary".`,
		}, "\n")
	case "sync":
		return strings.Join([]string{
			fmt.Sprintf(`[whatnow] Sync request for feature "%s".`, or(get("featureTitle"), get("feature"))),
			"The dashboard may differ from what you last told the user. Compare the feature.json of this feature with " +
				"your latest status: every open item you listed (and who has to act on it), what is done, what was verified, " +
				`and what git really shows (merged branches, commits). Fix what differs, set "who" on every blocker, ` +
				"rebuild the dashboard (" + cliPath + " render --index) and mark the message resolved: " +
				cliPath + " inbox resolve " + get("id") + ` "what you changed, or: already in sync".`,
		}, "\n")
	}
	act := actions[get("action")]
	if act == "" {
		act = get("action")
	}
	return strings.Join([]string{
		fmt.Sprintf(`[whatnow] Message from the dashboard for feature "%s".`, or(get("featureTitle"), get("feature"))),
		fmt.Sprintf("Stage %s: %s", get("stage"), get("stageTitle")),
		"Item: " + get("item"),
		"Action: " + act,
		"User note: " + or(get("text"), "none"),
		fmt.Sprintf(`When you are done: update the feature.json of feature "%s", rebuild the dashboard `+
			"(%s render --index) and mark the message resolved: "+
			`%s inbox resolve %s "one-line summary".`, get("feature"), cliPath, cliPath, get("id")),
	}, "\n")
}

func inboxEntries() []map[string]any {
	var out []map[string]any
	names, err := os.ReadDir(inboxDir())
	if err != nil {
		return out
	}
	for _, n := range names {
		if !strings.HasSuffix(n.Name(), ".json") {
			continue
		}
		e := readMap(filepath.Join(inboxDir(), n.Name()))
		if s, _ := e["id"].(string); s != "" {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return num(out[i]["ts"]) < num(out[j]["ts"]) })
	return out
}

func saveEntry(e map[string]any) error {
	return atomicWriteJSON(filepath.Join(inboxDir(), fmt.Sprint(e["id"])+".json"), e)
}

// takeLock creates the delivery lock of an entry; false when somebody else holds it already.
func takeLock(id string) bool {
	f, err := os.OpenFile(filepath.Join(inboxDir(), id+".lock"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// ------------------------------------------------------------------ small helpers for parsed JSON

func num(v any) int64 {
	switch x := v.(type) {
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		f, _ := x.Float64()
		return int64(f)
	case float64:
		return int64(x)
	case int64:
		return x
	case int:
		return int64(x)
	}
	return 0
}

// intOf is the value of an integer literal; a float, a string or anything else is not one.
func intOf(v any) (int64, bool) {
	n, ok := v.(json.Number)
	if !ok || strings.ContainsAny(string(n), ".eE") {
		return 0, false
	}
	i, err := n.Int64()
	return i, err == nil
}

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case json.Number:
		return x != "0"
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	}
	return true
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

// runes cuts a string to n characters (not bytes), as Python's s[:n] does.
func runes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
