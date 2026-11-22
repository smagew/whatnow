package main

// What a session is about, read from its Claude Code transcript (~/.claude/projects/*/<id>.jsonl).

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	tailBytes = 600 * 1024
	fullChars = 6000 // characters of the last reply shown on a feature page
)

var (
	sidRe     = regexp.MustCompile(`^[0-9a-fA-F-]{8,64}$`)
	noiseOpen = regexp.MustCompile(`<(system-reminder|pasted_content|command-[a-z]+|local-command[a-z-]*|agent-message|task-notification)[^>]*>`)
	listMark  = regexp.MustCompile(`(?m)^\s{0,3}(?:#{1,6}|[-*])\s+`)
	spaces    = regexp.MustCompile(`[\s\p{Z}]+`)
	blankRuns = regexp.MustCompile(`\n{3,}`)
)

// stripNoise removes the blocks Claude Code adds around a message (reminders, pasted text, commands).
// RE2 has no back-references, so the closing tag is looked for by hand.
func stripNoise(t string) string {
	var out strings.Builder
	for {
		loc := noiseOpen.FindStringSubmatchIndex(t)
		if loc == nil {
			out.WriteString(t)
			return out.String()
		}
		name := t[loc[2]:loc[3]]
		rest := t[loc[1]:]
		end := -1
		for from := 0; ; {
			i := strings.Index(rest[from:], "</"+name)
			if i < 0 {
				break
			}
			j := strings.IndexByte(rest[from+i:], '>')
			if j < 0 {
				break
			}
			end = from + i + j + 1
			break
		}
		if end < 0 { // not closed: keep the tag as text and go on after it
			out.WriteString(t[:loc[1]])
			t = rest
			continue
		}
		out.WriteString(t[:loc[0]])
		t = rest[end:]
	}
}

func clean(t string) string {
	t = stripNoise(t)
	t = strings.ReplaceAll(strings.ReplaceAll(t, "**", ""), "`", "")
	t = listMark.ReplaceAllString(t, "")
	return strings.TrimSpace(spaces.ReplaceAllString(t, " "))
}

// full is a reply as it was written: line breaks and markdown stay, the page formats them.
func full(t string) string {
	t = strings.TrimSpace(blankRuns.ReplaceAllString(stripNoise(t), "\n\n"))
	if r := []rune(t); len(r) > fullChars {
		return strings.TrimRight(string(r[:fullChars]), " \t\n\r") + " …"
	}
	return t
}

func stamp(row map[string]any, def int64) int64 {
	s, _ := row["timestamp"].(string)
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return def
	}
	return t.UnixMilli()
}

func msgText(m map[string]any) string {
	switch c := m["content"].(type) {
	case string:
		return c
	case []any:
		var parts []string
		for _, x := range c {
			if p, ok := x.(map[string]any); ok && p["type"] == "text" {
				s, _ := p["text"].(string)
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}

func realUser(t string) bool {
	if len([]rune(t)) <= 3 {
		return false
	}
	for _, p := range []string{"This session is being continued", "Another Claude session", "Base directory for this skill"} {
		if strings.HasPrefix(t, p) {
			return false
		}
	}
	return true
}

type chatLine struct {
	who  string
	text string
	t    int64
}

type transcript struct {
	First, LastU, LastA, Said string
	Mtime, SaidT              int64
	Chat                      []chatLine
}

type cacheItem struct {
	path  string
	mtime int64
	size  int64
	info  *transcript
}

var (
	tcache   = map[string]cacheItem{}
	tcacheMu sync.Mutex
)

func findTranscript(sid string) string {
	tcacheMu.Lock()
	hit, ok := tcache[sid]
	tcacheMu.Unlock()
	if ok && isFile(hit.path) {
		return hit.path
	}
	base := filepath.Join(claudeHome(), "projects")
	dirs, err := os.ReadDir(base)
	if err != nil {
		return ""
	}
	for _, d := range dirs {
		if p := filepath.Join(base, d.Name(), sid+".jsonl"); isFile(p) {
			return p
		}
	}
	return ""
}

func rowOf(line []byte) (map[string]any, map[string]any) {
	row, _ := parseJSON(line).(map[string]any)
	if row == nil {
		return nil, nil
	}
	m, _ := row["message"].(map[string]any)
	if m == nil {
		m = map[string]any{}
	}
	return row, m
}

// transcriptInfo says how a session started, what the user asked last, what Claude said last, and the recent
// conversation. Cached until the file changes; only the head and the last 600 KB are read.
func transcriptInfo(sid string) *transcript {
	if !sidRe.MatchString(sid) {
		return nil
	}
	path := findTranscript(sid)
	if path == "" {
		return nil
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil
	}
	mtime, size := st.ModTime().UnixNano(), st.Size()
	tcacheMu.Lock()
	hit, ok := tcache[sid]
	tcacheMu.Unlock()
	if ok && hit.path == path && hit.mtime == mtime && hit.size == size {
		return hit.info
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	info := &transcript{Mtime: st.ModTime().UnixMilli()}
	info.SaidT = info.Mtime
	r := bufio.NewReaderSize(f, 1<<16)
	for i := 0; i <= 400 && info.First == ""; i++ {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			if _, m := rowOf(line); m != nil && m["role"] == "user" {
				if t := clean(msgText(m)); realUser(t) {
					info.First = t
				}
			}
		}
		if err != nil {
			break
		}
	}
	start := size - tailBytes
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil
	}
	buf, err := io.ReadAll(f)
	if err != nil {
		return nil
	}
	lines := bytes.Split(buf, []byte("\n"))
	if start > 0 && len(lines) > 0 {
		lines = lines[1:] // the first line is cut in the middle
	}
	for i := len(lines) - 1; i >= 0 && (info.LastU == "" || info.LastA == ""); i-- {
		row, m := rowOf(lines[i])
		if row == nil {
			continue
		}
		raw := msgText(m)
		t := clean(raw)
		if m["role"] == "assistant" && t != "" && info.LastA == "" {
			info.LastA, info.Said, info.SaidT = t, full(raw), stamp(row, info.Mtime)
		} else if m["role"] == "user" && realUser(t) && info.LastU == "" {
			info.LastU = t
		}
	}
	// The recent conversation, for the chat window: what the user typed and what Claude answered. A text that
	// Claude follows with a tool call is a note made on the way, not an answer: it is left out, except the
	// newest one, which is all there is to show while a long turn runs.
	var chat []chatLine
	var at int64
	var note *chatLine
	locked := true
	for _, line := range lines {
		row, m := rowOf(line)
		if row == nil {
			continue
		}
		role, _ := m["role"].(string)
		if (role != "user" && role != "assistant") || truthy(row["isSidechain"]) {
			continue
		}
		types := map[string]bool{}
		if c, ok := m["content"].([]any); ok {
			for _, x := range c {
				if p, ok := x.(map[string]any); ok {
					s, _ := p["type"].(string)
					types[s] = true
				}
			}
		}
		raw := msgText(m)
		t := clean(raw)
		if s := stamp(row, at); s > at {
			at = s // file order is the order of the conversation
		}
		if role == "assistant" {
			if t != "" {
				chat = append(chat, chatLine{"a", full(raw), at})
				locked = false
			}
			if types["tool_use"] && len(chat) > 0 && !locked {
				n := chat[len(chat)-1]
				note, chat, locked = &n, chat[:len(chat)-1], true
			}
		} else if !types["tool_result"] {
			locked = true // the turn ended: what Claude said last in it is its answer
			if realUser(t) && !strings.Contains(t, "[whatnow]") {
				chat = append(chat, chatLine{"u", runes(full(raw), 2000), at})
			}
		}
	}
	if note != nil && (len(chat) == 0 || note.t >= chat[len(chat)-1].t) {
		chat = append(chat, *note)
	}
	if len(chat) > 80 {
		chat = chat[len(chat)-80:]
	}
	info.First, info.LastU, info.LastA, info.Chat = runes(info.First, 140), runes(info.LastU, 160), runes(info.LastA, 200), chat
	tcacheMu.Lock()
	tcache[sid] = cacheItem{path, mtime, size, info}
	tcacheMu.Unlock()
	return info
}

var _ = json.Number("") // parsed transcripts keep numbers as json.Number
