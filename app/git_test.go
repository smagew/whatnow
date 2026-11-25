package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The address of a remote becomes the address of the repository's page; a name and a password in it are dropped.
func TestWebAddressOfARemote(t *testing.T) {
	for remote, want := range map[string][2]string{
		"git@github.com:acme/app.git":                     {"https://github.com/acme/app", "github.com"},
		"https://github.com/acme/app.git":                 {"https://github.com/acme/app", "github.com"},
		"https://user:secret@git.example.org/grp/sub/app": {"https://git.example.org/grp/sub/app", "git.example.org"},
		"ssh://git@git.example.org:2222/grp/app.git":      {"https://git.example.org/grp/app", "git.example.org"},
		"git@git.example.org:grp/sub/app.git":             {"https://git.example.org/grp/sub/app", "git.example.org"},
		"/srv/repos/app.git":                              {"", ""},
	} {
		web, host := webOf(remote)
		eq(t, web, want[0], remote)
		eq(t, host, want[1], remote)
		if strings.Contains(web, "secret") || strings.Contains(web, "@") {
			t.Errorf("%q: credentials left in %q", remote, web)
		}
	}
}

// The kind of the server is read from what it publishes: merge requests mean GitLab, pull requests mean GitHub,
// whatever the server is called. The host name and config.json answer only when the remote said nothing.
func TestKindOfTheServer(t *testing.T) {
	e := isolate(t)
	eq(t, forgeOf("git.example.org", map[string]string{"refs/merge-requests/7/head": "a"}), "gitlab")
	eq(t, forgeOf("code.example.org", map[string]string{"refs/pull/7/head": "a"}), "github")
	eq(t, forgeOf("github.com", nil), "github")
	eq(t, forgeOf("gitlab.example.org", nil), "gitlab")
	eq(t, forgeOf("git.example.org", nil), "", "not known")
	e.writeConfig(map[string]any{"gitHosts": map[string]any{"git.example.org": "gitlab"}})
	eq(t, forgeOf("git.example.org", nil), "gitlab", "set in config.json")
}

func gitDo(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=T", "-c", "user.email=t@example.org", "-c", "commit.gpgsign=false"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// A repository with a remote: a branch with one pushed and one unpushed commit, and a file in no commit.
func TestStateOfARepository(t *testing.T) {
	isolate(t)
	base := t.TempDir()
	remote, repo := filepath.Join(base, "remote.git"), filepath.Join(base, "app")
	must(t, exec.Command("git", "init", "-q", "--bare", "-b", "main", remote).Run())
	must(t, exec.Command("git", "init", "-q", "-b", "main", repo).Run())
	if real, err := filepath.EvalSymlinks(repo); err == nil {
		repo = real // git prints the real path (/private/var on macOS)
	}
	commit := func(file, msg string) {
		writeFile(t, filepath.Join(repo, file), []byte(msg))
		gitDo(t, repo, "add", file)
		gitDo(t, repo, "commit", "-q", "-m", msg)
	}
	gitDo(t, repo, "remote", "add", "origin", remote)
	commit("a.txt", "first on main")
	gitDo(t, repo, "push", "-q", "-u", "origin", "main")
	gitDo(t, repo, "checkout", "-q", "-b", "feature/x")
	commit("b.txt", "pushed work")
	gitDo(t, repo, "push", "-q", "-u", "origin", "feature/x")
	commit("c.txt", "work of the session, not pushed")
	writeFile(t, filepath.Join(repo, "d.txt"), []byte("in no commit"))

	eq(t, repoOf(filepath.Join(repo)), repo, "the repository of a folder")
	eq(t, repoOf(base), "", "a folder outside any repository")
	r := repoState(repo, []string{"work of the session, not pushed and the body that the hook joined to it"}, true)
	eq(t, r["branch"], any("feature/x"))
	eq(t, r["base"], any("main"), "the main branch")
	eq(t, r["dirty"], any(1), "files in no commit")
	eq(t, r["ahead"], any(1), "not pushed")
	eq(t, r["behind"], any(0))
	eq(t, r["baseAhead"], any(2), "commits the main branch does not have")
	eq(t, r["baseBehind"], any(0))
	eq(t, r["onRemote"], any(true), "the branch is on the remote")
	c := r["commits"].([]any)
	eq(t, len(c), 2, "only the commits of the branch")
	first, second := c[0].(map[string]any), c[1].(map[string]any)
	eq(t, first["subject"], any("work of the session, not pushed"))
	eq(t, first["pushed"], any(false))
	eq(t, first["mine"], any(true), "made by a session of the feature")
	eq(t, second["pushed"], any(true))
	if second["mine"] != nil {
		t.Error("a commit of somebody else is marked as the feature's")
	}
	// nothing was written to the repository
	if _, err := os.Stat(filepath.Join(repo, ".git", "index.lock")); err == nil {
		t.Error("a lock was left in the repository")
	}
}

// Without a remote there is nowhere to push: no commit is called "not pushed".
func TestARepositoryWithoutARemote(t *testing.T) {
	isolate(t)
	repo := t.TempDir()
	must(t, exec.Command("git", "init", "-q", "-b", "main", repo).Run())
	writeFile(t, filepath.Join(repo, "a.txt"), []byte("x"))
	gitDo(t, repo, "add", "a.txt")
	gitDo(t, repo, "commit", "-q", "-m", "only here")
	r := repoState(repo, nil, true)
	eq(t, r["remote"], any(false))
	c := r["commits"].([]any)[0].(map[string]any)
	if _, has := c["pushed"]; has {
		t.Error("a commit of a repository without a remote says whether it is pushed")
	}
}

// An item of the plan that names a commit ("commit 3f2a9c1") is linked to that commit; a word or a number that only
// looks like a hash is not.
func TestThePlanNamesItsCommits(t *testing.T) {
	e := withDemo(t)
	repo := t.TempDir()
	must(t, exec.Command("git", "init", "-q", "-b", "main", repo).Run())
	writeFile(t, filepath.Join(repo, "a.txt"), []byte("x"))
	gitDo(t, repo, "add", "a.txt")
	gitDo(t, repo, "commit", "-q", "-m", "the work")
	out, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	must(t, err)
	full := strings.TrimSpace(string(out))
	d := demoFeature()
	st := d["stages"].([]any)[0].(map[string]any)
	st["result"] = "The stage ended well.\n\n- one\n- two"
	st["facts"] = []any{
		[]any{"y", "Schema written, commit " + full[:8], "The detail: what was done.\n\nChecked with the tests, 1234567 rows, see deadbee."},
		[]any{"q", "Not done yet"}}
	writeFile(t, e.demoSrc(), encodeJSON(d))
	loaded := e.loadDemo() // a detail and a result are accepted
	eq(t, len(loaded["stages"].([]any)[0].(map[string]any)["facts"].([]any)[0].([]any)), 3, "the detail stays with the item")
	refs := planRefs("demo-feature", []string{repo})
	eq(t, len(refs), 1, "only the real commit")
	c := refs[full[:7]].(map[string]any)
	eq(t, c["hash"], any(full))
	eq(t, c["subject"], any("the work"))
	// a detail that is not text is refused with a message, like any other mistake in the file
	st["facts"] = []any{[]any{"y", "x", 5}}
	writeFile(t, e.demoSrc(), encodeJSON(d))
	if _, err := load(e.demoSrc()); err == nil || !strings.Contains(err.Error(), "detail") {
		t.Errorf("a detail that is not text was accepted: %v", err)
	}
}

// A feature whose sessions never worked in a repository has no Git tab.
func TestNoRepositoriesNoTab(t *testing.T) {
	withDemo(t)
	eq(t, len(gitState("demo-feature", false)["repos"].([]any)), 0)
}
