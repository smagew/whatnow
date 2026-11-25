package main

// Ported from tests/test_inbox.py (OverridesTest, ValidationTest, RoutingTest, AttachTest, BlockerItemTest,
// SyncTest, SessionsTabTest, FavoriteTest, HttpSecurityTest) and tests/test_robustness.py (AgentsCacheTest).
//
// Python replaced server functions with stubs; here the same situations are made for real:
//   - active_sessions: the cache of `claude agents` is filled (and a fake claude prints the same list);
//   - awake = True: a fake --watch process is registered for the session (wake), removed by unwake;
//   - resume_in_background: the fake claude answers `--resume` and logs the session id;
//   - agents_at: the time of the cached list is set;
//   - UNSTICK_AFTER = -1: the message is made older than unstickMs.

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func act2(t *testing.T, body map[string]any) (int, map[string]any) {
	t.Helper()
	code, obj := act(jsMap(body))
	m, _ := obj.(map[string]any)
	return code, m
}

// ------------------------------------------------------------------ OverridesTest

func TestSkipIsAppliedAndReversible(t *testing.T) {
	e := withDemo(t)
	code, _ := act2(t, map[string]any{"feature": "demo-feature", "fid": e.fid(1), "action": "skip", "text": "already checked"})
	eq(t, code, 200)
	st := e.loadDemo()["stages"].([]any)[0].(map[string]any)
	eq(t, st["facts"].([]any)[1].([]any)[0], any("n"))
	eq(t, st["reasons"].(map[string]any)["1"], any("already checked"))
	eq(t, st["state"], any("ok"), "nothing left undone")
	// feature.json itself is never touched
	raw := readMap(e.demoSrc())
	eq(t, raw["stages"].([]any)[0].(map[string]any)["facts"].([]any)[1].([]any)[0], any("q"))
	act2(t, map[string]any{"feature": "demo-feature", "fid": e.fid(1), "action": "unskip"})
	st = e.loadDemo()["stages"].([]any)[0].(map[string]any)
	eq(t, st["facts"].([]any)[1].([]any)[0], any("q"))
	eq(t, st["state"], any("part"))
}

func TestAFinishedItemCannotBeSkipped(t *testing.T) {
	e := withDemo(t)
	code, _ := act2(t, map[string]any{"feature": "demo-feature", "fid": e.fid(0), "action": "skip"})
	eq(t, code, 400)
}

func TestDecisionFollowsTheTextNotThePosition(t *testing.T) {
	e := withDemo(t)
	act2(t, map[string]any{"feature": "demo-feature", "fid": e.fid(1), "action": "skip", "text": "r"})
	d := readMap(e.demoSrc())
	st := d["stages"].([]any)[0].(map[string]any)
	st["facts"] = append([]any{[]any{"y", "A new first fact"}}, st["facts"].([]any)...) // positions shift
	writeFile(t, e.demoSrc(), encodeJSON(d))
	var kinds []any
	for _, f := range e.loadDemo()["stages"].([]any)[0].(map[string]any)["facts"].([]any) {
		kinds = append(kinds, f.([]any)[0])
	}
	eqList(t, kinds, []any{"y", "y", "n"})
}

// ------------------------------------------------------------------ ValidationTest

func TestRejectsBadInput(t *testing.T) {
	e := withDemo(t)
	for _, bad := range []map[string]any{{"feature": "../etc"}, {"feature": "nope-feature"}, {"fid": "zzzz"}, {"fid": "abcdef0123"},
		{"action": "rm -rf"}, {"text": strings.Repeat("x", 2001)}} {
		body := map[string]any{"feature": "demo-feature", "fid": e.fid(1), "action": "ask", "text": "why?"}
		for k, v := range bad {
			body[k] = v
		}
		code, _ := act2(t, body)
		if code != 400 && code != 404 {
			t.Errorf("%v: got %d", bad, code)
		}
	}
}

// ------------------------------------------------------------------ RoutingTest

func routeSend(t *testing.T, e *testEnv, action string) map[string]any {
	t.Helper()
	code, obj := act2(t, map[string]any{"feature": "demo-feature", "fid": e.fid(1), "action": action, "text": "please"})
	eq(t, code, 200)
	return entryOf(t, obj)
}

func TestOpenSessionGetsItAddressed(t *testing.T) {
	e := withDemo(t)
	e.session("11111111-aaaa", "idle", "demo-feature")
	e.wake("11111111-aaaa")
	e.setAgents(map[string]map[string]any{"11111111-aaaa": {"name": "api-xy", "status": "idle"}})
	en := routeSend(t, e, "do")
	eq(t, fmt.Sprint(en["route"], en["state"], en["session"], en["sessionName"]), fmt.Sprint("active", "new", "11111111-aaaa", "api-xy"))
	eq(t, len(e.fakeLines("resumed")), 0, "nothing resumed in background")
}

func TestOpenSessionThatCannotBeWoken(t *testing.T) {
	// open, but idle for longer than its watcher lives: nothing can wake it
	e := withDemo(t)
	e.session("11111111-aaaa", "idle", "demo-feature")
	e.setAgents(map[string]map[string]any{"11111111-aaaa": {"name": "api-xy", "status": "idle"}})
	en := routeSend(t, e, "do")
	_, hasSession := en["session"]
	eq(t, fmt.Sprint(en["route"], en["state"], hasSession, en["asleep"], en["sessionName"]), fmt.Sprint("active", "new", false, true, "api-xy"))
	eq(t, versionInfo("demo-feature")["pending"], any(false), "not a sync request")
	// so it is left to the feature: any of its sessions that turns up takes it
	must(t, bind("22222222-bbbb", "demo-feature"))
	eqList(t, ids(claim("22222222-bbbb", "demo-feature")), []string{en["id"].(string)})
}

// ageEntries makes every inbox message older than unstickMs (Python set UNSTICK_AFTER = -1).
func ageEntries(t *testing.T) {
	for _, en := range inboxEntries() {
		en["ts"] = nowMs() - unstickMs - 1000
		must(t, saveEntry(en))
	}
}

func TestMessageStuckOnASleepingSessionIsHandedToTheFeature(t *testing.T) {
	e := withDemo(t)
	e.session("11111111-aaaa", "idle", "demo-feature")
	e.wake("11111111-aaaa")
	e.setAgents(map[string]map[string]any{"11111111-aaaa": {"name": "api-xy", "status": "idle"}})
	_, obj := act2(t, map[string]any{"feature": "demo-feature", "action": "sync"})
	en := entryOf(t, obj)
	eq(t, en["session"], any("11111111-aaaa"), "it was awake then")
	e.unwake("11111111-aaaa") // ... and fell asleep
	eq(t, versionInfo("demo-feature")["pending"], any("active"), "too early to give up")
	ageEntries(t)
	info := versionInfo("demo-feature")
	eq(t, fmt.Sprint(info["pending"], info["asleep"]), fmt.Sprint("asleep", "api-xy"))
	if _, ok := inboxEntries()[0]["session"]; ok {
		t.Error("the stuck message is still addressed to the sleeping session")
	}
	if _, err := os.Stat(filepath.Join(inboxDir(), en["id"].(string)+".lock")); err == nil {
		t.Error("the lock of the message was left behind")
	}
	// a session that can take it is attached: the request goes to it by name on the next poll
	e.session("22222222-bbbb", "work", "demo-feature")
	e.setAgents(map[string]map[string]any{"11111111-aaaa": {"name": "api-xy"}, "22222222-bbbb": {"name": "api-new"}})
	ageEntries(t)
	info = versionInfo("demo-feature")
	eq(t, fmt.Sprint(info["pending"], inboxEntries()[0]["session"]), fmt.Sprint("active", "22222222-bbbb"))
	eqList(t, ids(claim("22222222-bbbb", "")), []string{en["id"].(string)})
}

func TestStatusRequestCanBeCancelled(t *testing.T) {
	e := withDemo(t)
	e.session("11111111-aaaa", "idle", "demo-feature")
	e.wake("11111111-aaaa")
	e.setAgents(map[string]map[string]any{"11111111-aaaa": {"name": "api-xy"}})
	_, obj := act2(t, map[string]any{"feature": "demo-feature", "action": "sync"})
	first := entryOf(t, obj)
	code, res := act2(t, map[string]any{"feature": "demo-feature", "action": "unsync"})
	eq(t, fmt.Sprint(code, res["ok"], res["cancelled"]), fmt.Sprint(200, true, 1))
	eq(t, versionInfo("demo-feature")["pending"], any(false))
	eq(t, len(claim("11111111-aaaa", "demo-feature")), 0, "it never arrives later")
	_, obj = act2(t, map[string]any{"feature": "demo-feature", "action": "sync"})
	if entryOf(t, obj)["id"] == first["id"] {
		t.Error("a new request after a cancelled one got the old id")
	}
}

func TestClosedSessionIsResumedInBackgroundWithTheMessage(t *testing.T) {
	e := withDemo(t)
	e.session("22222222-bbbb", "closed", "demo-feature")
	e.setAgents(map[string]map[string]any{})
	en := routeSend(t, e, "do")
	eq(t, fmt.Sprint(en["route"], en["state"]), fmt.Sprint("bg", "delivered"))
	eqList(t, e.fakeLines("resumed"), []string{"22222222-bbbb"})
}

func TestAFailedResumeIsReported(t *testing.T) {
	e := withDemo(t)
	e.session("22222222-bbbb", "closed", "demo-feature")
	e.setAgents(map[string]map[string]any{})
	e.fakeSet("resume.out", "Workspace not trusted")
	e.fakeSet("resume.code", "1")
	en := routeSend(t, e, "do")
	eq(t, fmt.Sprint(en["state"], "|", en["error"]), fmt.Sprint("failed", "|", "Workspace not trusted"))
}

func TestNoKnownSessionWaits(t *testing.T) {
	e := withDemo(t)
	e.setAgents(map[string]map[string]any{})
	en := routeSend(t, e, "ask")
	eq(t, fmt.Sprint(en["route"], en["state"]), fmt.Sprint("none", "new"))
}

func TestSessionOfAnotherFeatureIsNotUsed(t *testing.T) {
	e := withDemo(t)
	e.session("33333333-cccc", "idle", "")
	e.wake("33333333-cccc")
	e.setAgents(map[string]map[string]any{"33333333-cccc": {"name": "x"}})
	eq(t, routeSend(t, e, "do")["route"], any("none"))
}

// ------------------------------------------------------------------ AttachTest

func withAttach(t *testing.T) *testEnv {
	e := withDemo(t)
	e.setAgents(map[string]map[string]any{"44444444-dddd": {"name": "api-work"}})
	return e
}

func rowIDs(l any) []string {
	out := []string{}
	for _, r := range l.([]map[string]any) {
		out = append(out, r["id"].(string))
	}
	return out
}

func TestAttachLinksASessionAndThePickerShowsIt(t *testing.T) {
	e := withAttach(t)
	e.session("44444444-dddd", "idle", "")
	e.wake("44444444-dddd")
	e.session("55555555-eeee", "closed", "")
	pick := pickList("demo-feature")
	eqList(t, rowIDs(pick["sessions"]), []string{"44444444-dddd", "55555555-eeee"}, "open first")
	s0 := pick["sessions"].([]map[string]any)[0]
	eq(t, fmt.Sprint(s0["name"], s0["open"]), fmt.Sprint("api-work", true))
	eq(t, len(pick["bound"].([]map[string]any)), 0)
	code, _ := act2(t, map[string]any{"feature": "demo-feature", "action": "attach", "session": "44444444-dddd"})
	eq(t, code, 200)
	pick = pickList("demo-feature")
	eqList(t, rowIDs(pick["bound"]), []string{"44444444-dddd"})
	eqList(t, rowIDs(pick["sessions"]), []string{"55555555-eeee"})
	route, _, _ := planRoute("demo-feature")
	eq(t, route, "active")
}

func TestAFinishedTurnWithABackgroundTaskDoesNotWaitForTheUser(t *testing.T) {
	e := withAttach(t)
	now := nowMs()
	e.sessionAt("44444444-dddd", "wait", "demo-feature", now)
	e.wake("44444444-dddd")
	row := func() *session { return readSessions(now, false, false, false)[0] }
	eq(t, row().State, "wait")
	e.setAgents(map[string]map[string]any{"44444444-dddd": {"name": "api-work", "status": "busy"}})
	setAgentsAt(time.UnixMilli(now - 1000)) // the list is older than the end of the turn
	eq(t, row().State, "wait")
	setAgentsAt(time.UnixMilli(now + 60000))
	eq(t, fmt.Sprint(row().State, "|", row().What), fmt.Sprint("bg", "|", "waiting for a background task"))
	eq(t, pickList("demo-feature")["bound"].([]map[string]any)[0]["state"], any("bg"))
}

func TestFeatureSwitcherListsFeaturesWithWhatTheirSessionDoes(t *testing.T) {
	e := withAttach(t)
	e.session("44444444-dddd", "wait", "demo-feature")
	got := featuresList()
	var demo map[string]any
	for _, f := range got["features"].([]map[string]any) {
		if f["id"] == "demo-feature" {
			demo = f
		}
	}
	if demo == nil {
		t.Fatal("demo-feature is not listed")
	}
	eq(t, fmt.Sprint(demo["state"], demo["fav"], demo["ask"], got["waiting"]), fmt.Sprint("wait", false, "reply", 1))
	act2(t, map[string]any{"action": "fav", "kind": "feature", "id": "demo-feature"})
	eq(t, featuresList()["features"].([]map[string]any)[0]["fav"], any(true))
	e.session("44444444-dddd", "idle", "demo-feature")
	eq(t, featuresList()["features"].([]map[string]any)[0]["ask"], any(""), "an idle session asks nothing")
}

func TestAttachValidatesInput(t *testing.T) {
	withAttach(t)
	code, _ := act2(t, map[string]any{"feature": "nope", "action": "attach", "session": "44444444-dddd"})
	eq(t, code, 404)
	code, _ = act2(t, map[string]any{"feature": "demo-feature", "action": "attach", "session": "../x"})
	eq(t, code, 400)
}

func TestAClosedAttachedSessionIsResumedInBackground(t *testing.T) {
	e := withAttach(t)
	e.session("55555555-eeee", "closed", "")
	act2(t, map[string]any{"feature": "demo-feature", "action": "attach", "session": "55555555-eeee"})
	route, _, _ := planRoute("demo-feature")
	eq(t, route, "bg")
}

// ------------------------------------------------------------------ BlockerItemTest

func withBlocker(t *testing.T) (*testEnv, string) {
	e := withDemo(t)
	d := readMap(e.demoSrc())
	d["blockers"] = js([]any{map[string]any{"t": "Need an account", "why": "w", "todo": "t", "proj": []any{"api"}, "stage": 1, "sev": "high", "who": "you"}})
	writeFile(t, e.demoSrc(), encodeJSON(d))
	bid := e.loadDemo()["blockers"].([]any)[0].(map[string]any)["bid"].(string)
	e.setAgents(map[string]map[string]any{})
	return e, bid
}

func TestAskAndDoGoToTheInboxWithTheItemText(t *testing.T) {
	_, bid := withBlocker(t)
	for _, action := range []string{"ask", "do"} {
		code, obj := act2(t, map[string]any{"feature": "demo-feature", "fid": bid, "kind": "blocker", "action": action, "text": "hi"})
		eq(t, code, 200, action)
		en := entryOf(t, obj)
		eq(t, fmt.Sprint(en["item"], num(en["stage"])), fmt.Sprint("Need an account", 1), action)
	}
}

// The user closes an open item on the page: "done" (did it) or "not needed". It leaves the open list at once
// and the session of the feature is told, so that it records it in feature.json. Restore brings it back.
func TestTheUserClosesAnOpenItem(t *testing.T) {
	for _, action := range []string{"done", "skip"} {
		e, bid := withBlocker(t)
		code, obj := act2(t, map[string]any{"feature": "demo-feature", "fid": bid, "kind": "blocker", "action": action, "text": "my word"})
		eq(t, code, 200, action)
		en := entryOf(t, obj)
		eq(t, fmt.Sprint(en["action"], en["item"]), action+"Need an account", "the session is told")
		contains(t, formatMessage(map[string]any{"action": action, "item": "Need an account", "text": "my word", "feature": "demo-feature", "id": "x"}),
			map[string]string{"done": "DONE", "skip": "NOT NEEDED"}[action], action)
		d := e.loadDemo()
		eq(t, len(d["blockers"].([]any)), 0, "no longer open")
		c := d["closedBlockers"].([]any)[0].(map[string]any)
		eq(t, fmt.Sprint(c["closed"], c["reason"]), action+"my word", "shown apart with the user's word")
		row, err := summary("demo-feature", d, time.Now())
		must(t, err)
		eq(t, num(row["waiting"]), 0, "not counted as waiting for the user")
		code, _ = act2(t, map[string]any{"feature": "demo-feature", "fid": bid, "kind": "blocker", "action": "unskip"})
		eq(t, code, 200, "restore")
		eq(t, len(e.loadDemo()["blockers"].([]any)), 1, "open again")
	}
}

// "Done" is the user's word about an open item; a fact of a stage is marked done by the session only, with evidence.
func TestDoneIsOnlyForOpenItems(t *testing.T) {
	e := withDemo(t)
	code, _ := act2(t, map[string]any{"feature": "demo-feature", "fid": e.fid(1), "action": "done"})
	eq(t, code, 400)
}

func TestUnknownItemIs404(t *testing.T) {
	withBlocker(t)
	code, _ := act2(t, map[string]any{"feature": "demo-feature", "fid": "0123456789", "kind": "blocker", "action": "ask"})
	eq(t, code, 404)
}

func TestWhoIsValidated(t *testing.T) {
	e, _ := withBlocker(t)
	d := readMap(e.demoSrc())
	d["blockers"].([]any)[0].(map[string]any)["who"] = "nobody"
	if _, ok := prepare(d, nil).(DataError); !ok {
		t.Error("who 'nobody' is accepted")
	}
}

// ------------------------------------------------------------------ SyncTest

func withSync(t *testing.T) *testEnv {
	e := withDemo(t)
	e.setAgents(map[string]map[string]any{"66666666-ffff": {"name": "api-work"}})
	return e
}

func touch(t *testing.T, path string, d time.Duration) {
	at := time.Now().Add(d)
	must(t, os.Chtimes(path, at, at))
}

func TestVersionChangesWithTheFile(t *testing.T) {
	e := withSync(t)
	v := featureVersion("demo-feature")
	touch(t, e.demoSrc(), 5*time.Second)
	if featureVersion("demo-feature") == v {
		t.Error("the version did not change")
	}
}

func TestBehindWhenTheSessionFinishedATurnAfterTheLastUpdate(t *testing.T) {
	e := withSync(t)
	touch(t, e.demoSrc(), -600*time.Second)
	e.session("66666666-ffff", "wait", "demo-feature")
	b, ok := versionInfo("demo-feature")["behind"].(map[string]any)
	if !ok {
		t.Fatal("not behind")
	}
	eq(t, b["name"], any("api-work"))
	if b["min"].(int) < 9 {
		t.Errorf("min = %v", b["min"])
	}
}

func TestNotBehindWhenTheFileWasUpdatedInTheSameTurn(t *testing.T) {
	e := withSync(t)
	e.session("66666666-ffff", "wait", "demo-feature")
	touch(t, e.demoSrc(), time.Second)
	eq(t, versionInfo("demo-feature")["behind"], nil)
}

func TestNotBehindWhileTheSessionWorks(t *testing.T) {
	e := withSync(t)
	touch(t, e.demoSrc(), -600*time.Second)
	e.session("66666666-ffff", "work", "demo-feature")
	eq(t, versionInfo("demo-feature")["behind"], nil)
}

func TestSyncRequestGoesToTheSessionOnce(t *testing.T) {
	e := withSync(t)
	e.session("66666666-ffff", "wait", "demo-feature")
	e.wake("66666666-ffff")
	code, obj := act2(t, map[string]any{"feature": "demo-feature", "action": "sync"})
	en := entryOf(t, obj)
	eq(t, fmt.Sprint(code, en["route"], en["kind"]), fmt.Sprint(200, "active", "sync"))
	_, obj2 := act2(t, map[string]any{"feature": "demo-feature", "action": "sync"})
	eq(t, entryOf(t, obj2)["id"], en["id"], "no second request while one is open")
	eq(t, versionInfo("demo-feature")["pending"], any("active"))
	text := formatMessage(inboxEntries()[0])
	contains(t, text, "Sync request")
	contains(t, text, "inbox resolve") // Python: "inbox.py resolve"
}

// ------------------------------------------------------------------ SessionsTabTest

func transcriptRow(role string, content any, stamp string) map[string]any {
	r := map[string]any{"message": map[string]any{"role": role, "content": content}}
	if stamp != "" {
		r["timestamp"] = stamp
	}
	return r
}

func withSessionsTab(t *testing.T) *testEnv {
	e := withDemo(t)
	e.setAgents(map[string]map[string]any{"77777777-aaaa": {"name": "api-xx", "cwd": e.home, "status": "idle", "startedAt": 5}})
	text := func(s string) []any { return []any{map[string]any{"type": "text", "text": s}} }
	e.appendTranscript(
		transcriptRow("user", "<system-reminder>noise</system-reminder>", ""),
		transcriptRow("user", "why does the cron stop on this error", ""),
		transcriptRow("assistant", text("Because of **die** in `parseRow()`"), ""),
		transcriptRow("user", "and how do we fix it", ""),
		transcriptRow("assistant", text("Replace it with an exception."), ""))
	return e
}

func (e *testEnv) appendTranscript(rows ...map[string]any) {
	path := filepath.Join(e.claude, "projects", "-x", "77777777-aaaa.jsonl")
	must(e.t, os.MkdirAll(filepath.Dir(path), 0o755))
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	must(e.t, err)
	for _, r := range rows {
		_, _ = f.Write(append(encodeJSON(r), '\n'))
	}
	f.Close()
	resetTranscripts()
}

func resetTranscripts() {
	tcacheMu.Lock()
	tcache = map[string]cacheItem{}
	tcacheMu.Unlock()
}

func TestSessionWithoutAnyHookEventIsListedWithWhatItIsAbout(t *testing.T) {
	withSessionsTab(t)
	rows := readSessions(nowMs(), false, false, true)
	if len(rows) != 1 {
		t.Fatalf("rows: %d", len(rows))
	}
	r := rows[0]
	eq(t, fmt.Sprint(r.ID, r.Name), fmt.Sprint("77777777-aaaa", "api-xx"))
	if r.First == nil || r.LastU == nil || r.LastA == nil {
		t.Fatal("no details from the transcript")
	}
	eq(t, *r.First, "why does the cron stop on this error")
	eq(t, *r.LastU, "and how do we fix it")
	eq(t, *r.LastA, "Replace it with an exception.")
	eq(t, r.Quiet, false)
	if strings.Contains(string(encodeJSON(r)), `"quiet"`) {
		t.Error(`the row has "quiet"`)
	}
}

func TestFeaturePageGetsTheLastReplyInFull(t *testing.T) {
	e := withSessionsTab(t)
	text := "Run is **still going**.\n\n- 21 of 21 checks match\n- left: `S17a`, `S17b`" + strings.Repeat(" x", 300)
	e.appendTranscript(
		transcriptRow("assistant", []any{map[string]any{"type": "text", "text": text}}, "2026-01-02T03:04:05.678Z"),
		transcriptRow("assistant", []any{map[string]any{"type": "tool_use", "name": "Bash"}}, ""))
	if _, ok := pickList("demo-feature")["sessions"].([]map[string]any)[0]["said"]; ok {
		t.Error("said is given for a session of another feature") // only for the feature's own
	}
	act2(t, map[string]any{"feature": "demo-feature", "action": "attach", "session": "77777777-aaaa"})
	row := pickList("demo-feature")["bound"].([]map[string]any)[0]
	eq(t, row["said"], any(text), "not cut to a line, markdown and line breaks kept")
	eq(t, row["saidT"], any(int64(1767323045678)))
}

func TestChatShowsTheConversationAndWhatWasSentFromThePage(t *testing.T) {
	e := withSessionsTab(t)
	stamp := func(n int) string { return fmt.Sprintf("2026-01-02T03:04:%02d.000Z", n) }
	tool := []any{map[string]any{"type": "tool_use", "name": "Bash"}}
	e.appendTranscript(
		transcriptRow("user", "what is the progress", stamp(1)),
		transcriptRow("assistant", "Let me look.", stamp(2)),
		transcriptRow("assistant", tool, stamp(2)),
		transcriptRow("user", []any{map[string]any{"type": "tool_result", "content": "ok"}}, stamp(2)),
		transcriptRow("assistant", "Done: **21 of 21**", stamp(3)),
		transcriptRow("user", "Stop hook feedback: [whatnow] sync", stamp(4)),
		transcriptRow("assistant", "dashboard in sync", stamp(5)),
		transcriptRow("assistant", "Checking S17b now.", stamp(6)),
		transcriptRow("assistant", tool, stamp(6)))
	code, r := act2(t, map[string]any{"action": "tell", "session": "77777777-aaaa", "text": "and S17a?"})
	eq(t, code, 200)
	items := chat("77777777-aaaa")["items"].([]map[string]any)
	if len(items) < 5 {
		t.Fatalf("items: %v", items)
	}
	items = items[len(items)-5:]
	var got []string
	for _, i := range items {
		got = append(got, fmt.Sprint(i["who"], ": ", i["text"]))
	}
	// the note before a tool call is left out, except the newest one
	eqList(t, got, []string{"u: what is the progress", "a: Done: **21 of 21**", "a: dashboard in sync", "a: Checking S17b now.", "me: and S17a?"})
	eq(t, fmt.Sprint(items[4]["state"], items[4]["id"]), fmt.Sprint("new", entryOf(t, r)["id"]))
}

// A message typed while Claude works is written by Claude Code as a queued attachment, not as a user row: the
// chat shows it too, so an answer is never left without its question.
func TestChatShowsWhatTheUserTypedWhileClaudeWorked(t *testing.T) {
	e := withSessionsTab(t)
	stamp := func(n int) string { return fmt.Sprintf("2026-01-02T03:05:%02d.000Z", n) }
	queued := func(a map[string]any, n int) map[string]any {
		a["type"] = "queued_command"
		return map[string]any{"type": "attachment", "attachment": a, "timestamp": stamp(n)}
	}
	human := map[string]any{"kind": "human"}
	e.appendTranscript(
		transcriptRow("user", "start the long work", stamp(1)),
		transcriptRow("assistant", []any{map[string]any{"type": "tool_use", "name": "Bash"}}, stamp(2)),
		queued(map[string]any{"commandMode": "prompt", "origin": human, "prompt": "and check the header too"}, 3),
		queued(map[string]any{"commandMode": "task-notification", "origin": map[string]any{"kind": "task-notification"}, "prompt": "<task-notification>done</task-notification>"}, 4),
		queued(map[string]any{"commandMode": "prompt", "isMeta": true, "origin": map[string]any{"body": "x"}, "prompt": "<cross-session-message>hello from another session</cross-session-message>"}, 5),
		queued(map[string]any{"commandMode": "prompt", "origin": human, "prompt": []any{map[string]any{"type": "image"}, map[string]any{"type": "text", "text": "look at this picture"}}}, 6),
		transcriptRow("user", "look at this picture", stamp(7)), // the same message, written a second time
		transcriptRow("assistant", "Both done.", stamp(8)))
	items := chat("77777777-aaaa")["items"].([]map[string]any)
	var got []string
	for _, i := range items {
		got = append(got, fmt.Sprint(i["who"], ": ", i["text"]))
	}
	if len(got) < 4 {
		t.Fatalf("items: %v", got)
	}
	eqList(t, got[len(got)-4:], []string{"u: start the long work", "u: and check the header too", "u: look at this picture", "a: Both done."})
}

func TestHideAndUnhide(t *testing.T) {
	withSessionsTab(t)
	code, _ := act2(t, map[string]any{"action": "hide", "session": "77777777-aaaa"})
	eq(t, code, 200)
	eq(t, len(readSessions(1, false, false, true)), 0)
	eq(t, len(readSessions(1, false, true, false)), 1)
	act2(t, map[string]any{"action": "unhide", "session": "77777777-aaaa"})
	eq(t, len(readSessions(1, false, false, false)), 1)
	code, _ = act2(t, map[string]any{"action": "hide", "session": "../x"})
	eq(t, code, 400)
}

func TestTellAnOpenSession(t *testing.T) {
	withSessionsTab(t)
	code, obj := act2(t, map[string]any{"action": "tell", "session": "77777777-aaaa", "text": "status please"})
	en := entryOf(t, obj)
	eq(t, fmt.Sprint(code, en["route"], en["session"]), fmt.Sprint(200, "active", "77777777-aaaa"))
	contains(t, formatMessage(inboxEntries()[0]), "status please")
	code, _ = act2(t, map[string]any{"action": "tell", "session": "77777777-aaaa", "text": " "})
	eq(t, code, 400)
}

func TestTrackAsksTheSessionToCreateAFeature(t *testing.T) {
	withSessionsTab(t)
	code, obj := act2(t, map[string]any{"action": "track", "session": "77777777-aaaa"})
	eq(t, fmt.Sprint(code, entryOf(t, obj)["route"]), fmt.Sprint(200, "active"))
	text := formatMessage(inboxEntries()[0])
	contains(t, text, "feature.json")
	contains(t, text, "render --index") // Python: "render.py --index"
}

func TestTellAClosedSessionResumesItInBackground(t *testing.T) {
	e := withSessionsTab(t)
	e.setAgents(map[string]map[string]any{})
	e.session("88888888-bbbb", "closed", "")
	e.fakeSet("resume.out", "ok")
	code, obj := act2(t, map[string]any{"action": "tell", "session": "88888888-bbbb", "text": "continue"})
	eq(t, fmt.Sprint(code, entryOf(t, obj)["route"]), fmt.Sprint(200, "bg"))
	eqList(t, e.fakeLines("resumed"), []string{"88888888-bbbb"})
	code, _ = act2(t, map[string]any{"action": "tell", "session": "99999999-cccc", "text": "x"})
	eq(t, code, 404)
}

// ------------------------------------------------------------------ FavoriteTest

func TestStarAndUnstarSessionsAndFeatures(t *testing.T) {
	withDemo(t)
	for _, body := range []map[string]any{{"action": "fav", "kind": "session", "id": "77777777-aaaa"},
		{"action": "fav", "kind": "feature", "id": "demo-feature"}, {"action": "fav", "kind": "feature", "id": "demo-feature"}} {
		code, _ := act2(t, body)
		eq(t, code, 200, body)
	}
	eqList(t, favorites(), []string{"s:77777777-aaaa", "f:demo-feature"}, "twice: once")
	eqList(t, stateJSON()["favorites"].([]string), []string{"s:77777777-aaaa", "f:demo-feature"})
	act2(t, map[string]any{"action": "unfav", "kind": "session", "id": "77777777-aaaa"})
	eqList(t, favorites(), []string{"f:demo-feature"})
}

func TestFavoriteInputIsValidated(t *testing.T) {
	withDemo(t)
	for _, body := range []map[string]any{{"action": "fav", "kind": "session", "id": "../x"}, {"action": "fav", "kind": "feature", "id": "a b"},
		{"action": "fav", "kind": "other", "id": "x"}, {"action": "fav", "kind": "session"}} {
		code, _ := act2(t, body)
		eq(t, code, 400, body)
	}
}

// ------------------------------------------------------------------ HttpSecurityTest

type httpT struct {
	*testEnv
	port int
}

func withHTTP(t *testing.T) *httpT {
	e := withDemo(t)
	e.setAgents(map[string]map[string]any{})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	h := &httpT{testEnv: e, port: ln.Addr().(*net.TCPAddr).Port}
	srv := &http.Server{Handler: handler{port: h.port}}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { srv.Close() })
	return h
}

// req sends a request; a header with the value "-" is left out, "Host" sets the Host header.
func (h *httpT) req(method, path, body string, headers map[string]string) (int, string) {
	h.t.Helper()
	var r *http.Request
	var err error
	if body != "" {
		r, err = http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d%s", h.port, path), strings.NewReader(body))
	} else {
		r, err = http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d%s", h.port, path), nil)
	}
	must(h.t, err)
	for k, v := range headers {
		switch {
		case v == "-":
		case k == "Host":
			r.Host = v
		default:
			r.Header.Set(k, v)
		}
	}
	c := http.Client{Timeout: 10 * time.Second}
	resp, err := c.Do(r)
	must(h.t, err)
	defer resp.Body.Close()
	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return resp.StatusCode, b.String()
}

func (h *httpT) post(headers map[string]string) int {
	all := map[string]string{"Content-Type": "application/json", "X-Whatnow": "1"}
	for k, v := range headers {
		all[k] = v
	}
	body := string(encodeJSON(map[string]any{"feature": "demo-feature", "fid": h.fid(1), "action": "skip", "text": "r"}))
	code, _ := h.req("POST", "/api/act", body, all)
	return code
}

func TestValidPostWorks(t *testing.T) {
	h := withHTTP(t)
	eq(t, h.post(nil), 200)
}

func TestCrossSitePostsAreRefused(t *testing.T) {
	h := withHTTP(t)
	eq(t, h.post(map[string]string{"Origin": "https://evil.example"}), 403)
	eq(t, h.post(map[string]string{"X-Whatnow": "-"}), 403, "no custom header")
	eq(t, h.post(map[string]string{"Content-Type": "text/plain"}), 403, "a simple cross-site request")
	eq(t, h.post(map[string]string{"Host": "evil.example"}), 403, "DNS rebinding")
	if _, err := os.Stat(filepath.Join(h.feats, "demo-feature", "overrides.json")); err == nil {
		t.Error("a refused request changed overrides.json")
	}
}

func TestBodyLimitsAndGarbage(t *testing.T) {
	h := withHTTP(t)
	hd := map[string]string{"Content-Type": "application/json", "X-Whatnow": "1"}
	code, _ := h.req("POST", "/api/act", strings.Repeat("x", 20000), hd)
	eq(t, code, 400)
	code, _ = h.req("POST", "/api/act", "not json", hd)
	eq(t, code, 400)
	code, _ = h.req("POST", "/api/other", "{}", hd)
	eq(t, code, 404)
}

func TestPagesAndInbox(t *testing.T) {
	h := withHTTP(t)
	code, html := h.req("GET", "/f/demo-feature/", "", nil)
	eq(t, code, 200)
	contains(t, html, `"live":true`) // Python's json.dumps writes "live": true; Go's encoder has no space
	for path, want := range map[string]int{"/f/../etc/": 404, "/api/inbox": 200, "/api/route?feature=demo-feature": 200, "/api/route?feature=../x": 400} {
		code, _ := h.req("GET", path, "", nil)
		eq(t, code, want, path)
	}
}

// ------------------------------------------------------------------ AgentsCacheTest (test_robustness.py)

func TestParallelRequestsStartOneClaudeAgents(t *testing.T) {
	e := isolate(t)
	if e.fakeBin == "" {
		t.Skip("the fake claude needs a POSIX shell")
	}
	e.fakeSet("agents.json", `[{"sessionId": "s"}]`)
	e.fakeSet("sleep", "0.3")
	parallel := func() []agentMap {
		var wg sync.WaitGroup
		var mu sync.Mutex
		var got []agentMap
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				v := activeSessions()
				mu.Lock()
				got = append(got, v)
				mu.Unlock()
			}()
		}
		wg.Wait()
		return got
	}
	got := parallel()
	eq(t, e.fakeCalls(), 1)
	for _, v := range got {
		if _, ok := v["s"]; !ok || len(v) != 1 {
			t.Errorf("a request got %v", v)
		}
	}
	setAgentsAt(time.Now().Add(-agentsTTL - time.Second)) // expired: one refresh, the rest get the old value
	parallel()
	eq(t, e.fakeCalls(), 2)
}
