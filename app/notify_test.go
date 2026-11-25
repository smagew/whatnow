package main

import (
	"strings"
	"testing"
	"time"
)

// A session that answers while the app window is not in front is announced by the system; what was there when
// the watch began is not news, nor is anything while the window is in front or the alerts are off.
func TestAnAnswerIsAnnouncedWhenTheWindowIsNotInFront(t *testing.T) {
	e := withSessionsTab(t)
	var ran [][]string
	old := alertRun
	alertRun = func(name string, args ...string) { ran = append(ran, append([]string{name}, args...)) }
	t.Cleanup(func() {
		alertRun = old
		alerts.mu.Lock()
		alerts.said, alerts.perm, alerts.sent, alerts.focused = nil, nil, nil, false
		alerts.mu.Unlock()
	})
	stamp := func(d time.Duration) string { return time.Now().Add(d).UTC().Format("2006-01-02T15:04:05.000Z") }
	say := func(text string, d time.Duration) {
		e.appendTranscript(transcriptRow("assistant", []any{map[string]any{"type": "text", "text": text}}, stamp(d)))
	}
	focus := func(on bool) {
		alerts.mu.Lock()
		alerts.focused = on
		alerts.mu.Unlock()
	}
	e.session("77777777-aaaa", "wait", "demo-feature")
	say("The first answer.", -10*time.Minute)
	alertCheck(true)
	eq(t, len(ran), 0, "what is there when the watch begins is not news")

	say("Done: the export \"works\" now.", -2*time.Second)
	alertCheck(false)
	if runtimeAlerts() {
		eq(t, len(ran), 1, ran)
		joined := strings.Join(ran[0], " ")
		contains(t, joined, "the export", "the text of the answer")
		contains(t, joined, `\"works\"`, "quotes are escaped for the system")
		contains(t, joined, "Glass", "with a sound by default")
	}
	alerts.mu.Lock()
	sent := len(alerts.sent)
	alerts.mu.Unlock()
	eq(t, sent, 1)
	alertCheck(false)
	alerts.mu.Lock()
	sent = len(alerts.sent)
	alerts.mu.Unlock()
	eq(t, sent, 1, "the same answer is announced once")

	// while Claude still works, what it says is not an answer yet
	e.session("77777777-aaaa", "work", "demo-feature")
	say("Looking at the files.", -1*time.Second)
	alertCheck(false)
	alerts.mu.Lock()
	sent = len(alerts.sent)
	alerts.mu.Unlock()
	eq(t, sent, 1, "a note made on the way")

	// the window is in front: nothing; alerts off: nothing
	e.session("77777777-aaaa", "wait", "demo-feature")
	focus(true)
	say("Second answer.", 0)
	alertCheck(false)
	focus(false)
	code, _ := setAlertMode("off")
	eq(t, code, 200)
	say("Third answer.", time.Second)
	alertCheck(false)
	alerts.mu.Lock()
	sent = len(alerts.sent)
	alerts.mu.Unlock()
	eq(t, sent, 1, "in front, then off")

	code, _ = setAlertMode("banner")
	eq(t, code, 200)
	eq(t, alertMode(), "banner")
	say("Fourth answer.", 2*time.Second)
	alertCheck(false)
	alerts.mu.Lock()
	sent = len(alerts.sent)
	alerts.mu.Unlock()
	eq(t, sent, 2)
	if runtimeAlerts() && strings.Contains(strings.Join(ran[len(ran)-1], " "), "Glass") {
		t.Error("a silent alert played a sound")
	}
	code, _ = setAlertMode("loud")
	eq(t, code, 400)
}
