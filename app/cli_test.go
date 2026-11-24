package main

// Ported from tests/test_whatnow.py (ConfigTest, AutostartTest), tests/test_robustness.py (HooksInstallTest) and
// tests/test_inbox.py (InboxCliTest, HooksInstallTest).

import (
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// ------------------------------------------------------------------ ConfigTest

func TestDefaultsDoNotDependOnTheMachine(t *testing.T) {
	e := isolate(t)
	os.Remove(e.config)
	t.Setenv("FEATURES_DIR", "")
	eq(t, featuresDir(), filepath.Join(e.home, "features"))
	eq(t, port(), 7777)
}

func TestConfigFileAndEnvOverride(t *testing.T) {
	e := isolate(t)
	t.Setenv("FEATURES_DIR", "")
	e.writeConfig(map[string]any{"featuresDir": "/data/feats", "port": 8123})
	want, _ := filepath.Abs("/data/feats")
	eq(t, featuresDir(), want)
	eq(t, port(), 8123)
	t.Setenv("FEATURES_DIR", "/other")
	want, _ = filepath.Abs("/other")
	eq(t, featuresDir(), want)
}

func TestBrokenConfigFallsBackToDefaults(t *testing.T) {
	e := isolate(t)
	writeFile(t, e.config, []byte("{broken"))
	eq(t, port(), 7777)
}

// ------------------------------------------------------------------ AutostartTest

// plistValue reads one value of a property list: dict, array, string, true/false.
func plistValue(d *xml.Decoder, start xml.StartElement) (any, error) {
	switch start.Name.Local {
	case "dict":
		m := map[string]any{}
		key := ""
		for {
			tok, err := d.Token()
			if err != nil {
				return nil, err
			}
			switch x := tok.(type) {
			case xml.StartElement:
				if x.Name.Local == "key" {
					var k string
					if err := d.DecodeElement(&k, &x); err != nil {
						return nil, err
					}
					key = k
					continue
				}
				v, err := plistValue(d, x)
				if err != nil {
					return nil, err
				}
				m[key] = v
			case xml.EndElement:
				return m, nil
			}
		}
	case "array":
		l := []any{}
		for {
			tok, err := d.Token()
			if err != nil {
				return nil, err
			}
			switch x := tok.(type) {
			case xml.StartElement:
				v, err := plistValue(d, x)
				if err != nil {
					return nil, err
				}
				l = append(l, v)
			case xml.EndElement:
				return l, nil
			}
		}
	case "true", "false":
		if err := d.Skip(); err != nil {
			return nil, err
		}
		return start.Name.Local == "true", nil
	default:
		var s string
		err := d.DecodeElement(&s, &start)
		return s, err
	}
}

func parsePlist(text string) (map[string]any, error) {
	d := xml.NewDecoder(strings.NewReader(text))
	for {
		tok, err := d.Token()
		if err != nil {
			return nil, err
		}
		if s, ok := tok.(xml.StartElement); ok && s.Name.Local == "dict" {
			v, err := plistValue(d, s)
			if err != nil {
				return nil, err
			}
			return v.(map[string]any), nil
		}
	}
}

func TestPlistIsValidAndPortable(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("autostartText writes a launchd plist only on macOS")
	}
	text := autostartText([]string{"/w/whatnow", "serve", "7777"}, "/h/server.log", [][2]string{{"WHATNOW_HOME", "/h"}})
	d, err := parsePlist(text)
	must(t, err)
	eq(t, d["Label"], any("whatnow.server"))
	eq(t, d["RunAtLoad"], any(true))
	eq(t, d["KeepAlive"], any(true))
	args, _ := d["ProgramArguments"].([]any)
	eqList(t, args, []any{"/w/whatnow", "serve", "7777"})
	env, _ := d["EnvironmentVariables"].(map[string]any)
	if len(env) != 1 || env["WHATNOW_HOME"] != "/h" {
		t.Errorf("EnvironmentVariables: %v", env)
	}
	eq(t, d["StandardOutPath"], any("/h/server.log"))
}

func TestSystemdUnit(t *testing.T) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		t.Skip("autostartText writes a systemd unit only on Linux and other unix systems")
	}
	unit := autostartText([]string{"/w/whatnow", "/path with space/x", "serve", "7777"}, "/h/server.log", nil)
	contains(t, unit, `ExecStart="/w/whatnow" "/path with space/x" "serve" "7777"`)
	contains(t, unit, "Restart=on-failure")
	contains(t, unit, "WantedBy=default.target")
	if strings.Contains(unit, "Environment=") {
		t.Error("Environment= without environment")
	}
}

func TestNoMachineSpecificPathInTheSource(t *testing.T) {
	files, err := filepath.Glob("*.go")
	must(t, err)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src := readText(t, f)
		for _, bad := range []string{"/Users/", "/www/"} {
			if strings.Contains(src, bad) {
				t.Errorf("%s contains %q", f, bad)
			}
		}
	}
}

// ------------------------------------------------------------------ HooksInstallTest (test_robustness.py)

func TestFileToolsReachTheHook(t *testing.T) {
	// autoBind links a session that edits feature.json; that needs the edit events, not only Bash
	pt, _ := desiredHooks().get("PostToolUse")
	var matchers []string
	for _, g := range pt.([]any) {
		m, _ := g.(*omap).get("matcher")
		s, _ := m.(string)
		matchers = append(matchers, s)
	}
	tools := []string{"Bash"}
	for k := range writeTools {
		tools = append(tools, k)
	}
	for _, tool := range tools {
		hit := false
		for _, m := range matchers {
			hit = hit || regexp.MustCompile(`^(?:`+m+`)$`).MatchString(tool)
		}
		if !hit {
			t.Errorf("%s does not reach the hook", tool)
		}
	}
}

func TestTheExampleInDocsIsCurrent(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "docs", "hooks-settings.example.json"))
	must(t, err)
	doc, err := decodeOrdered(b)
	must(t, err)
	want := &omap{}
	want.set("hooks", desiredHooks())
	eq(t, string(encodeOrdered(doc, "")), string(encodeOrdered(want, "")))
}

// ------------------------------------------------------------------ HooksInstallTest (test_inbox.py)

func settings(t *testing.T, text string) *omap {
	t.Helper()
	v, err := decodeOrdered([]byte(text))
	must(t, err)
	return v.(*omap)
}

func groups(d *omap, event string) []any {
	hv, _ := d.get("hooks")
	l, _ := hv.(*omap).get(event)
	return asList(l)
}

func firstHook(g any) *omap {
	hs, _ := g.(*omap).get("hooks")
	return asList(hs)[0].(*omap)
}

func commandOf(g any) string {
	c, _ := firstHook(g).get("command")
	return c.(string)
}

func TestInstallIsIdempotentAndKeepsOtherHooks(t *testing.T) {
	base := `{"theme": "x", "hooks": {"Stop": [{"hooks": [{"type": "command", "command": "echo mine"}]}]}}`
	once := settings(t, base)
	installHooks(once)
	twice := settings(t, string(encodeOrdered(once, "")))
	installHooks(twice)
	eq(t, string(encodeOrdered(once, "")), string(encodeOrdered(twice, "")))
	theme, _ := once.get("theme")
	eq(t, theme, any("x"))
	var stop []string
	ours := 0
	for _, g := range groups(once, "Stop") {
		stop = append(stop, commandOf(g))
		if strings.Contains(commandOf(g), "whatnow/whatnow hook") {
			ours++
		}
	}
	contains(t, strings.Join(stop, "\n"), "echo mine")
	eq(t, ours, 3, "state hook, watcher, nudge")
}

func TestWakeAndDeliverEntries(t *testing.T) {
	d := settings(t, "{}")
	installHooks(d)
	var watch, deliver *omap
	for _, g := range groups(d, "Stop") {
		if strings.Contains(commandOf(g), "--watch") {
			watch = firstHook(g)
		}
	}
	for _, g := range groups(d, "UserPromptSubmit") {
		if strings.Contains(commandOf(g), "--deliver") {
			deliver = firstHook(g)
		}
	}
	if watch == nil || deliver == nil {
		t.Fatal("no --watch or --deliver entry")
	}
	rw, _ := watch.get("asyncRewake")
	eq(t, rw, any(true))
	if _, ok := deliver.get("async"); ok {
		t.Error("--deliver is async: it must be synchronous to reach the prompt")
	}
}

func TestUpgradeFromTheOldSetOnlyAdds(t *testing.T) {
	d := settings(t, `{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "python3 ~/.claude/skills/whatnow/hook.py", "async": true}]}]}}`)
	installHooks(d)
	eq(t, len(groups(d, "Stop")), 3)
	for _, g := range groups(d, "Stop") {
		if strings.Contains(commandOf(g), "hook.py") {
			t.Error("the old hook.py entry was kept, not replaced")
		}
	}
}

func TestUninstallRemovesOnlyOurs(t *testing.T) {
	mine := `{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "echo mine"}]}]}}`
	d := settings(t, mine)
	installHooks(d)
	uninstallHooks(d)
	eq(t, string(encodeOrdered(d, "")), string(encodeOrdered(settings(t, mine), "")))
}

// ------------------------------------------------------------------ InboxCliTest

func quiet(t *testing.T, f func() int) int {
	t.Helper()
	r, w, err := os.Pipe()
	must(t, err)
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w
	done := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, r); close(done) }()
	rc := f()
	os.Stdout, os.Stderr = oldOut, oldErr
	w.Close()
	<-done
	r.Close()
	return rc
}

func TestResolve(t *testing.T) {
	e := withDemo(t)
	e.saveEntry(map[string]any{"id": "abc123def456", "ts": 1, "state": "delivered", "feature": "f", "item": "x", "action": "do"})
	eq(t, quiet(t, func() int { return runInbox([]string{"resolve", "abc123def456", "done: checked"}) }), 0)
	en := readMap(filepath.Join(inboxDir(), "abc123def456.json"))
	eq(t, en["state"], any("done"))
	eq(t, en["note"], any("done: checked"))
	if quiet(t, func() int { return runInbox([]string{"resolve", "../../x"}) }) == 0 {
		t.Error("a bad id was accepted")
	}
}
