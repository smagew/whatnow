package main

import (
	"path/filepath"
	"testing"
)

// A stage that a session has not finished writing (no verdict, no plan, no facts yet) does not break the page.
func TestAStageWithoutVerdictIsShown(t *testing.T) {
	e := isolate(t)
	src := filepath.Join(e.feats, "half", "feature.json")
	writeFile(t, src, encodeJSON(map[string]any{"title": "Half written", "projects": []any{"www"},
		"stages": []any{
			map[string]any{"n": 1, "title": "Agree on the scope", "verdict": "In progress", "plan": []any{"Ask"}, "facts": []any{[]any{"w", "Asking"}}},
			map[string]any{"n": 2, "title": "Then the work"}}}))
	d, err := load(src)
	if err != nil {
		t.Fatalf("a stage without verdict, plan and facts fails the page: %v", err)
	}
	s := d["stages"].([]any)[1].(map[string]any)
	eq(t, s["verdict"], any(""), "verdict")
	eq(t, len(s["facts"].([]any)), 0, "facts")
	for _, f := range collect(e.feats)["broken"].([]any) {
		t.Errorf("listed as broken: %v", f)
	}
}
