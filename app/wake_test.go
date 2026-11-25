package main

import (
	"strings"
	"testing"
)

// Nothing is typed into a session whose screen asks a question: there Enter would be an answer.
func TestNoLineIsTypedIntoAQuestion(t *testing.T) {
	clean := func(s string) string { return strings.Join(strings.Fields(ansiRe.ReplaceAllString(s, " ")), " ") }
	asks := []string{
		"\x1b[1mDo you want to proceed?\x1b[0m\r\n \x1b[36m❯ 1. Yes\x1b[0m\r\n   2. No",
		"Do you want to overwrite feature.json ?",
		"Esc to cancel · Tab to amend",
		"Continue? (y/n)",
	}
	for _, s := range asks {
		if !askRe.MatchString(clean(s)) {
			t.Errorf("not seen as a question: %q", s)
		}
	}
	idle := []string{
		"\x1b[2J\x1b[H ▐▛███▜▌ Claude Code v2.1\r\n\x1b[38;2;153;153;153m❯ \x1b[0m\r\n  ? for shortcuts",
		"⏺ PONG\r\n\r\n❯ ",
	}
	for _, s := range idle {
		if askRe.MatchString(clean(s)) {
			t.Errorf("an idle prompt was taken for a question: %q", s)
		}
	}
	if wakeSession("not a session id!") {
		t.Error("a bad id was accepted")
	}
	e := withDemo(t)
	e.setAgents(map[string]map[string]any{"77777777-aaaa": {"name": "x", "kind": "interactive", "cwd": e.home, "status": "idle"}})
	if wakeSession("77777777-aaaa") {
		t.Error("a session open in the user's own terminal cannot be woken by typing, yet it was tried")
	}
}
