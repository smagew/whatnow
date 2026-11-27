package main

import (
	"path/filepath"
	"testing"
)

func TestContextWindowOfAModel(t *testing.T) {
	for _, c := range []struct {
		model string
		used  int64
		want  int64
	}{
		{"claude-fable-5-2", 10, 1_000_000},
		{"claude-opus-5-5", 10, 1_000_000},
		{"claude-sonnet-5-5", 10, 1_000_000},
		{"claude-haiku-5-5", 10, 1_000_000},
		{"claude-opus-4-7", 10, 1_000_000},
		{"claude-opus-4-6", 10, 200_000},
		{"claude-opus-4-6[1m]", 10, 1_000_000},
		{"claude-sonnet-4-6", 10, 200_000},
		{"claude-haiku-4-5", 10, 200_000},
		{"something-else", 250_000, 1_000_000},
	} {
		eq(t, ctxWindow(c.model, c.used), c.want, c.model)
	}
}

// The fill is the input of the latest reply: input + cache creation + cache read; output does not count.
func TestContextFillFromTheTranscript(t *testing.T) {
	e := isolate(t)
	sid := "dddddddd-0000-0000-0000-000000000004"
	reply := func(in, cc, cr, out int) map[string]any {
		return map[string]any{"type": "assistant", "message": map[string]any{"role": "assistant", "model": "claude-opus-4-6",
			"content": []any{map[string]any{"type": "text", "text": "ok"}},
			"usage":   map[string]any{"input_tokens": in, "cache_creation_input_tokens": cc, "cache_read_input_tokens": cr, "output_tokens": out}}}
	}
	var rows []byte
	for _, r := range []any{map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": "hello"}}, reply(10, 1000, 5000, 300), reply(20, 2000, 90000, 400)} {
		rows = append(append(rows, encodeJSON(r)...), '\n')
	}
	writeFile(t, filepath.Join(e.claude, "projects", "-x", sid+".jsonl"), rows)
	info := transcriptInfo(sid)
	if info == nil {
		t.Fatal("no transcript")
	}
	eq(t, info.Ctx, int64(92020), "the fill of the latest reply")
	eq(t, info.Model, "claude-opus-4-6", "model")
}
