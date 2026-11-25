package main

// The Access tab of a feature: what each of its sessions is allowed to do and to reach. Two sources:
//   - the transcript of the session, where Claude Code writes its permission mode, its model, the MCP servers and
//     instruction files it got, and the tools it used;
//   - the settings files that apply to it (user, project, local), where the allow / ask / deny rules live.
// Nothing is changed anywhere; the page only shows it.

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type access struct {
	Mode, Model string
	MCP         map[string]int  // server -> number of its tools the session was offered
	mcpTool     map[string]bool // tools already counted (the same list comes again after a compaction)
	Files       []any           // instruction files it loaded: [{path, type}]
	seenFile    map[string]bool // paths already in Files
	Tools       map[string]bool // tools it used
	GitDirs     map[string]bool // folders it committed or pushed in with `git -C dir` or `cd dir && git` (see git.go)
}

type accessCache struct {
	path string
	size int64
	a    *access
}

var (
	accCache   = map[string]*accessCache{}
	accCacheMu sync.Mutex
	// only the rows that carry one of these are parsed: a transcript is large and most of it is conversation
	accMarks = [][]byte{[]byte(`"permission-mode"`), []byte(`"tools-used"`), []byte(`"mcp_instructions_delta"`),
		[]byte(`"deferred_tools_delta"`), []byte(`"type":"instructions"`), []byte(`"type":"model"`)}
)

func (a *access) note(line []byte) {
	hit := false
	for _, m := range accMarks {
		if bytes.Contains(line, m) {
			hit = true
			break
		}
	}
	if !hit {
		return
	}
	row, _ := parseJSON(line).(map[string]any)
	if row == nil {
		return
	}
	switch row["type"] {
	case "permission-mode":
		if m := str(row, "permissionMode"); m != "" {
			a.Mode = m
		}
	case "tools-used":
		for _, t := range asListAny(row["tools"]) {
			if s, _ := t.(string); s != "" {
				a.Tools[s] = true
			}
		}
	case "attachment":
		at, _ := row["attachment"].(map[string]any)
		switch at["type"] {
		case "model":
			if id, _ := at["identity"].(map[string]any); id != nil {
				if n := str(id, "marketingName"); n != "" {
					a.Model = n
				}
			}
		case "mcp_instructions_delta":
			for _, n := range asListAny(at["addedNames"]) {
				if s, _ := n.(string); s != "" {
					if _, ok := a.MCP[s]; !ok {
						a.MCP[s] = 0
					}
				}
			}
			for _, n := range asListAny(at["removedNames"]) {
				if s, _ := n.(string); s != "" {
					delete(a.MCP, s)
				}
			}
		case "deferred_tools_delta": // mcp__<server>__<tool>
			for _, n := range asListAny(at["addedNames"]) {
				s, _ := n.(string)
				if p := strings.SplitN(s, "__", 3); len(p) == 3 && p[0] == "mcp" && p[1] != "" && !a.mcpTool[s] {
					a.mcpTool[s] = true
					a.MCP[mcpName(a.MCP, p[1])]++
				}
			}
		case "instructions":
			for _, f := range asListAny(at["files"]) {
				m, _ := f.(map[string]any)
				if p := str(m, "path"); p != "" && !a.seenFile[p] {
					a.seenFile[p] = true
					a.Files = append(a.Files, map[string]any{"path": tilde(p), "type": str(m, "type")})
				}
			}
		}
	}
}

func asListAny(v any) []any { l, _ := v.([]any); return l }

// mcpName finds the server a tool belongs to among the names already known: a tool is called
// mcp__claude_ai_Claude_Docs__read, its server "claude.ai Claude Docs".
func mcpName(known map[string]int, id string) string {
	norm := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r == '.' || r == ' ' || r == '-' {
				return '_'
			}
			return r
		}, s)
	}
	for name := range known {
		if norm(name) == id {
			return name
		}
	}
	return id
}

// accessOf reads what a session may do from its transcript. Only what was appended since the last look is read.
func accessOf(sid string) *access {
	path := findTranscript(sid)
	if path == "" {
		return nil
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil
	}
	accCacheMu.Lock()
	defer accCacheMu.Unlock()
	c := accCache[sid]
	if c == nil || c.path != path || st.Size() < c.size {
		c = &accessCache{path: path, a: &access{MCP: map[string]int{}, mcpTool: map[string]bool{}, seenFile: map[string]bool{}, Tools: map[string]bool{}}}
		accCache[sid] = c
	}
	if st.Size() == c.size {
		return c.a
	}
	f, err := os.Open(path)
	if err != nil {
		return c.a
	}
	defer f.Close()
	if _, err := f.Seek(c.size, io.SeekStart); err != nil {
		return c.a
	}
	r := bufio.NewReaderSize(f, 1<<20)
	read := c.size
	for {
		line, err := r.ReadBytes('\n')
		if err != nil { // the last line may still be written: it is read next time, whole
			break
		}
		read += int64(len(line))
		c.a.note(line)
		c.a.noteGit(line)
	}
	c.size = read
	return c.a
}

// rulesOf reads the permission rules of one settings file; nil when the file is not there or says nothing.
func rulesOf(path, source string) map[string]any {
	p, _ := readMap(path)["permissions"].(map[string]any)
	if p == nil {
		return nil
	}
	list := func(k string) []any {
		out := []any{}
		for _, x := range asListAny(p[k]) {
			if s, _ := x.(string); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	r := map[string]any{"source": source, "file": tilde(path), "allow": list("allow"), "ask": list("ask"), "deny": list("deny"),
		"dirs": list("additionalDirectories"), "mode": str(p, "defaultMode")}
	if len(r["allow"].([]any))+len(r["ask"].([]any))+len(r["deny"].([]any))+len(r["dirs"].([]any)) == 0 && r["mode"] == "" {
		return nil
	}
	return r
}

// accessState is the Access tab: one entry per session of the feature, open ones first.
func accessState(slug string) map[string]any {
	type row = map[string]any
	out := []any{}
	names := activeSessions()
	var sids []string
	for sid, f := range bindings() {
		if f == slug {
			sids = append(sids, sid)
		}
	}
	sort.Strings(sids)
	for _, sid := range sids {
		s := findSession(sid)
		info := transcriptInfo(sid)
		a := accessOf(sid)
		if s == nil && info == nil && a == nil {
			continue
		}
		if s == nil {
			s = &session{ID: sid, State: "closed"}
		}
		r := row{"id": sid, "name": names.name(sid), "state": s.State, "open": names[sid] != nil, "dirs": sessionDirs(s, info)}
		root := s.Cwd
		if info != nil && info.Root != "" {
			root = info.Root
		}
		if a != nil {
			r["mode"], r["model"] = a.Mode, a.Model
			// a server can be known by its name and, from its tools, by its id: one line for both
			norm := func(s string) string { return strings.NewReplacer(".", "_", " ", "_", "-", "_").Replace(s) }
			byID := map[string]row{}
			for name, n := range a.MCP {
				id := norm(name)
				if have, ok := byID[id]; ok {
					have["tools"] = have["tools"].(int) + n
					if name != id { // the readable name wins
						have["name"] = name
					}
					continue
				}
				byID[id] = row{"name": name, "tools": n}
			}
			mcp := []any{}
			for _, m := range byID {
				mcp = append(mcp, m)
			}
			sort.Slice(mcp, func(i, j int) bool { return mcp[i].(row)["name"].(string) < mcp[j].(row)["name"].(string) })
			tools := make([]string, 0, len(a.Tools))
			for t := range a.Tools {
				tools = append(tools, t)
			}
			sort.Strings(tools)
			tl := make([]any, len(tools))
			for i, t := range tools {
				tl[i] = t
			}
			files := a.Files
			if files == nil {
				files = []any{}
			}
			r["mcp"], r["tools"], r["files"] = mcp, tl, files
		}
		rules := []any{}
		for _, x := range [][2]string{{filepath.Join(claudeHome(), "settings.json"), "user"},
			{filepath.Join(root, ".claude", "settings.json"), "project"}, {filepath.Join(root, ".claude", "settings.local.json"), "local"}} {
			if root == "" && x[1] != "user" {
				continue
			}
			if rl := rulesOf(x[0], x[1]); rl != nil {
				rules = append(rules, rl)
			}
		}
		r["rules"] = rules
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].(row)["open"] == true && out[j].(row)["open"] != true })
	return map[string]any{"sessions": out}
}
