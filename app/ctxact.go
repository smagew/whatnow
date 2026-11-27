package main

// What can be done when the context of a session is full (the page offers it, see side.js): compact it with what to
// keep, clear it and go on from the page of the work, or go on in a fresh session that starts from that page. A
// background session gets the command typed in (as by hand, in its terminal); a session open in the user's own
// terminal cannot be reached from here, and the page shows the command to paste there.

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// typeCommand is typeInto for the commands of this file; the tests replace it.
var typeCommand = func(sid string, keys ...string) bool { return typeInto(sid, true, keys...) }

func ctxSession(sid string) (map[string]any, bool) {
	if !sidRe.MatchString(sid) {
		return nil, false
	}
	a := activeSessions()[sid]
	return a, a != nil && a["kind"] == "background"
}

func ownTerminal(cmd string) (int, any) {
	return 409, map[string]any{"own": true, "command": cmd,
		"error": "This session is open in your own terminal, so whatnow cannot type into it. Paste this there: " + cmd}
}

// compactSession: /compact with what to keep.
func compactSession(sid, keep string) (int, any) {
	keep = strings.Join(strings.Fields(keep), " ")
	if len([]rune(keep)) > 2000 {
		return 400, map[string]any{"error": "what to keep is too long (2000 characters at most)"}
	}
	cmd := strings.TrimSpace("/compact " + keep)
	if _, bg := ctxSession(sid); !bg {
		return ownTerminal(cmd)
	}
	go typeCommand(sid, cmd, "\r")
	return 200, map[string]any{"ok": true}
}

// continueText: what a session reads after its context was emptied, to go on with its work.
func continueText(slug string) string {
	return "Your context was emptied. The work goes on from its page: read " + filepath.Join(featuresDir(), slug, "feature.json") +
		" (the plan, what is done, what is left, the decisions) and continue with the stage it is at. Keep the page in step."
}

// clearSession: /clear, then the session goes on from the page of its work.
func clearSession(sid string) (int, any) {
	s := findSession(sid)
	slug := ""
	if s != nil {
		slug = s.feature()
	}
	if slug == "" {
		return 400, map[string]any{"error": "this session has no work, so there is no page to go on from"}
	}
	if _, bg := ctxSession(sid); !bg {
		return ownTerminal("/clear")
	}
	go func() {
		if typeCommand(sid, "/clear", "\r") {
			time.Sleep(3 * time.Second)
			typeCommand(sid, continueText(slug), "\r")
		}
	}()
	return 200, map[string]any{"ok": true}
}

// handoff: the first message of a fresh session that takes over a work.
func handoff(slug string) string {
	return "You take over the work \"" + workTitle(slug) + "\" from a session whose context was full. Its page is " +
		filepath.Join(featuresDir(), slug, "feature.json") + ": read it first, it says where the work stands (the plan, what is done, " +
		"what is left, the decisions and questions). Then continue with the stage it is at, and keep the page in step with the work. " +
		"You may edit that file and run " + cliPath + " without asking."
}

// startFresh starts the fresh session; the tests replace it. It returns the id of the new session.
var startFresh = func(p map[string]any, slug string) (string, error) {
	claude := claudeBin()
	if claude == "" {
		return "", errString("claude not found")
	}
	args := []string{"--bg", "-n", runes(workTitle(slug), 40)}
	for _, d := range p["dirs"].([]any) {
		args = append(args, "--add-dir", d.(string))
	}
	if m := str(p, "mode"); m != "" {
		args = append(args, "--permission-mode", m)
	}
	args = append(append(args, "--allowedTools"), spawnAllow(slug)...)
	args = append(args, "--", handoff(slug))
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, claude, args...)
	cmd.Dir, cmd.Env = str(p, "root"), childEnv(claude)
	out, err := cmd.CombinedOutput()
	m := bgID.FindSubmatch(out)
	if m == nil {
		msg := runes(strings.TrimSpace(string(out)), 300)
		if msg == "" && err != nil {
			msg = err.Error()
		}
		return "", errString("Claude Code did not start the session: " + msg)
	}
	short := string(m[1])
	for i := 0; i < 40; i++ {
		for id, a := range readAgents() {
			if str(a, "id") == short {
				return id, nil
			}
		}
		time.Sleep(400 * time.Millisecond)
	}
	return "", nil
}

// freshSession: the work goes on in a new session with the same access and an empty context; the old one, when it
// runs in the background, is stopped.
// One fresh session per work at a time: starting one takes up to half a minute, and a second press (or the
// dialog opened again because nothing seemed to happen) started a second session on the same files.
var freshMu sync.Mutex
var freshAt = map[string]time.Time{}

func freshSession(sid string) (int, any) {
	s := findSession(sid)
	if s == nil || s.feature() == "" {
		return 400, map[string]any{"error": "this session has no work, so there is no page to start from"}
	}
	slug := s.feature()
	freshMu.Lock()
	if t, ok := freshAt[slug]; ok && time.Since(t) < 2*time.Minute {
		freshMu.Unlock()
		return 409, map[string]any{"error": "a fresh session for this work is being started already; it shows here in a moment"}
	}
	freshAt[slug] = time.Now()
	freshMu.Unlock()
	defer func() { // the guard stays a little after the start, until the new session is seen in the list
		go func() { time.Sleep(20 * time.Second); freshMu.Lock(); delete(freshAt, slug); freshMu.Unlock() }()
	}()
	code, p := spawnPlan(slug)
	if code != 200 {
		return code, p
	}
	nsid, err := startFresh(p, slug)
	if err != nil {
		return 500, map[string]any{"error": err.Error()}
	}
	if nsid != "" {
		_ = bind(nsid, slug)
	}
	// the old session lets the work go: the window of the work shows the new one, and messages to the work no longer
	// reach (and wake) the old one
	_ = unbind(sid, slug)
	stopped := false
	if a, bg := ctxSession(sid); bg {
		id := str(a, "id")
		if id == "" {
			id = sid
		}
		stopped = stopBackground(id) == nil
	}
	agentsReset()
	return 200, map[string]any{"ok": true, "session": nsid, "stopped": stopped}
}
