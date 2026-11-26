package main

// Finishing a piece of work: the user says it is over (whatever its plan says). Its background sessions are
// stopped, it leaves the bar, and the list of work keeps it under Completed, from where it can be reopened.
// Which works are finished is kept in ~/.whatnow/closed.json ({work: ms}), not in feature.json: the session of the
// work writes that file and must not be able to undo the user's decision by rewriting it.

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var closedMu sync.Mutex

func closedFile() string { return filepath.Join(home(), "closed.json") }

// closedWorks: the finished works and when they were finished
func closedWorks() map[string]int64 {
	closedMu.Lock()
	defer closedMu.Unlock()
	out := map[string]int64{}
	m, _ := readJSON(closedFile()).(map[string]any)
	for k, v := range m {
		out[k] = num(v)
	}
	return out
}

func setClosed(slug string, on bool) error {
	closedMu.Lock()
	defer closedMu.Unlock()
	m, _ := readJSON(closedFile()).(map[string]any)
	if m == nil {
		m = map[string]any{}
	}
	if on {
		m[slug] = nowMs()
	} else {
		delete(m, slug)
	}
	return atomicWriteJSON(closedFile(), m)
}

// workSessions: the open sessions of a work, split into the ones that run in the background (they can be stopped
// from here) and the ones open in a terminal of the user's own (they cannot).
func workSessions(slug string) (bg, own []map[string]string) {
	ag := activeSessions()
	for _, s := range readSessions(nowMs(), false, false, false) {
		a, listed := ag[s.ID]
		if s.feature() != slug || !listed || s.State == "closed" {
			continue
		}
		r := map[string]string{"session": s.ID, "id": str(a, "id"), "name": ag.name(s.ID)}
		if r["name"] == "" {
			r["name"] = s.ID[:8]
		}
		if a["kind"] == "background" {
			bg = append(bg, r)
		} else {
			own = append(own, r)
		}
	}
	return
}

func names(l []map[string]string) []string {
	out := []string{}
	for _, r := range l {
		out = append(out, r["name"])
	}
	return out
}

func finishSlug(slug string) bool {
	return winSlug(slug) && isDir(filepath.Join(featuresDir(), slug))
}

// finishPlan is what the confirmation says: which sessions stop, which cannot be stopped from here.
func finishPlan(slug string) (int, any) {
	if !finishSlug(slug) {
		return 400, map[string]any{"error": "bad work"}
	}
	bg, own := workSessions(slug)
	return 200, map[string]any{"work": slug, "name": workTitle(slug), "background": names(bg), "own": names(own), "closed": closedWorks()[slug]}
}

// stopBackground stops a background session (`claude stop <id>`); the tests replace it.
var stopBackground = func(id string) error {
	claude := claudeBin()
	if claude == "" {
		return errString("claude not found")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, claude, "stop", id)
	cmd.Env = childEnv(claude)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if m := strings.TrimSpace(string(out)); m != "" {
			return errString(runes(m, 200))
		}
		return err
	}
	return nil
}

type errString string

func (e errString) Error() string { return string(e) }

// finishWork: the background sessions of the work are stopped and the work is finished. A session that cannot be
// stopped does not keep the work open: the answer names it.
func finishWork(slug string) (int, any) {
	if !finishSlug(slug) {
		return 400, map[string]any{"error": "bad work"}
	}
	bg, own := workSessions(slug)
	stopped, failed := []string{}, []map[string]string{}
	for _, r := range bg {
		id := r["id"]
		if id == "" {
			id = r["session"]
		}
		if err := stopBackground(id); err != nil {
			failed = append(failed, map[string]string{"name": r["name"], "error": err.Error()})
			continue
		}
		stopped = append(stopped, r["name"])
	}
	if err := setClosed(slug, true); err != nil {
		return 500, map[string]any{"error": "could not mark the work finished: " + err.Error()}
	}
	agentsReset() // the stopped sessions are not open any more: read the list again
	return 200, map[string]any{"ok": true, "stopped": stopped, "own": names(own), "failed": failed}
}

func reopenWork(slug string) (int, any) {
	if !finishSlug(slug) {
		return 400, map[string]any{"error": "bad work"}
	}
	if err := setClosed(slug, false); err != nil {
		return 500, map[string]any{"error": err.Error()}
	}
	return 200, map[string]any{"ok": true}
}

// agentsReset: the list of open sessions is read again on its next use
func agentsReset() {
	agents.mu.Lock()
	agents.at = time.Time{}
	agents.mu.Unlock()
}
