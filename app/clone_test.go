package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheAddressOfARepository(t *testing.T) {
	for addr, want := range map[string]string{
		"https://github.com/acme/tool.git":       "github.com/acme/tool tool",
		"https://GitHub.com/acme/tool/":          "github.com/acme/tool tool",
		"git@github.com:acme/tool.git":           "github.com/acme/tool tool",
		"ssh://git@git.example.org:2222/a/b/c":   "git.example.org/a/b/c c",
		"http://git.local/team/site.git":         "git.local/team/site site",
		"":                                       "",
		"--upload-pack=touch /tmp/x":             "",
		"-oProxyCommand=x@host:a/b":              "",
		"ext::sh -c 'touch /tmp/x'":              "",
		"file:///etc":                            "",
		"/some/local/path":                       "",
		"https://github.com/":                    "",
		"https://github.com/acme/..":             "",
		"https://github.com/acme/tool.git; rm x": "",
	} {
		key, name, ok := repoAddr(addr)
		got := ""
		if ok {
			got = key + " " + name
		}
		eq(t, got, want, addr)
	}
}

// New work from a repository: cloned into the folder the user chose, the session starts in the clone; a second
// time the clone that is there is used; a folder of that name that is something else is left alone.
func TestNewWorkFromARepository(t *testing.T) {
	e := withDemo(t)
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.test", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.test")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	// the "remote": a local repository that git is told to use for an https address
	src := filepath.Join(e.home, "remote", "acme", "tool.git")
	must(t, os.MkdirAll(src, 0o755))
	run(src, "init", "-q")
	must(t, os.WriteFile(filepath.Join(src, "README"), []byte("hi\n"), 0o644))
	run(src, "add", "README")
	run(src, "commit", "-q", "-m", "first")
	cfg := filepath.Join(e.home, "gitconfig")
	must(t, os.WriteFile(cfg, []byte("[url \"file://"+filepath.ToSlash(filepath.Join(e.home, "remote"))+"/\"]\n\tinsteadOf = https://example.test/\n"), 0o644))
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	old := cloneProtocols
	cloneProtocols += ":file"
	t.Cleanup(func() { cloneProtocols = old })

	into := filepath.Join(e.home, "code")
	must(t, os.MkdirAll(into, 0o755))
	addr := "https://example.test/acme/tool.git"
	root := filepath.Join(into, "tool")
	e.setAgents(map[string]map[string]any{"99999999-cl01": {"id": "ab12cd34", "name": "tool", "cwd": root, "status": "idle", "startedAt": 5}})

	code, r := act2(t, map[string]any{"action": "spawn", "repo": "file:///etc", "into": into})
	eq(t, code, 400, r)
	code, r = act2(t, map[string]any{"action": "spawn", "repo": addr, "into": ""})
	eq(t, code, 400, r)
	code, r = act2(t, map[string]any{"action": "spawn", "repo": addr, "into": filepath.Join(e.home, "nope")})
	eq(t, code, 404, r)

	code, p := clonePlan(addr, into, "")
	eq(t, code, 200, p)
	eq(t, p["exists"], any(false))
	eq(t, p["root"], any(root))

	// a repository that cannot be cloned: said, and nothing is left behind
	code, r = act2(t, map[string]any{"action": "spawn", "repo": "https://example.test/acme/absent.git", "into": into})
	eq(t, code, 502, r)
	contains(t, fmt.Sprint(r["error"]), "could not clone", "the reason")
	if _, err := os.Stat(filepath.Join(into, "absent")); err == nil {
		t.Error("the failed clone left a folder behind")
	}

	code, r = act2(t, map[string]any{"action": "spawn", "repo": addr, "into": into, "mode": "plan", "text": "", "place": "worktree"})
	eq(t, code, 200, r)
	if !isFile(filepath.Join(root, "README")) {
		t.Fatal("the repository was not cloned")
	}
	args := strings.Join(strings.Split(strings.TrimSpace(string(mustRead(t, filepath.Join(e.fake, "spawn.args")))), "\n"), "|")
	contains(t, args, "--permission-mode|plan", "the chosen mode")
	if strings.Contains(args, "--worktree") {
		t.Errorf("a fresh clone needs no second copy: %s", args)
	}
	if a, _ := filepath.EvalSymlinks(strings.TrimSpace(string(mustRead(t, filepath.Join(e.fake, "spawn.cwd"))))); a != mustEval(t, root) {
		t.Errorf("started in %s", a)
	}
	eq(t, r["feature"], any("tool"))

	// again: the clone is there, it is used (the ssh address of the same repository is the same repository)
	must(t, os.WriteFile(filepath.Join(root, "mine.txt"), []byte("x"), 0o644))
	code, p = clonePlan("git@example.test:acme/tool.git", into, "")
	eq(t, code, 200, p)
	eq(t, p["exists"], any(true))
	code, r = act2(t, map[string]any{"action": "spawn", "repo": addr, "into": into, "text": ""})
	eq(t, code, 200, r)
	if !isFile(filepath.Join(root, "mine.txt")) {
		t.Error("the existing clone was replaced")
	}

	// a folder of that name that is something else is not touched
	other := filepath.Join(into, "site")
	must(t, os.MkdirAll(other, 0o755))
	must(t, os.WriteFile(filepath.Join(other, "keep"), []byte("x"), 0o644))
	code, r = act2(t, map[string]any{"action": "spawn", "repo": "https://example.test/acme/site.git", "into": into})
	eq(t, code, 409, r)
	if !isFile(filepath.Join(other, "keep")) {
		t.Error("the other folder was touched")
	}

	// Claude Code does not trust the new folder yet: the clone stays, the answer names it
	e.fakeSet("spawn.out", "Workspace not trusted. Run `claude` once.")
	must(t, os.RemoveAll(root))
	code, r = act2(t, map[string]any{"action": "spawn", "repo": addr, "into": into})
	eq(t, code, 409, r)
	if r["trust"] == nil || r["cloned"] == nil || !isFile(filepath.Join(root, "README")) {
		t.Errorf("the clone should stay and be named: %v", r)
	}
}
