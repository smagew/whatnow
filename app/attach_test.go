package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A file dropped on the window is kept with the work, listed with the screenshots under its own name, and can be
// deleted; only the app window can send it, and a large one is refused.
func TestAttachFiles(t *testing.T) {
	withDemo(t)
	up := func(h handler, name, body string, hdr bool) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7777/api/upload?feature=demo-feature&name="+name, strings.NewReader(body))
		if hdr {
			r.Header.Set("X-Whatnow", "1")
		}
		h.ServeHTTP(w, r)
		return w
	}
	win := handler{local: true}
	w := up(win, "report%20(final).pdf", "%PDF-1.4 ...", true)
	eq(t, w.Code, 200, "upload")
	name := jsMap(parseJSON(w.Body.Bytes()))["name"].(string)
	if !fileRe.MatchString(name) || !strings.HasSuffix(name, "report_final_.pdf") {
		t.Fatalf("name %q", name)
	}
	var found map[string]any
	for _, x := range listShots("demo-feature") {
		if m := x.(map[string]any); m["name"] == name {
			found = m
		}
	}
	if found == nil || found["kind"] != "file" || found["shown"] != "report_final_.pdf" || found["image"] != false {
		t.Fatalf("listed as %v", found)
	}
	b, _ := os.ReadFile(found["path"].(string))
	eq(t, string(b), "%PDF-1.4 ...", "the content")

	// not from a page on the port, not without the header, not a name that leaves the folder
	eq(t, up(handler{port: 7777}, "x.txt", "x", true).Code, 403, "from the port")
	eq(t, up(win, "x.txt", "x", false).Code, 403, "without the header")
	w = up(win, "..%2F..%2Fescape.txt", "x", true)
	eq(t, w.Code, 200, "a name with a path in it")
	if n := jsMap(parseJSON(w.Body.Bytes()))["name"].(string); strings.Contains(n, "/") || !isFile(filepath.Join(shotsDir("demo-feature"), n)) {
		t.Errorf("the file left its folder: %q", n)
	}
	// a file that is not a picture is downloaded, never shown or run in the page
	w = httptest.NewRecorder()
	win.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7777/shot?feature=demo-feature&name="+name, nil))
	eq(t, w.Header().Get("Content-Type"), "application/octet-stream", "content type of a pdf")
	// the 50 MB limit
	if _, err := saveAttach("demo-feature", "big.bin", strings.NewReader(strings.Repeat("x", maxAttach+1))); err == nil {
		t.Error("a file over 50 MB was kept")
	}
}
