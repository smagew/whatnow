package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The terminal is full access to the machine: only the app window gets it, never the 127.0.0.1 port.
func TestTheTerminalIsNotServedOnThePort(t *testing.T) {
	withDemo(t)
	port := handler{port: 7777}
	get := func(h handler, path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7777"+path, nil)
		h.ServeHTTP(w, r)
		return w
	}
	for _, p := range []string{"/api/term/list", "/api/term/read?id=x&off=0", "/xterm.js"} {
		eq(t, get(port, p).Code, 404, p)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7777/api/term", strings.NewReader(`{"op":"open","session":"11111111-2222-3333-4444-555555555555"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Whatnow", "1")
	port.ServeHTTP(w, r)
	eq(t, w.Code, 404, "POST /api/term on the port")
	for _, p := range []string{"/", "/f/demo-feature/"} {
		if body := get(port, p).Body.String(); strings.Contains(body, "WN_TERM=1") || strings.Contains(body, "/api/term") {
			t.Errorf("%s served on the port offers the terminal", p)
		}
	}
	if termSupported {
		window := handler{local: true}
		eq(t, get(window, "/api/term/list").Code, 200, "the app window")
		contains(t, get(window, "/f/demo-feature/").Body.String(), "WN_TERM=1", "the page in the app window")
	}
}

// /background continues a session under a new id; what the dashboard knows about it follows.
func TestASessionMovedToTheBackgroundKeepsItsFeature(t *testing.T) {
	e := withDemo(t)
	old, next := "aaaaaaaa-0000-0000-0000-000000000001", "bbbbbbbb-0000-0000-0000-000000000002"
	dir := filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "projects", "p")
	writeFile(t, filepath.Join(dir, old+".jsonl"), []byte(
		`{"type":"user","message":{"role":"user","content":"a note that mentions \"continued-in\" is not a move"}}`+"\n"+
			`{"type":"continued-in","sessionId":"`+old+`","continuedInSessionId":"`+next+`"}`+"\n"))
	writeFile(t, filepath.Join(dir, next+".jsonl"), []byte(`{"type":"user","message":{"role":"user","content":"hello again"}}`+"\n"))
	must(t, bind(old, "demo-feature"))
	must(t, setFavorite("s:"+old, true))
	eq(t, continuedIn(next), "", "a session that was not moved")
	eq(t, latest(old), next)
	eq(t, featureOf(next), "demo-feature", "the feature follows the session")
	fav := strings.Join(favorites(), " ")
	contains(t, fav, "s:"+next, "the star follows the session")
	_ = e
}
