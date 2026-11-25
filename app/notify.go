package main

// Alerts of the app window: when a session has answered, or asks for a permission, and the window is not in
// front, the system shows a notification (with a sound, unless the user turned the sound or the alerts off).
//
// The window says whether it is in front (focus reports of its pages); the sessions are watched here, in the
// app itself, because a page that is not shown may have its timers slowed down or stopped by the system. The
// server on the 127.0.0.1 port never alerts: with both running there would be two of everything.

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var alerts struct {
	mu      sync.Mutex
	started bool
	focused bool
	said    map[string]int64 // session -> the reply that is known already
	perm    map[string]bool  // session -> it was waiting for a permission at the last look
	sent    []string         // what was shown, newest last (for the tests)
}

// alertMode is "sound" (a notification with a sound), "banner" (without) or "off"; config.json "alerts".
func alertMode() string {
	switch m, _ := config()["alerts"].(string); m {
	case "banner", "off":
		return m
	}
	return "sound"
}

func setAlertMode(m string) (int, any) {
	if m != "sound" && m != "banner" && m != "off" {
		return 400, map[string]any{"error": "bad mode"}
	}
	c := config()
	c["alerts"] = m
	if err := atomicWriteJSON(filepath.Join(home(), "config.json"), c); err != nil {
		return 500, map[string]any{"error": err.Error()}
	}
	return 200, map[string]any{"ok": true, "alerts": m}
}

// appFocus is a page of the app window saying whether the window is in front. The first report starts the watch.
func appFocus(on bool) {
	alerts.mu.Lock()
	alerts.focused = on
	start := !alerts.started
	alerts.started = true
	alerts.mu.Unlock()
	if start {
		go func() {
			alertCheck(true) // what is there already is not news
			for {
				time.Sleep(3 * time.Second)
				alertCheck(false)
			}
		}()
	}
}

// alertCheck looks at the sessions once. A reply is news when the turn has ended (the session waits or is idle)
// with a text newer than the one known; what Claude says while it still works is not.
func alertCheck(first bool) {
	type note struct{ title, text, slug string }
	var notes []note
	names := map[string]string{}
	for _, x := range collect(featuresDir())["features"].([]any) {
		if f, ok := x.(map[string]any); ok {
			names[str(f, "id")], _ = f["name"].(string)
		}
	}
	alerts.mu.Lock()
	if alerts.said == nil {
		alerts.said, alerts.perm = map[string]int64{}, map[string]bool{}
	}
	focused := alerts.focused
	alerts.mu.Unlock()
	for _, s := range readSessions(nowMs(), false, false, false) {
		who := names[s.feature()]
		if who == "" {
			who = s.Name
		}
		if who == "" {
			who, _ = s.Project.(string)
		}
		asks := s.What == "waiting for permission"
		alerts.mu.Lock()
		was := alerts.perm[s.ID]
		alerts.perm[s.ID] = asks
		alerts.mu.Unlock()
		if asks && !was && !first {
			notes = append(notes, note{who, "Claude asks for your permission.", s.feature()})
		}
		if s.State != "wait" && s.State != "idle" {
			continue
		}
		info := transcriptInfo(s.ID)
		if info == nil || info.SaidT == 0 || info.LastA == "" {
			continue
		}
		alerts.mu.Lock()
		known, seen := alerts.said[s.ID]
		alerts.said[s.ID] = info.SaidT
		alerts.mu.Unlock()
		if first || info.SaidT <= known || (!seen && nowMs()-info.SaidT > 60000) {
			continue // known already, or a session met for the first time with an old reply
		}
		notes = append(notes, note{who, runes(info.LastA, 160), s.feature()})
	}
	mode := alertMode()
	if focused || mode == "off" {
		return
	}
	for _, n := range notes {
		alertShow(n.title, n.text, n.slug, mode == "sound")
	}
}

var alertRun = func(name string, args ...string) { _ = exec.Command(name, args...).Start() } // replaced in the tests

// alertShow hands one notification to the system.
func alertShow(title, text, slug string, sound bool) {
	alerts.mu.Lock()
	alerts.sent = append(alerts.sent, title+": "+text)
	if len(alerts.sent) > 20 {
		alerts.sent = alerts.sent[len(alerts.sent)-20:]
	}
	alerts.mu.Unlock()
	if nativeNotify("whatnow", title, text, slug, sound) { // from whatnow.app itself: a click opens the work
		return
	}
	switch runtime.GOOS {
	case "darwin":
		q := func(s string) string { // an AppleScript string
			return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", " ", "\r", " ").Replace(s) + `"`
		}
		script := "display notification " + q(text) + " with title \"whatnow\" subtitle " + q(title)
		if sound {
			script += " sound name \"Glass\""
		}
		alertRun("osascript", "-e", script)
	case "linux":
		alertRun("notify-send", "--app-name=whatnow", title, text)
	}
}
