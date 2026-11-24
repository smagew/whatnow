package main

// The commands an agent or the user runs: render, inbox, hooks, autostart. They replace render.py, inbox.py,
// hooks_install.py and autostart.py and read and write the same files.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// cliPath is how agents and hooks call this program: install.sh puts it next to the skill.
const cliPath = "~/.claude/skills/whatnow/whatnow"

const usage = `usage:
  whatnow                               open the window
  whatnow serve [port]                  the pages for a browser on 127.0.0.1, no window
  whatnow render <feature.json> [--out file] [--open]
  whatnow render --index [features dir] [--open]
  whatnow inbox list [--all]
  whatnow inbox resolve <id> "one line summary"
  whatnow hook [--deliver|--watch|--nudge]   (called by Claude Code, reads the event on stdin)
  whatnow hooks install|uninstall|print [--file settings.json] [--dry-run]
  whatnow autostart install|uninstall|status [--dry-run]`

func die(code int, format string, a ...any) int {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	return code
}

func flag(args []string, name string) bool {
	for _, a := range args {
		if a == name {
			return true
		}
	}
	return false
}

func flagValue(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// positional drops --flags and the values of the flags that take one.
func positional(args []string, valued ...string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--") {
			for _, v := range valued {
				if a == v {
					i++
				}
			}
			continue
		}
		out = append(out, a)
	}
	return out
}

func openFile(path string) {
	cmd := "xdg-open"
	if runtime.GOOS == "darwin" {
		cmd = "open"
	}
	_ = exec.Command(cmd, path).Start()
}

// ------------------------------------------------------------------ render

func writePanel(src, out, backHref string) (map[string]any, error) {
	html, data, err := panelPage(src, backHref, false)
	if err != nil {
		return nil, err
	}
	return data, os.WriteFile(out, []byte(html), 0o644)
}

func runRender(args []string) int {
	pos := positional(args, "--out")
	if flag(args, "--index") {
		root := featuresDir()
		if len(pos) > 0 {
			root, _ = filepath.Abs(pos[0])
		}
		if !isDir(root) {
			return die(1, "features folder not found: %s", root)
		}
		payload := collectFiles(root, true)
		out := filepath.Join(root, "index.html")
		if err := os.WriteFile(out, []byte(indexHTML(payload)), 0o644); err != nil {
			return die(1, "%v", err)
		}
		fmt.Println(out)
		if flag(args, "--open") {
			openFile(out)
		}
		if len(payload["broken"].([]any)) > 0 {
			return 1
		}
		return 0
	}
	if len(pos) == 0 {
		return die(2, "%s", usage)
	}
	src, _ := filepath.Abs(pos[0])
	out := filepath.Join(filepath.Dir(src), "panel.html")
	if o := flagValue(args, "--out"); o != "" {
		out, _ = filepath.Abs(o)
	}
	if _, err := writePanel(src, out, ""); err != nil {
		return die(1, "feature.json: %v", err)
	}
	fmt.Println(out)
	if flag(args, "--open") {
		openFile(out)
	}
	return 0
}

// ------------------------------------------------------------------ inbox

var inboxID = regexp.MustCompile(`^[0-9a-f]{8,32}$`)

func runInbox(args []string) int {
	if len(args) > 0 && args[0] == "list" {
		show := func(e map[string]any, k string) string {
			if v, ok := e[k]; ok && v != nil {
				return fmt.Sprint(v)
			}
			return "None"
		}
		for _, e := range inboxEntries() {
			if flag(args, "--all") || e["state"] == "new" || e["state"] == "delivered" {
				item, _ := e["item"].(string)
				fmt.Printf("%s  %-9s %-6s %s: %s\n", e["id"], show(e, "state"), show(e, "action"), show(e, "feature"), runes(item, 70))
			}
		}
		return 0
	}
	if len(args) >= 2 && args[0] == "resolve" {
		if !inboxID.MatchString(args[1]) {
			return die(1, "bad id")
		}
		e := readMap(filepath.Join(inboxDir(), args[1]+".json"))
		if len(e) == 0 {
			return die(1, "no such message: %s", args[1])
		}
		e["state"], e["note"], e["doneAt"] = "done", runes(strings.Join(args[2:], " "), 600), nowMs()
		if err := saveEntry(e); err != nil {
			return die(1, "%v", err)
		}
		fmt.Println("resolved " + args[1])
		return 0
	}
	return die(2, "%s", usage)
}

// ------------------------------------------------------------------ hooks in ~/.claude/settings.json

// Our entries are recognised by their command: the Python hook of older installs too, so install replaces it.
func ourCommand(c string) bool {
	return strings.Contains(c, "whatnow/hook.py") || strings.Contains(c, "whatnow/whatnow hook")
}

const hookTools = "Bash|Edit|Write|MultiEdit|NotebookEdit"

type hookEntry struct {
	event, matcher, mode string
	extra                []kv
}

func hookEntries() []hookEntry {
	async := []kv{{"async", true}}
	watch := []kv{{"asyncRewake", true}, {"timeout", json.Number("3600")}}
	return []hookEntry{
		{"SessionStart", "", "", async},
		{"SessionStart", "", "--watch", watch},
		{"UserPromptSubmit", "", "", async},
		{"UserPromptSubmit", "", "--deliver", []kv{{"timeout", json.Number("10")}}},
		{"PostToolUse", hookTools, "", async},
		{"PostToolUseFailure", "Bash", "", async},
		{"Stop", "", "", async},
		{"Stop", "", "--watch", watch},
		{"Stop", "", "--nudge", []kv{{"asyncRewake", true}, {"timeout", json.Number("30")}}},
		{"Notification", "", "", async},
		{"SessionEnd", "", "", async},
	}
}

// desiredHooks is the hooks block, event by event, in the order above.
func desiredHooks() *omap {
	out := &omap{}
	for _, e := range hookEntries() {
		cmd := cliPath + " hook"
		if e.mode != "" {
			cmd += " " + e.mode
		}
		h := &omap{}
		h.set("type", "command")
		h.set("command", cmd)
		for _, x := range e.extra {
			h.set(x.k, x.v)
		}
		g := &omap{}
		if e.matcher != "" {
			g.set("matcher", e.matcher)
		}
		g.set("hooks", []any{h})
		l, _ := out.get("hooks_" + e.event)
		out.set("hooks_"+e.event, append(asList(l), g))
	}
	res := &omap{}
	for _, k := range out.keys {
		v, _ := out.get(k)
		res.set(strings.TrimPrefix(k, "hooks_"), v)
	}
	return res
}

func asList(v any) []any { l, _ := v.([]any); return l }

func isOurs(g any) bool {
	m, ok := g.(*omap)
	if !ok {
		return false
	}
	hs, _ := m.get("hooks")
	for _, h := range asList(hs) {
		if hm, ok := h.(*omap); ok {
			c, _ := hm.get("command")
			if s, _ := c.(string); ourCommand(s) {
				return true
			}
		}
	}
	return false
}

func installHooks(d *omap) {
	hv, ok := d.get("hooks")
	hooks, isMap := hv.(*omap)
	if !ok || !isMap {
		hooks = &omap{}
		d.set("hooks", hooks)
	}
	want := desiredHooks()
	for _, event := range want.keys {
		w, _ := want.get(event)
		cur, _ := hooks.get(event)
		var mine, other []any
		for _, g := range asList(cur) {
			if isOurs(g) {
				mine = append(mine, g)
			} else {
				other = append(other, g)
			}
		}
		if string(encodeOrdered(mine, "")) != string(encodeOrdered(w, "")) {
			hooks.set(event, append(append([]any{}, other...), asList(w)...))
		}
	}
}

func uninstallHooks(d *omap) {
	hv, _ := d.get("hooks")
	hooks, ok := hv.(*omap)
	if !ok {
		return
	}
	for _, event := range append([]string{}, hooks.keys...) {
		cur, _ := hooks.get(event)
		keep := []any{}
		for _, g := range asList(cur) {
			if !isOurs(g) {
				keep = append(keep, g)
			}
		}
		if len(keep) == 0 {
			hooks.del(event)
		} else {
			hooks.set(event, keep)
		}
	}
	if len(hooks.keys) == 0 {
		d.del("hooks")
	}
}

func runHooks(args []string) int {
	op := ""
	if len(args) > 0 {
		op = args[0]
	}
	if op == "print" {
		d := &omap{}
		d.set("hooks", desiredHooks())
		fmt.Println(string(encodeOrdered(d, "  ")))
		return 0
	}
	if op != "install" && op != "uninstall" {
		return die(2, "%s", usage)
	}
	path := expandUser("~/.claude/settings.json")
	if f := flagValue(args, "--file"); f != "" {
		path = expandUser(f)
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		raw, err = []byte("{}\n"), nil
	}
	if err != nil {
		return die(1, "%v", err)
	}
	data, err := decodeOrdered(raw)
	if err != nil {
		return die(1, "%s is not valid JSON, nothing changed: %v", path, err)
	}
	d, ok := data.(*omap)
	if !ok {
		return die(1, "%s: top level must be an object, nothing changed", path)
	}
	before := string(encodeOrdered(d, "  ")) + "\n"
	if op == "install" {
		installHooks(d)
	} else {
		uninstallHooks(d)
	}
	text := string(encodeOrdered(d, "  ")) + "\n"
	if text == before {
		fmt.Println("no changes needed")
		return 0
	}
	fmt.Print(lineDiff(before, text))
	if flag(args, "--dry-run") {
		fmt.Println("(dry run, nothing written)")
		return 0
	}
	if len(raw) > 0 && isFile(path) {
		bak := path + ".bak-whatnow-" + time.Now().Format("20060102-150405")
		if err := os.WriteFile(bak, raw, 0o600); err != nil {
			return die(1, "backup failed, nothing changed: %v", err)
		}
		fmt.Println("backup: " + bak)
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if err := atomicWrite(path, []byte(text)); err != nil {
		return die(1, "%v", err)
	}
	fmt.Println("written: " + path)
	return 0
}

// lineDiff shows the lines that differ (removed with "-", added with "+"), with the common head and tail cut.
func lineDiff(a, b string) string {
	x, y := strings.SplitAfter(a, "\n"), strings.SplitAfter(b, "\n")
	i := 0
	for i < len(x) && i < len(y) && x[i] == y[i] {
		i++
	}
	j := 0
	for j < len(x)-i && j < len(y)-i && x[len(x)-1-j] == y[len(y)-1-j] {
		j++
	}
	var out strings.Builder
	for _, l := range x[i : len(x)-j] {
		out.WriteString("-" + l)
	}
	for _, l := range y[i : len(y)-j] {
		out.WriteString("+" + l)
	}
	return out.String()
}

// ------------------------------------------------------------------ autostart of `whatnow serve`

const (
	launchLabel = "whatnow.server"
	unitName    = "whatnow.service"
)

func autostartPath() string {
	switch runtime.GOOS {
	case "darwin":
		return expandUser("~/Library/LaunchAgents/" + launchLabel + ".plist")
	case "linux":
		return expandUser("~/.config/systemd/user/" + unitName)
	}
	return ""
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

func autostartText(cmd []string, log string, env [][2]string) string {
	if runtime.GOOS == "darwin" {
		var b strings.Builder
		b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
`)
		if len(env) > 0 {
			b.WriteString("\t<key>EnvironmentVariables</key>\n\t<dict>\n")
			for _, e := range env {
				fmt.Fprintf(&b, "\t\t<key>%s</key>\n\t\t<string>%s</string>\n", xmlEscape(e[0]), xmlEscape(e[1]))
			}
			b.WriteString("\t</dict>\n")
		}
		fmt.Fprintf(&b, "\t<key>KeepAlive</key>\n\t<true/>\n\t<key>Label</key>\n\t<string>%s</string>\n", launchLabel)
		b.WriteString("\t<key>ProcessType</key>\n\t<string>Background</string>\n\t<key>ProgramArguments</key>\n\t<array>\n")
		for _, c := range cmd {
			fmt.Fprintf(&b, "\t\t<string>%s</string>\n", xmlEscape(c))
		}
		fmt.Fprintf(&b, "\t</array>\n\t<key>RunAtLoad</key>\n\t<true/>\n\t<key>StandardErrorPath</key>\n\t<string>%s</string>\n", xmlEscape(log))
		fmt.Fprintf(&b, "\t<key>StandardOutPath</key>\n\t<string>%s</string>\n</dict>\n</plist>\n", xmlEscape(log))
		return b.String()
	}
	q := func(s string) string { return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"` }
	var quoted []string
	for _, c := range cmd {
		quoted = append(quoted, q(c))
	}
	lines := []string{"[Unit]", "Description=whatnow server (feature status page)", "",
		"[Service]", "ExecStart=" + strings.Join(quoted, " "), "Restart=on-failure", "RestartSec=5",
		"StandardOutput=append:" + log, "StandardError=append:" + log}
	for _, e := range env {
		lines = append(lines, "Environment="+q(e[0]+"="+e[1]))
	}
	return strings.Join(append(lines, "", "[Install]", "WantedBy=default.target", ""), "\n")
}

func serverAlive() bool {
	c := http.Client{Timeout: 2 * time.Second}
	r, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/api/state", port()))
	if err != nil {
		return false
	}
	r.Body.Close()
	return r.StatusCode == 200
}

func runQuiet(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s failed: %s", name, strings.Join(args, " "), bytes.TrimSpace(out))
	}
	return nil
}

func runAutostart(args []string) int {
	op := ""
	if len(args) > 0 {
		op = args[0]
	}
	path := autostartPath()
	switch op {
	case "status":
		state := "absent"
		if path != "" && isFile(path) {
			state = "exists"
		}
		yes := map[bool]string{true: "yes", false: "no"}
		fmt.Printf("platform: %s\nautostart file: %s (%s)\nserver answers on port %d: %s\n", runtime.GOOS, path, state, port(), yes[serverAlive()])
		return 0
	case "install":
		if path == "" {
			return die(1, "Autostart is not supported on %s. Start the server by hand: whatnow serve", runtime.GOOS)
		}
		exe, err := os.Executable()
		if err == nil {
			exe, err = filepath.EvalSymlinks(exe)
		}
		if err != nil {
			return die(1, "cannot find this program: %v", err)
		}
		_ = os.MkdirAll(home(), 0o755)
		log := filepath.Join(home(), "server.log")
		var env [][2]string
		for _, k := range []string{"WHATNOW_HOME", "FEATURES_DIR"} {
			if v := os.Getenv(k); v != "" {
				env = append(env, [2]string{k, v})
			}
		}
		text := autostartText([]string{exe, "serve", strconv.Itoa(port())}, log, env)
		fmt.Printf("file: %s\n%s", path, text)
		if flag(args, "--dry-run") {
			fmt.Println("(dry run, nothing written)")
			return 0
		}
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if runtime.GOOS == "darwin" { // stop the old one first, so the port is free for the new one
			_ = runQuiet("launchctl", "bootout", fmt.Sprintf("gui/%d/%s", os.Getuid(), launchLabel))
		}
		if serverAlive() {
			fmt.Printf("note: something already answers on port %d (a server started by hand?). Stop it, "+
				"otherwise the autostarted one cannot bind the port.\n", port())
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			return die(1, "%v", err)
		}
		if runtime.GOOS == "darwin" {
			err = runQuiet("launchctl", "bootstrap", fmt.Sprintf("gui/%d", os.Getuid()), path)
		} else if err = runQuiet("systemctl", "--user", "daemon-reload"); err == nil {
			err = runQuiet("systemctl", "--user", "enable", "--now", unitName)
		}
		if err != nil {
			return die(1, "%v", err)
		}
		fmt.Printf("installed. Page: http://127.0.0.1:%d/   log: %s\n", port(), log)
		return 0
	case "uninstall":
		if path == "" {
			return die(1, "Autostart is not supported on %s.", runtime.GOOS)
		}
		if runtime.GOOS == "darwin" {
			_ = runQuiet("launchctl", "bootout", fmt.Sprintf("gui/%d/%s", os.Getuid(), launchLabel))
		} else {
			_ = runQuiet("systemctl", "--user", "disable", "--now", unitName)
		}
		if isFile(path) {
			os.Remove(path)
			fmt.Println("removed " + path)
		} else {
			fmt.Println("was not installed")
		}
		if runtime.GOOS == "linux" {
			_ = runQuiet("systemctl", "--user", "daemon-reload")
		}
		return 0
	}
	return die(2, "%s", usage)
}
