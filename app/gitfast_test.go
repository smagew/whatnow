package main

import (
	"testing"
	"time"
)

// A page that is being opened gets the state that is known already, at once; an old one is read again in the
// background and is there for the next page.
func TestAPageDoesNotWaitForGit(t *testing.T) {
	withDemo(t)
	old := map[string]any{"repos": []any{}, "mark": "old"}
	gitCacheMu.Lock()
	gitCache["demo-feature-"] = gitCacheItem{time.Now().Add(-time.Hour), old}
	gitCacheMu.Unlock()
	t.Cleanup(func() {
		gitCacheMu.Lock()
		delete(gitCache, "demo-feature-")
		gitCacheMu.Unlock()
	})
	if got := gitStateFast("demo-feature"); got["mark"] != "old" {
		t.Fatalf("the page waited for a fresh state: %v", got)
	}
	for i := 0; i < 100; i++ { // the background reading replaces it
		time.Sleep(50 * time.Millisecond)
		gitCacheMu.Lock()
		c, busy := gitCache["demo-feature-"], gitBusy["demo-feature-"]
		gitCacheMu.Unlock()
		if c.v["mark"] == nil && !busy {
			return
		}
	}
	t.Fatal("the old state was not read again in the background")
}
