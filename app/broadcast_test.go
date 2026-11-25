package main

import "testing"

// One message to several sessions: each one gets it as a message to one session; a bad id fails alone.
func TestOneMessageToSeveralSessions(t *testing.T) {
	e := withSessionsTab(t)
	e.session("77777777-aaaa", "idle", "demo-feature")
	code, _ := act2(t, map[string]any{"action": "broadcast", "text": "Status report"})
	eq(t, code, 400, "nobody chosen")
	code, r := act2(t, map[string]any{"action": "broadcast", "text": "Status report", "sessions": []any{"77777777-aaaa", "../bad"}})
	eq(t, code, 200, r)
	eq(t, r["sent"], any(1))
	res := r["results"].([]any)
	eq(t, res[0].(map[string]any)["ok"], any(true))
	eq(t, res[1].(map[string]any)["ok"], any(false))
	found := false
	for _, x := range inboxEntries() {
		if x["session"] == "77777777-aaaa" && x["text"] == "Status report" {
			found = true
		}
	}
	if !found {
		t.Error("the message is not in the inbox of the session")
	}
	l := targets()["sessions"].([]any)
	if len(l) == 0 || l[0].(map[string]any)["id"] != "77777777-aaaa" {
		t.Errorf("targets: %v", l)
	}
}
