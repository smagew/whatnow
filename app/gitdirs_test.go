package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A session that commits in another repository with `git -C` or `cd … &&`, without moving there, still has
// that repository on the Git tab of its feature. A repository it only read is not added.
func TestARepositoryCommittedInFromOutsideIsOnTheGitTab(t *testing.T) {
	e := withDemo(t)
	e.setAgents(map[string]map[string]any{"77777777-aaaa": {"name": "api-xx", "cwd": e.home, "status": "idle", "startedAt": 5}})
	mk := func(name string) string {
		d := filepath.Join(e.home, name)
		must(t, os.MkdirAll(d, 0o755))
		must(t, exec.Command("git", "-C", d, "init", "-q").Run())
		p, err := filepath.EvalSymlinks(d)
		must(t, err)
		return p
	}
	main, site, docs, read, rel := mk("main"), mk("site"), mk("docs"), mk("readonly"), mk("main/../rel")
	e.session("77777777-aaaa", "idle", "demo-feature")
	bash := func(cwd, cmd string) map[string]any {
		return map[string]any{"cwd": cwd, "message": map[string]any{"role": "assistant", "content": []any{
			map[string]any{"type": "tool_use", "name": "Bash", "input": map[string]any{"command": cmd}}}}}
	}
	e.appendTranscript(
		map[string]any{"cwd": main, "message": map[string]any{"role": "user", "content": "start"}},
		bash(main, `git -C `+site+` commit -m "docs: no imsi"`),
		bash(main, `cd "`+docs+`" && git add -A && git push origin HEAD`),
		bash(main, `cd ../rel && git commit -qm x`),
		bash(main, `git -C `+read+` log --oneline -5`),
		bash(main, `cd $SOMEWHERE && git commit -m y`))
	got := map[string]bool{}
	for _, r := range featureRepos("demo-feature") {
		got[r] = true
	}
	for name, want := range map[string]bool{site: true, docs: true, rel: true, read: false} {
		if got[name] != want {
			t.Errorf("%s on the Git tab: %v, want %v (all: %v)", filepath.Base(name), got[name], want, got)
		}
	}
}
