package main

// Ported from tests/test_whatnow.py (HookTest, BindingTest), tests/test_robustness.py (HookClassifyTest) and
// tests/test_inbox.py (ClaimTest, HookDeliveryTest). The hook runs in process (runHook with stdin/stdout/stderr
// replaced), which checks the same exit codes and output as the subprocess did.

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ------------------------------------------------------------------ HookTest

type hookT struct {
	*testEnv
	repo string
}

func newHookT(t *testing.T) *hookT {
	e := isolate(t)
	h := &hookT{testEnv: e, repo: filepath.Join(t.TempDir(), "repo")}
	gitInit(t, h.repo)
	return h
}

func (h *hookT) send(p map[string]any) {
	h.t.Helper()
	if _, ok := p["session_id"]; !ok {
		p["session_id"] = "s1"
	}
	if _, ok := p["cwd"]; !ok {
		p["cwd"] = h.repo
	}
	rc, out, errOut := h.hook(nil, p)
	eq(h.t, rc, 0, "exit code")
	eq(h.t, out+errOut, "", "a hook must be silent")
}

func (h *hookT) bash(cmd string, ok bool, out string) {
	if ok {
		h.send(map[string]any{"hook_event_name": "PostToolUse", "tool_name": "Bash", "tool_input": map[string]any{"command": cmd},
			"tool_response": map[string]any{"stdout": out}})
	} else {
		h.send(map[string]any{"hook_event_name": "PostToolUseFailure", "tool_name": "Bash", "tool_input": map[string]any{"command": cmd},
			"error": out})
	}
}

func (h *hookT) events() []map[string]any {
	f, err := os.Open(filepath.Join(h.home, "events.jsonl"))
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []map[string]any
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		out = append(out, parseJSON(sc.Bytes()).(map[string]any))
	}
	return out
}

func (h *hookT) state() any { return readMap(filepath.Join(h.home, "sessions", "s1.json"))["state"] }

func TestFilterKeepsOnlyGitAndTests(t *testing.T) {
	h := newHookT(t)
	h.bash("ls -la", true, "")
	h.bash("git status", true, "")
	eq(t, len(h.events()), 0)
	h.bash(`git commit -m "Add thing"`, true, "")
	h.bash("git push -u origin x", true, "")
	h.bash("vendor/bin/phpunit", true, "OK (214 tests, 900 assertions)")
	h.bash("vendor/bin/phpunit", false, "Exit code 1\nTests: 214, Assertions: 900, Failures: 2.")
	ev := h.events()
	var kinds []string
	for _, e := range ev {
		kinds = append(kinds, fmt.Sprintf("%v/%v", e["kind"], e["ok"]))
	}
	eqList(t, kinds, []string{"commit/true", "push/true", "tests/true", "tests/false"})
	if len(ev) == 4 {
		contains(t, ev[0]["text"].(string), "Add thing")
		eq(t, ev[2]["text"], any("tests passed: 214"))
		eq(t, ev[3]["text"], any("tests failed: 2 of 214"))
		eq(t, ev[0]["branch"], any("feature/demo-feature"))
	}
}

func TestPythonUnittestOutputOnStderr(t *testing.T) {
	h := newHookT(t)
	h.send(map[string]any{"hook_event_name": "PostToolUse", "tool_name": "Bash", "tool_input": map[string]any{"command": "python3 -m unittest discover -s tests"},
		"tool_response": map[string]any{"stdout": "", "stderr": "Ran 5 tests in 2.1s\n\nOK\n"}})
	h.send(map[string]any{"hook_event_name": "PostToolUseFailure", "tool_name": "Bash", "tool_input": map[string]any{"command": "python3 -m unittest"},
		"error": "Exit code 1\nRan 5 tests in 1s\n\nFAILED (failures=2)"})
	var texts []string
	for _, e := range h.events() {
		texts = append(texts, e["text"].(string))
	}
	eqList(t, texts, []string{"tests passed: 5", "tests failed: 2 of 5"})
}

func TestStateMachine(t *testing.T) {
	h := newHookT(t)
	h.send(map[string]any{"hook_event_name": "UserPromptSubmit", "prompt": "secret"})
	eq(t, h.state(), any("work"))
	h.send(map[string]any{"hook_event_name": "Notification", "notification_type": "permission_prompt"})
	eq(t, h.state(), any("wait"))
	h.bash("echo hi", true, "") // work resumed after approval
	eq(t, h.state(), any("work"))
	h.send(map[string]any{"hook_event_name": "Stop"})
	eq(t, h.state(), any("wait"))
	h.send(map[string]any{"hook_event_name": "Notification", "notification_type": "idle_prompt"})
	eq(t, h.state(), any("idle"))
	h.send(map[string]any{"hook_event_name": "SessionEnd"})
	eq(t, h.state(), any("closed"), "kept: the page can resume it later")
}

func TestPromptTextIsNeverStored(t *testing.T) {
	h := newHookT(t)
	h.send(map[string]any{"hook_event_name": "UserPromptSubmit", "prompt": "very secret prompt"})
	var blob strings.Builder
	must(t, filepath.Walk(h.home, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			b, _ := os.ReadFile(p)
			blob.Write(b)
		}
		return nil
	}))
	if strings.Contains(blob.String(), "very secret prompt") {
		t.Error("the prompt was stored")
	}
}

func TestNeverFailsOnGarbage(t *testing.T) {
	h := newHookT(t)
	for _, bad := range []string{"", "not json", "[]", `{"hook_event_name": 5}`} {
		rc, out, errOut := h.hook(nil, bad)
		eq(t, fmt.Sprint(rc, out, errOut), fmt.Sprint(0, "", ""), bad)
	}
}

// ------------------------------------------------------------------ BindingTest

// Python's bind returned True/False for "changed"; Go's returns an error. "Nothing changed" is checked by the
// file not being written again.
func TestBindAndFind(t *testing.T) {
	e := isolate(t)
	eq(t, featureOf("s1"), "")
	must(t, bind("s1", "csv-export"))
	eq(t, featureOf("s1"), "csv-export")
	path := filepath.Join(e.home, "bindings.json")
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	must(t, os.Chtimes(path, old, old))
	must(t, bind("s1", "csv-export")) // nothing changed: not written
	if st, _ := os.Stat(path); !st.ModTime().Equal(old) {
		t.Error("bindings.json was written although nothing changed")
	}
	must(t, bind("s1", "reports")) // the last feature wins
	eq(t, featureOf("s1"), "reports")
	must(t, bind("", "x"))
	if _, ok := bindings()[""]; ok {
		t.Error("an empty session id was bound")
	}
}

// ------------------------------------------------------------------ HookClassifyTest

func runBash(cmd, out string) (string, bool, string) {
	return classify(jsMap(map[string]any{"tool_input": map[string]any{"command": cmd}, "tool_response": map[string]any{"stdout": out}}), false)
}

func TestAPipeDoesNotHideFailedTests(t *testing.T) {
	check := func(cmd, out, kind string, ok bool, text string) {
		t.Helper()
		k, o, x := runBash(cmd, out)
		eq(t, fmt.Sprint(k, o, x), fmt.Sprint(kind, ok, text), cmd)
	}
	check("pytest -q | tail -5", "=== 3 failed, 10 passed in 1.2s ===", "tests", false, "tests failed: 3 of 13")
	check("npx jest | tail", "Tests:       1 failed, 5 passed, 6 total", "tests", false, "tests failed: 1 of 6")
	check("pytest", "=== 2 errors in 0.31s ===", "tests", false, "tests failed: 2")
	check("pytest -q", "=== 10 passed in 1.2s ===", "tests", true, "tests passed: 10")
}

func TestMentioningATestToolIsNotATestRun(t *testing.T) {
	for _, cmd := range []string{"pip install pytest", "grep -rn unittest docs/", "cat pytest.ini", "npm install --save-dev jest",
		"ls | grep vitest", `echo "run pytest later"`, "uv add pytest"} {
		k, o, x := runBash(cmd, "")
		eq(t, fmt.Sprint(k, o, x), fmt.Sprint("", false, ""), cmd)
	}
}

func TestRealRunsAreStillSeen(t *testing.T) {
	for _, cmd := range []string{"pytest", "python3 -m pytest -q", "cd api && CI=1 vendor/bin/phpunit", "python3 -m unittest discover -s tests",
		"npm test", "npx vitest run", "go test ./...", "docker compose exec app phpunit", "(cd web && yarn test)"} {
		k, _, _ := runBash(cmd, "")
		eq(t, k, "tests", cmd)
	}
}

func TestOnlyOurOwnWatcherIsEverSignalled(t *testing.T) {
	if isWatcher(os.Getpid()) {
		t.Error("the test process is taken for a watcher")
	}
	if isWatcher(1<<22 + 12345) {
		t.Error("a pid that does not exist is taken for a watcher")
	}
}

// ------------------------------------------------------------------ ClaimTest

func claimEntry(e *testEnv, more map[string]any) {
	m := map[string]any{"id": "abc123def456", "ts": 1, "state": "new", "feature": "demo-feature", "featureTitle": "Demo",
		"stage": 1, "stageTitle": "Design", "item": "Not done", "action": "do", "text": "go"}
	for k, v := range more {
		m[k] = v
	}
	e.saveEntry(m)
}

func TestClaimIsExclusiveAndMarksDelivered(t *testing.T) {
	e := withDemo(t)
	claimEntry(e, map[string]any{"session": "s1"})
	eq(t, len(claim("s2", "demo-feature")), 0, "addressed to someone else")
	eq(t, len(claim("s1", "demo-feature")), 1)
	eq(t, len(claim("s1", "demo-feature")), 0, "never twice")
	eq(t, readMap(filepath.Join(inboxDir(), "abc123def456.json"))["state"], any("delivered"))
}

func TestUnaddressedMessageGoesToASessionOfTheFeature(t *testing.T) {
	e := withDemo(t)
	claimEntry(e, nil)
	eq(t, len(claim("s9", "other-feature")), 0)
	eq(t, len(claim("s9", "demo-feature")), 1)
}

func TestParallelClaimsDeliverOnce(t *testing.T) {
	e := withDemo(t)
	claimEntry(e, nil)
	var wg sync.WaitGroup
	var mu sync.Mutex
	total := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n := len(claim("s9", "demo-feature"))
			mu.Lock()
			total += n
			mu.Unlock()
		}()
	}
	wg.Wait()
	eq(t, total, 1)
}

// ------------------------------------------------------------------ HookDeliveryTest

type deliveryT struct {
	*testEnv
	repo    string
	payload map[string]any
}

func newDeliveryT(t *testing.T) *deliveryT {
	e := withDemo(t)
	d := &deliveryT{testEnv: e, repo: filepath.Join(t.TempDir(), "api")}
	gitInit(t, d.repo)
	claimEntry(e, nil)
	must(t, bind("s1", "demo-feature"))
	d.payload = map[string]any{"session_id": "s1", "cwd": d.repo, "hook_event_name": "UserPromptSubmit"}
	return d
}

func (d *deliveryT) with(kv ...any) map[string]any {
	p := map[string]any{}
	for k, v := range d.payload {
		p[k] = v
	}
	for i := 0; i+1 < len(kv); i += 2 {
		p[kv[i].(string)] = kv[i+1]
	}
	return p
}

func (d *deliveryT) run(mode string, p map[string]any) (int, string, string) {
	return d.hook([]string{mode}, p)
}

func (d *deliveryT) stop(sid string) int {
	rc, _, _ := d.run("--nudge", map[string]any{"session_id": sid, "cwd": d.repo, "hook_event_name": "Stop"})
	return rc
}

func (d *deliveryT) prompt(sid string) {
	d.run("x", map[string]any{"session_id": sid, "cwd": d.repo, "hook_event_name": "UserPromptSubmit"})
}

func (d *deliveryT) ageFeature() {
	old := time.Now().Add(-600 * time.Second)
	must(d.t, os.Chtimes(d.demoSrc(), old, old))
}

func TestDeliverPrintsTheMessageOnce(t *testing.T) {
	d := newDeliveryT(t)
	rc, out, _ := d.run("--deliver", d.payload)
	eq(t, rc, 0)
	contains(t, out, `[whatnow] Message from the dashboard for feature "Demo"`)
	contains(t, out, "inbox resolve abc123def456") // Python: "inbox.py resolve"; the Go CLI is "whatnow inbox resolve"
	_, out, _ = d.run("--deliver", d.payload)
	eq(t, out, "")
}

func TestDeliverIgnoresASessionNotLinkedToTheFeature(t *testing.T) {
	d := newDeliveryT(t)
	_, out, _ := d.run("--deliver", d.with("session_id", "s2")) // same repo and branch, but not linked
	eq(t, out, "")
}

func TestWorkingOnTheFeatureFilesLinksTheSessionButReadingDoesNot(t *testing.T) {
	d := newDeliveryT(t)
	path := d.demoSrc()
	cases := []struct {
		sid, tool string
		input     map[string]any
		want      string
	}{
		{"e1", "Edit", map[string]any{"file_path": path}, "demo-feature"},
		{"e2", "Write", map[string]any{"file_path": path}, "demo-feature"},
		{"e3", "Bash", map[string]any{"command": fmt.Sprintf(`python3 ~/.claude/skills/whatnow/render.py "%s" --open`, path)}, "demo-feature"},
		{"e4", "Bash", map[string]any{"command": fmt.Sprintf(`~/.claude/skills/whatnow/whatnow render "%s" --open`, path)}, "demo-feature"},
		{"r0", "Bash", map[string]any{"command": fmt.Sprintf("cp %s /tmp/x/ && python3 render.py /tmp/x/feature.json", path)}, ""},
		{"r1", "Read", map[string]any{"file_path": path}, ""},
		{"r2", "Bash", map[string]any{"command": "cat " + path}, ""},
		{"r3", "Bash", map[string]any{"command": fmt.Sprintf("ls %s/demo-feature", d.feats)}, ""},
		{"r4", "Edit", map[string]any{"file_path": "/tmp/other/feature.json"}, ""},
	}
	for _, c := range cases {
		d.run("x", map[string]any{"session_id": c.sid, "cwd": d.repo, "hook_event_name": "PostToolUse", "tool_name": c.tool, "tool_input": c.input})
		eq(t, featureOf(c.sid), c.want, c.sid)
	}
}

func TestNudgeRemindsOnceWhenTheTurnLeftTheDashboardUntouched(t *testing.T) {
	d := newDeliveryT(t)
	d.ageFeature()
	d.prompt("s1")
	rc, _, errOut := d.run("--nudge", map[string]any{"session_id": "s1", "cwd": d.repo, "hook_event_name": "Stop"})
	eq(t, rc, 2)
	contains(t, errOut, "without updating its dashboard")
	eq(t, d.stop("s1"), 0, "same prompt: never twice, so no loop")
	time.Sleep(5 * time.Millisecond) // a new prompt has a new time
	d.prompt("s1")
	eq(t, d.stop("s1"), 0, "the reminder's own wake-up is a prompt: cooldown")
	writeFile(t, filepath.Join(d.home, "nudged", "s1"), []byte(fmt.Sprintf("1 %f", float64(time.Now().Unix()-3600))))
	d.prompt("s1")
	eq(t, d.stop("s1"), 2, "after the cooldown a new prompt may be reminded")
}

func TestNudgeNeverChainsFromAStopHookTurn(t *testing.T) {
	d := newDeliveryT(t)
	d.ageFeature()
	d.prompt("s1")
	rc, _, _ := d.run("--nudge", map[string]any{"session_id": "s1", "cwd": d.repo, "hook_event_name": "Stop", "stop_hook_active": true})
	eq(t, rc, 0)
}

func TestNudgeIsSilentWhenTheFileWasUpdatedOrTheSessionIsNotLinked(t *testing.T) {
	d := newDeliveryT(t)
	d.ageFeature()
	d.prompt("s1")
	now := time.Now()
	must(t, os.Chtimes(d.demoSrc(), now, now)) // updated during the turn
	eq(t, d.stop("s1"), 0)
	d.ageFeature()
	d.prompt("s2")
	eq(t, d.stop("s2"), 0, "s2 is not linked to a feature")
}

func TestNudgeCanBeTurnedOff(t *testing.T) {
	d := newDeliveryT(t)
	d.ageFeature()
	d.writeConfig(map[string]any{"nudge": false})
	d.prompt("s1")
	eq(t, d.stop("s1"), 0)
}

func TestTakingAMessageLinksTheSession(t *testing.T) {
	d := newDeliveryT(t)
	d.run("--deliver", d.with("session_id", "s9"))
	eq(t, featureOf("s9"), "", "not linked: nothing is delivered to it")
	eq(t, inboxEntries()[0]["state"], any("new"))
	must(t, bind("s9", "demo-feature"))
	d.run("--deliver", d.with("session_id", "s9"))
	eq(t, inboxEntries()[0]["state"], any("delivered"))
}

func TestWatcherWakesTheSessionWithExitCode2(t *testing.T) {
	d := newDeliveryT(t)
	t.Setenv("WHATNOW_WATCH_POLL", "0.2")
	rc, _, errOut := d.run("--watch", d.with("hook_event_name", "Stop"))
	eq(t, rc, 2)
	contains(t, errOut, "abc123def456")
}

func TestWatcherExitsQuietlyForASessionWithoutAFeature(t *testing.T) {
	d := newDeliveryT(t)
	t.Setenv("WHATNOW_WATCH_SECONDS", "0.5")
	t.Setenv("WHATNOW_WATCH_POLL", "0.2")
	rc, _, errOut := d.run("--watch", d.with("session_id", "unlinked", "hook_event_name", "Stop"))
	eq(t, rc, 0)
	eq(t, errOut, "", "nothing is addressed to it")
}

func TestAMessageAddressedToAnUnlinkedSessionWakesIt(t *testing.T) {
	d := newDeliveryT(t)
	d.saveEntry(map[string]any{"id": "aaa111bbb222", "ts": 2, "state": "new", "kind": "session", "feature": "", "fid": "session",
		"stage": 0, "item": "Message to a session", "action": "ask", "text": "hello there", "session": "unlinked"})
	t.Setenv("WHATNOW_WATCH_POLL", "0.2")
	rc, _, errOut := d.run("--watch", d.with("session_id", "unlinked", "hook_event_name", "Stop"))
	eq(t, rc, 2)
	contains(t, errOut, "hello there")
	if strings.Contains(errOut, "Stage") {
		t.Errorf("a message to a session mentions a stage: %q", errOut)
	}
}

func TestWatcherWaitsThenGivesUpAtItsDeadline(t *testing.T) {
	d := newDeliveryT(t)
	must(t, os.Remove(filepath.Join(inboxDir(), "abc123def456.json")))
	t.Setenv("WHATNOW_WATCH_POLL", "0.2")
	t.Setenv("WHATNOW_WATCH_SECONDS", "1")
	start := time.Now()
	rc, _, _ := d.run("--watch", d.with("hook_event_name", "Stop"))
	eq(t, rc, 0)
	if time.Since(start) < time.Second {
		t.Errorf("gave up after %v, before its deadline", time.Since(start))
	}
}

func TestWatcherPicksUpAMessageThatArrivesLater(t *testing.T) {
	d := newDeliveryT(t)
	must(t, os.Remove(filepath.Join(inboxDir(), "abc123def456.json")))
	t.Setenv("WHATNOW_WATCH_POLL", "0.2")
	t.Setenv("WHATNOW_WATCH_SECONDS", "20")
	type result struct {
		rc  int
		err string
	}
	done := make(chan result, 1)
	p := d.with("hook_event_name", "Stop")
	go func() {
		rc, _, errOut := d.run("--watch", p)
		done <- result{rc, errOut}
	}()
	time.Sleep(time.Second)
	d.saveEntry(map[string]any{"id": "ffff00001111", "ts": 2, "state": "new", "feature": "demo-feature", "featureTitle": "Demo",
		"stage": 1, "stageTitle": "Design", "item": "X", "action": "ask", "text": "why?"})
	r := <-done
	eq(t, r.rc, 2)
	contains(t, r.err, "ffff00001111")
}
