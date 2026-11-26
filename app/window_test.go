package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// fakeWindows: the other processes are not started; what would start is counted.
func fakeWindows(t *testing.T) (solos *[]string, mains *int, quits *int) {
	solos, mains, quits = new([]string), new(int), new(int)
	s0, m0, q0 := startSolo, startMain, quitWindow
	startSolo = func(slug string) error { *solos = append(*solos, slug); return nil }
	startMain = func() error { *mains++; return nil }
	quitWindow = func() { *quits++ }
	t.Cleanup(func() { startSolo, startMain, quitWindow = s0, m0, q0; soloSlug = "" })
	return
}

func appCall(t *testing.T, h handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7777/api/app", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Whatnow", "1")
	h.ServeHTTP(w, r)
	return w
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// From the main window: the work opens in a window of its own; asked again, that window comes to the front.
func TestAWorkOpensInASeparateWindow(t *testing.T) {
	withDemo(t)
	solos, _, _ := fakeWindows(t)
	win := handler{local: true}
	eq(t, appCall(t, win, `{"op":"detach","feature":"demo-feature"}`).Code, 200, "detach")
	eq(t, len(*solos), 1, "a window process started")
	eq(t, (*solos)[0], "demo-feature", "for the work")
	if !soloOpen("demo-feature") || !detachedSet()["demo-feature"] {
		t.Fatal("the work does not count as open in a separate window")
	}
	// the bar of the main window marks it
	var marked bool
	for _, f := range featuresList()["features"].([]map[string]any) {
		if f["id"] == "demo-feature" {
			marked = f["detached"] == true
		}
	}
	if !marked {
		t.Error("the bar does not mark the work as open in a separate window")
	}
	// asked again: no second window, the one there is comes to the front
	eq(t, appCall(t, win, `{"op":"detach","feature":"demo-feature"}`).Code, 200, "detach again")
	eq(t, len(*solos), 1, "still one window")
	if !exists(winFile("demo-feature.focus")) {
		t.Error("the open window is not asked to come to the front")
	}
	// from the main window it is taken back
	eq(t, appCall(t, win, `{"op":"reattach","feature":"demo-feature"}`).Code, 200, "reattach")
	if !exists(winFile("demo-feature.back")) {
		t.Error("the separate window is not asked to close")
	}
	// not a work, not a path out of the folder, not from a page on the port
	for _, bad := range []string{`"nope"`, `".."`, `"."`, `"../x"`} {
		if c := appCall(t, win, `{"op":"detach","feature":`+bad+`}`).Code; c != 400 {
			t.Errorf("detach %s: %d", bad, c)
		}
	}
	if c := appCall(t, handler{port: 7777}, `{"op":"detach","feature":"demo-feature"}`).Code; c == 200 {
		t.Error("a page on the port opened a window")
	}
}

// A window that stopped beating does not count as open: a crashed window can be opened again.
func TestAGoneSeparateWindowIsNotOpen(t *testing.T) {
	withDemo(t)
	solos, _, _ := fakeWindows(t)
	touchFile(winFile("demo-feature.json"))
	old := time.Now().Add(-time.Minute)
	must(t, os.Chtimes(winFile("demo-feature.json"), old, old))
	if soloOpen("demo-feature") {
		t.Fatal("a window that stopped beating counts as open")
	}
	appCall(t, handler{local: true}, `{"op":"detach","feature":"demo-feature"}`)
	eq(t, len(*solos), 1, "a new window is opened")
}

// In the separate window: the page of the work only, without the bar, and a way back to the main window.
func TestTheSeparateWindowGoesBack(t *testing.T) {
	withDemo(t)
	_, mains, quits := fakeWindows(t)
	soloSlug = "demo-feature"
	win := handler{local: true}
	w := httptest.NewRecorder()
	win.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7777/", nil))
	eq(t, w.Code, 200, "the home page of a separate window (the window shows a redirect as it is: no 302)")
	contains(t, w.Body.String(), `location.replace("/f/demo-feature/")`, "leads to its work")
	w = httptest.NewRecorder()
	win.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7777/f/demo-feature/", nil))
	contains(t, w.Body.String(), `window.WN_SOLO="demo-feature"`, "the page knows it is a separate window")

	// the main window is not open: it is started, and shows the work
	eq(t, appCall(t, win, `{"op":"back"}`).Code, 200, "back")
	eq(t, readText(t, winFile("main.goto")), "demo-feature", "the main window is asked to show the work")
	eq(t, *mains, 1, "the main window is started")
	time.Sleep(300 * time.Millisecond)
	eq(t, *quits, 1, "the separate window closes")

	// the main window is open: it is not started again
	_ = os.Remove(winFile("main.goto"))
	touchFile(winFile("main.json"))
	appCall(t, win, `{"op":"back"}`)
	eq(t, *mains, 1, "no second main window")

	// the main window has no "back"
	soloSlug = ""
	eq(t, appCall(t, win, `{"op":"back"}`).Code, 400, "back in the main window")
}

// The place of a separate window is kept per work.
func TestTheSeparateWindowKeepsItsPlace(t *testing.T) {
	withDemo(t)
	if placeOf("demo-feature") != nil {
		t.Fatal("a place before one was kept")
	}
	placeSave("demo-feature", 1900, 40, 1200, 900)
	p := placeOf("demo-feature")
	if p == nil || p[0] != 1900 || p[1] != 40 || p[2] != 1200 || p[3] != 900 {
		t.Fatalf("place: %v", p)
	}
	placeSave("demo-feature", 10, 10, 100, 100) // too small to be a real window
	if placeOf("demo-feature") != nil {
		t.Error("a window too small to use is taken up")
	}
	eq(t, workTitle("demo-feature"), demoFeature()["title"].(string), "the title of the window")
}
