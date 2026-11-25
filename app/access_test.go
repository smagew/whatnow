package main

import (
	"os"
	"path/filepath"
	"testing"
)

// What a session may do comes from its transcript (mode, model, MCP servers, instruction files, tools) and from
// the settings files that apply to the folder it was started in.
func TestAccessOfASession(t *testing.T) {
	withDemo(t)
	sid := "dddddddd-0000-0000-0000-000000000004"
	root := t.TempDir()
	line := func(v map[string]any) string { return string(encodeJSON(v)) + "\n" }
	att := func(a map[string]any) string {
		return line(map[string]any{"type": "attachment", "cwd": root, "attachment": a})
	}
	tools := []any{"mcp__claude_ai_Docs__read", "mcp__claude_ai_Docs__write", "mcp__claude_ai_Docs__list", "mcp__box__authenticate", "Monitor"}
	path := filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "projects", "p", sid+".jsonl")
	writeFile(t, path, []byte(
		line(map[string]any{"type": "permission-mode", "permissionMode": "default"})+
			att(map[string]any{"type": "model", "identity": map[string]any{"marketingName": "Opus 5.5"}})+
			att(map[string]any{"type": "deferred_tools_delta", "addedNames": tools})+
			att(map[string]any{"type": "mcp_instructions_delta", "addedNames": []any{"claude.ai Docs"}})+
			att(map[string]any{"type": "instructions", "files": []any{map[string]any{"path": "/x/CLAUDE.md", "type": "User"}}})+
			line(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": `a note about "permission-mode" and "tools-used"`}})+
			line(map[string]any{"type": "tools-used", "tools": []any{"Bash", "Read"}})))
	a := accessOf(sid)
	if a == nil {
		t.Fatal("no transcript")
	}
	eq(t, a.Mode, "default")
	eq(t, a.Model, "Opus 5.5")
	// the session changes its mode, uses one more tool, and the list of tools comes again after a compaction
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	must(t, err)
	_, err = f.WriteString(line(map[string]any{"type": "permission-mode", "permissionMode": "auto"}) +
		att(map[string]any{"type": "deferred_tools_delta", "addedNames": tools}) +
		line(map[string]any{"type": "tools-used", "tools": []any{"Edit", "Bash"}}))
	must(t, err)
	must(t, f.Close())
	a = accessOf(sid)
	eq(t, a.Mode, "auto", "the mode it has now")
	eq(t, len(a.Tools), 3, "tools it has used")
	eq(t, len(a.Files), 1, "instruction files")

	must(t, bind(sid, "demo-feature"))
	writeFile(t, filepath.Join(root, ".claude", "settings.local.json"),
		[]byte(`{"permissions":{"allow":["Bash(go test *)"],"deny":["Read(./.env)"],"additionalDirectories":["/data"]}}`))
	writeFile(t, filepath.Join(root, ".claude", "settings.json"), []byte(`{"permissions":{}}`)) // says nothing: not listed
	ss := accessState("demo-feature")["sessions"].([]any)
	eq(t, len(ss), 1)
	s := ss[0].(map[string]any)
	eq(t, s["mode"], any("auto"))
	mcp := map[string]int{}
	for _, m := range s["mcp"].([]any) {
		mcp[m.(map[string]any)["name"].(string)] = m.(map[string]any)["tools"].(int)
	}
	eq(t, mcp["claude.ai Docs"], 3, "a server by its name, its tools counted once")
	eq(t, mcp["box"], 1, "a server known only from its tools")
	eq(t, len(mcp), 2)
	rules := s["rules"].([]any)
	eq(t, len(rules), 1, "only the file that has rules")
	r := rules[0].(map[string]any)
	eq(t, r["source"], any("local"))
	eq(t, len(r["allow"].([]any))+len(r["deny"].([]any))+len(r["dirs"].([]any)), 3)
}
