package main

// Waking a session that sleeps. A message from the dashboard reaches an open session through its hooks: while
// it works, at the end of the turn; while it is idle, through a watcher that Claude Code keeps for an hour. After
// that hour nothing listens, and the message waited until the user typed something in the terminal of the session.
//
// A background session has a terminal the dashboard can attach to. So for a sleeping background session the
// dashboard does what the user was told to do: it types one line there. The hook of that prompt delivers the
// waiting message. A session that is open in the user's own terminal cannot be reached this way.

import (
	"regexp"
	"strings"
	"sync"
	"time"
)

const wakeLine = "There is a message for you from the whatnow dashboard."

var (
	ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)|\x1b[()][0-9A-Za-z]|\x1b[=>78]`)
	// a question on the screen of the session: there Enter is an answer, and nothing is typed
	askRe = regexp.MustCompile(`(?i)do you want|esc to cancel|❯\s*1\.|\(y/n\)|press enter to`)
	woke  = struct {
		mu sync.Mutex
		at map[string]time.Time
	}{at: map[string]time.Time{}}
	wakeSettle = 2500 * time.Millisecond // the terminal of the session draws itself first
)

// wakeSession types the wake line into a sleeping background session, at most once a minute per session. It
// returns at once; false when the session is not one that can be woken this way.
func wakeSession(sid string) bool {
	if !termSupported || !sidRe.MatchString(sid) {
		return false
	}
	if a := activeSessions()[sid]; a == nil || a["kind"] != "background" {
		return false
	}
	woke.mu.Lock()
	if time.Since(woke.at[sid]) < time.Minute {
		woke.mu.Unlock()
		return true
	}
	woke.at[sid] = time.Now()
	woke.mu.Unlock()
	go wakeType(sid)
	return true
}

func wakeType(sid string) bool { return typeInto(sid, true, wakeLine, "\r") }

// interruptSession stops what a background session is doing right now, as Esc in its terminal does, then wakes it
// so that the message that waits for it (sent just before) is delivered at once. Esc also closes a question the
// session asked: that is a stop too.
func interruptSession(sid string) bool {
	if !termSupported || !sidRe.MatchString(sid) {
		return false
	}
	if a := activeSessions()[sid]; a == nil || a["kind"] != "background" {
		return false
	}
	go func() {
		if typeInto(sid, false, "\x1b") {
			time.Sleep(2 * time.Second)
			woke.mu.Lock()
			woke.at[sid] = time.Now()
			woke.mu.Unlock()
			typeInto(sid, true, wakeLine, "\r")
		}
	}()
	return true
}

// typeInto writes keys into the terminal of a background session, attaching to it for a moment when the window
// does not show it. careful: nothing is typed when the screen asks a question (Enter there would be an answer).
func typeInto(sid string, careful bool, keys ...string) bool {
	t := termFor(latest(sid))
	own := t == nil
	if own {
		code, v := termOpen(sid, 120, 32, true)
		m, _ := v.(map[string]any)
		info, _ := m["term"].(map[string]any)
		if code != 200 || info == nil {
			return false
		}
		t = termOf(str(info, "id"))
		if t == nil {
			return false
		}
		defer func() {
			time.Sleep(1500 * time.Millisecond) // the line is taken, then this terminal leaves; the session goes on
			termClose(t.ID)
		}()
		// wait until the screen has been drawn and is quiet
		deadline, last, quiet := time.Now().Add(8*time.Second), int64(-1), time.Now()
		for time.Now().Before(deadline) {
			time.Sleep(200 * time.Millisecond)
			t.mu.Lock()
			n := t.end()
			t.mu.Unlock()
			if n != last {
				last, quiet = n, time.Now()
			} else if n > 0 && time.Since(quiet) > wakeSettle {
				break
			}
		}
	}
	t.mu.Lock()
	buf := t.buf
	if len(buf) > 6000 {
		buf = buf[len(buf)-6000:]
	}
	screen := ansiRe.ReplaceAllString(string(buf), " ")
	done := t.done
	t.mu.Unlock()
	if done || (careful && askRe.MatchString(strings.Join(strings.Fields(screen), " "))) {
		return false // the session asks something: the user answers that, not a typed line
	}
	for i, k := range keys {
		if i > 0 {
			time.Sleep(200 * time.Millisecond)
		}
		if code, _ := termWrite(t.ID, k); code != 200 {
			return false
		}
	}
	return true
}
