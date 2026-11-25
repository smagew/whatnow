package main

// One message to several sessions at once (the "Message sessions" form of the bar). Every session gets it the way a
// message to one session goes (tellSession: at once when it can take it, else woken, resumed or queued). With
// "interrupt" a background session is first stopped, as Esc in its terminal does; a session open in the user's own
// terminal cannot be stopped from here and gets the message when its turn ends.

import (
	"sort"
	"time"
)

// targets are the open sessions with what the form needs to choose among them.
func targets() map[string]any {
	ag := activeSessions()
	names := map[string]string{}
	for _, x := range collect(featuresDir())["features"].([]any) {
		if f, ok := x.(map[string]any); ok {
			names[str(f, "id")], _ = f["name"].(string)
		}
	}
	out := []any{}
	for _, s := range readSessions(nowMs(), false, false, false) {
		if s.State == "closed" {
			continue
		}
		kind := ""
		if a := ag[s.ID]; a != nil {
			kind = str(a, "kind")
		}
		name := s.Name
		if name == "" {
			name = ag.name(s.ID)
		}
		out = append(out, map[string]any{"id": s.ID, "name": name, "project": s.Project, "feature": s.feature(), "featureName": names[s.feature()],
			"state": s.State, "what": s.What, "t": s.T, "background": kind == "background"})
	}
	sort.SliceStable(out, func(i, j int) bool { return num(out[i].(map[string]any)["t"]) > num(out[j].(map[string]any)["t"]) })
	return map[string]any{"sessions": out}
}

func broadcast(body map[string]any) (int, any) {
	text, _ := body["text"].(string)
	interrupt := truthy(body["interrupt"])
	l, _ := body["sessions"].([]any)
	if len(l) == 0 {
		return 400, map[string]any{"error": "choose who gets the message"}
	}
	if len(l) > 100 {
		return 400, map[string]any{"error": "too many sessions at once (100 max)"}
	}
	results := []any{}
	sent := 0
	for _, x := range l {
		sid, _ := x.(string)
		if interrupt { // the wake comes after the stop, not before it
			woke.mu.Lock()
			woke.at[sid] = time.Now()
			woke.mu.Unlock()
		}
		code, v := tellSession(sid, text)
		r := map[string]any{"session": sid, "ok": code == 200}
		if m, ok := v.(map[string]any); ok {
			if e, ok := m["entry"].(map[string]any); ok {
				r["state"], r["route"] = e["state"], e["route"]
			}
			if m["error"] != nil {
				r["error"] = m["error"]
			}
		}
		if code == 200 {
			sent++
			if interrupt {
				r["interrupted"] = interruptSession(sid)
			}
		}
		results = append(results, r)
	}
	return 200, map[string]any{"ok": sent > 0, "sent": sent, "results": results}
}
