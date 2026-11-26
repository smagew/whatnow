package main

import (
	"testing"
)

// Finishing a work: its background sessions are stopped, the ones in the user's own terminal are named, the work
// leaves the bar and is kept as finished; reopening brings it back.
func TestFinishAWork(t *testing.T) {
	e := withDemo(t)
	bg, own, other := "aaaaaaaa-0000-0000-0000-000000000001", "bbbbbbbb-0000-0000-0000-000000000002", "cccccccc-0000-0000-0000-000000000003"
	e.session(bg, "work", "demo-feature")
	e.session(own, "wait", "demo-feature")
	e.session(other, "work", "")
	e.setAgents(map[string]map[string]any{
		bg:    {"kind": "background", "id": "aaaaaaaa", "name": "bg-one", "status": "busy", "pid": 1},
		own:   {"kind": "interactive", "id": "bbbbbbbb", "name": "mine", "status": "idle", "pid": 2},
		other: {"kind": "background", "id": "cccccccc", "name": "else", "status": "busy", "pid": 3},
	})
	var stopped []string
	old := stopBackground
	stopBackground = func(id string) error { stopped = append(stopped, id); return nil }
	t.Cleanup(func() { stopBackground = old })

	code, obj := finishPlan("demo-feature")
	eq(t, code, 200, "plan")
	p := jsMap(obj)
	eq(t, len(p["background"].([]any)), 1, "background sessions in the plan")
	eq(t, len(p["own"].([]any)), 1, "sessions in your terminal in the plan")

	code, obj = act(map[string]any{"action": "finish", "feature": "demo-feature"})
	eq(t, code, 200, "finish")
	if len(stopped) != 1 || stopped[0] != "aaaaaaaa" {
		t.Fatalf("stopped %v, want only the background session of the work", stopped)
	}
	r := jsMap(obj)
	eq(t, len(r["own"].([]any)), 1, "the session in your terminal is named, not stopped")
	if closedWorks()["demo-feature"] == 0 {
		t.Fatal("the work is not kept as finished")
	}
	for _, f := range featuresList()["features"].([]map[string]any) {
		if f["id"] == "demo-feature" {
			t.Error("a finished work is still in the bar")
		}
	}
	var mark any
	for _, f := range stateJSON()["features"].([]any) {
		if m := f.(map[string]any); m["id"] == "demo-feature" {
			mark = m["closed"]
		}
	}
	if mark == nil {
		t.Error("the list of work does not know the work is finished")
	}

	code, _ = act(map[string]any{"action": "reopen", "feature": "demo-feature"})
	eq(t, code, 200, "reopen")
	if closedWorks()["demo-feature"] != 0 {
		t.Error("a reopened work is still finished")
	}
	back := false
	for _, f := range featuresList()["features"].([]map[string]any) {
		back = back || f["id"] == "demo-feature"
	}
	if !back {
		t.Error("a reopened work is not back in the bar")
	}

	for _, bad := range []string{"nope", "..", ""} {
		if c, _ := act(map[string]any{"action": "finish", "feature": bad}); c != 400 {
			t.Errorf("finish %q: %d", bad, c)
		}
	}
}
