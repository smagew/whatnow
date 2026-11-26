package main

import (
	"testing"
	"time"
)

// A session that ended without Claude Code saying so (closed with its terminal, crashed, a background session
// stopped) is closed, not "open and silent": after 6 h it vanished, and its work said that no session was linked.
func TestASessionThatEndedWithoutAWordStaysWithItsWork(t *testing.T) {
	e := withDemo(t)
	gone := "11111111-0000-0000-0000-000000000001"
	open := "22222222-0000-0000-0000-000000000002"
	e.sessionAt(gone, "idle", "demo-feature", nowMs()-13*3600*1000)
	e.sessionAt(open, "idle", "", nowMs()-7*3600*1000)
	e.setAgents(map[string]map[string]any{open: {"kind": "interactive", "status": "idle", "pid": 1}})

	var found *session
	for _, s := range readSessions(nowMs(), true, false, false) {
		if s.ID == gone {
			found = s
		}
	}
	if found == nil {
		t.Fatal("the session that ended without a word is gone from the list")
	}
	eq(t, found.State, "closed", "its state")
	var bound []string
	for _, r := range pickList("demo-feature")["bound"].([]map[string]any) {
		bound = append(bound, r["id"].(string))
	}
	if len(bound) != 1 || bound[0] != gone {
		t.Errorf("the work does not show its session: %v", bound)
	}

	// a session that Claude Code still lists stays open, however long it is silent
	for _, s := range readSessions(nowMs(), true, false, false) {
		if s.ID == open && s.State == "closed" {
			t.Error("an open session counts as closed")
		}
	}

	// a session that just started may not be in the list yet: not closed while the list is older than it
	fresh := "33333333-0000-0000-0000-000000000003"
	e.sessionAt(fresh, "work", "", nowMs())
	setAgentsAt(time.Now().Add(-time.Minute))
	for _, s := range readSessions(nowMs(), true, false, false) {
		if s.ID == fresh && s.State == "closed" {
			t.Error("a session newer than the list of open sessions counts as closed")
		}
	}
}
